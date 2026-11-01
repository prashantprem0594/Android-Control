package parentpush

import (
	"appcontrol/server/internal/storage"
	"context"
	"database/sql"
	"encoding/json"
	"regexp"
	"time"
)

const maxFeedEvents = 100

var feedCallID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// Validate the whole bounded snapshot before projecting an event. A valid
// ringing member must not hide null, scalar, duplicate or malformed siblings.
func feedRingingSnapshot(raw, callID string, observed, now time.Time) bool {
	age := now.Sub(observed)
	if age < 0 || age > eventFreshness || len(raw) > 4096 {
		return false
	}
	var calls []map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &calls) != nil || calls == nil || len(calls) > 8 {
		return false
	}
	seen := make(map[string]bool, len(calls))
	ringing := false
	for _, call := range calls {
		var id, state string
		if call == nil || json.Unmarshal(call["callId"], &id) != nil || !feedCallID.MatchString(id) || seen[id] ||
			json.Unmarshal(call["state"], &state) != nil {
			return false
		}
		switch state {
		case "RINGING", "ACTIVE", "HOLDING", "DIALING", "CONNECTING", "SELECTING_ACCOUNT", "DISCONNECTING", "DISCONNECTED", "OTHER":
		default:
			return false
		}
		if encoded, exists := call["remoteActionsAllowed"]; exists {
			var allowed *bool
			if json.Unmarshal(encoded, &allowed) != nil || allowed == nil {
				return false
			}
		}
		seen[id] = true
		if id == callID && state == "RINGING" {
			ringing = true
		}
	}
	return ringing
}

// CallEvent contains navigation and freshness metadata only, never caller details
// or routing tokens. An event is not permission to perform a call action.
type CallEvent struct {
	EventID          string    `json:"eventId"`
	DeviceID         string    `json:"deviceId"`
	CallID           string    `json:"callId"`
	SnapshotSequence int64     `json:"snapshotSequence"`
	ObservedAt       time.Time `json:"observedAt"`
	ExpiresAt        time.Time `json:"expiresAt"`
}

type CallEventFeed struct {
	ServerTime time.Time   `json:"serverTime"`
	Events     []CallEvent `json:"events"`
}

// CallEvents reads a bounded, newest-first current view. Family validation and
// device ownership are checked in the same transaction as the event snapshot.
// No Firebase registration is required: possession of a routing token is never
// account authorization, and explicit authenticated synchronization still works.
func (s *Store) CallEvents(ctx context.Context, parentID, familyID string) (CallEventFeed, error) {
	result := CallEventFeed{Events: []CallEvent{}}
	if parentID == "" || familyID == "" {
		return result, ErrUnauthorized
	}
	err := storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		now := s.now().UTC()
		result.ServerTime = now
		if err := s.requireActiveFamily(ctx, tx, parentID, familyID, now); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, `SELECT e.event_id,e.device_id,e.call_id,s.snapshot_sequence,
			e.observed_at,e.expires_at_ns,s.observed_at,s.calls_json
			FROM parent_call_events e
			JOIN devices d ON d.id=e.device_id AND d.state='active' AND d.parent_account_id=e.parent_account_id
			JOIN remote_call_snapshots s ON s.device_id=e.device_id AND s.parent_account_id=e.parent_account_id
			WHERE e.parent_account_id=? AND e.status='active' AND e.expires_at_ns>?
			AND e.created_at_ns<=? AND e.created_at_ns>=? AND s.snapshot_sequence>=e.snapshot_sequence
			AND length(CAST(s.calls_json AS BLOB))<=4096
			AND EXISTS(SELECT 1 FROM json_each(CASE WHEN json_valid(s.calls_json) THEN s.calls_json ELSE '[]' END) c
				WHERE CASE WHEN c.type='object' THEN json_extract(c.value,'$.callId') END=e.call_id
				AND CASE WHEN c.type='object' THEN json_extract(c.value,'$.state') END='RINGING')
			ORDER BY e.created_at_ns DESC,e.event_id DESC LIMIT ?`,
			parentID, now.UnixNano(), now.UnixNano(), now.Add(-eventFreshness).UnixNano(), maxFeedEvents)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var event CallEvent
			var eventObserved, latestObserved, callsJSON string
			var expiresNS int64
			if err := rows.Scan(&event.EventID, &event.DeviceID, &event.CallID, &event.SnapshotSequence,
				&eventObserved, &expiresNS, &latestObserved, &callsJSON); err != nil {
				return err
			}
			observed, err := time.Parse(time.RFC3339Nano, eventObserved)
			if err != nil || now.Before(observed) || now.Sub(observed) > eventFreshness {
				continue
			}
			latest, err := time.Parse(time.RFC3339Nano, latestObserved)
			if err != nil || latest.Before(observed) || !feedRingingSnapshot(callsJSON, event.CallID, latest, now) {
				continue
			}
			// Return coherent latest snapshot metadata while preserving the first
			// ringing event's deadline. Repeated snapshots never extend alert TTL.
			event.ObservedAt = latest.UTC()
			event.ExpiresAt = time.Unix(0, expiresNS).UTC()
			if deadline := observed.Add(eventFreshness); deadline.Before(event.ExpiresAt) {
				event.ExpiresAt = deadline
			}
			if deadline := latest.Add(eventFreshness); deadline.Before(event.ExpiresAt) {
				event.ExpiresAt = deadline
			}
			if !now.Before(event.ExpiresAt) {
				continue
			}
			result.Events = append(result.Events, event)
		}
		return rows.Err()
	})
	if err != nil {
		return CallEventFeed{}, err
	}
	return result, nil
}
