package control

import (
	"appcontrol/server/internal/storage"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type RemoteCall struct {
	CallID               string `json:"callId"`
	State                string `json:"state"`
	RemoteActionsAllowed bool   `json:"remoteActionsAllowed"`
}

type RemoteCalls struct {
	ObservedAt       *time.Time   `json:"observedAt,omitempty"`
	ServerTime       time.Time    `json:"serverTime"`
	SnapshotSequence int64        `json:"snapshotSequence,omitempty"`
	Calls            []RemoteCall `json:"calls"`
}

var validRemoteCallStates = map[string]bool{
	"RINGING": true, "ACTIVE": true, "HOLDING": true, "DIALING": true,
	"CONNECTING": true, "SELECTING_ACCOUNT": true, "DISCONNECTING": true,
	"DISCONNECTED": true, "OTHER": true,
}

func (s *Service) AllocateRemoteCallSequence(ctx context.Context, token string) (int64, error) {
	var sequence int64
	err := storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		id, err := deviceAuthorization(ctx, tx, token, s.now().UTC())
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO remote_call_snapshot_counters(device_id,sequence)
			VALUES(?,1) ON CONFLICT(device_id) DO UPDATE SET sequence=sequence+1`, id); err != nil {
			return err
		}
		return tx.QueryRowContext(ctx, "SELECT sequence FROM remote_call_snapshot_counters WHERE device_id=?", id).Scan(&sequence)
	})
	return sequence, err
}

func (s *Service) ReportRemoteCalls(ctx context.Context, token string, sequence int64, calls []RemoteCall) (bool, error) {
	if sequence < 1 || calls == nil || len(calls) > 8 {
		return false, ErrInvalid
	}
	seen := make(map[string]bool, len(calls))
	for index, call := range calls {
		if !callIDPattern.MatchString(call.CallID) || seen[call.CallID] || !validRemoteCallStates[call.State] {
			return false, ErrInvalid
		}
		if !s.remoteCallsEnabled {
			calls[index].RemoteActionsAllowed = false
		}
		seen[call.CallID] = true
	}
	raw, err := json.Marshal(calls)
	if err != nil {
		return false, err
	}
	accepted := false
	err = storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		now := s.now().UTC()
		id, err := deviceAuthorization(ctx, tx, token, now)
		if err != nil {
			return err
		}
		var allocated int64
		err = tx.QueryRowContext(ctx, "SELECT sequence FROM remote_call_snapshot_counters WHERE device_id=?", id).Scan(&allocated)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalid
		}
		if err != nil {
			return err
		}
		if sequence > allocated {
			return ErrInvalid
		}
<<<<<<< HEAD
		previous := make(map[string]string)
		var previousJSON string
		err = tx.QueryRowContext(ctx, `SELECT calls_json FROM remote_call_snapshots WHERE device_id=?`, id).Scan(&previousJSON)
		if err == nil {
			var oldCalls []RemoteCall
			if err := json.Unmarshal([]byte(previousJSON), &oldCalls); err != nil {
				return err
			}
			for _, call := range oldCalls {
				previous[call.CallID] = call.State
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
=======
>>>>>>> origin/main
		result, err := tx.ExecContext(ctx, `INSERT INTO remote_call_snapshots(device_id,parent_account_id,observed_at,calls_json,snapshot_sequence)
		 SELECT id,parent_account_id,?,?,? FROM devices WHERE id=?
		 ON CONFLICT(device_id) DO UPDATE SET parent_account_id=excluded.parent_account_id,
		 observed_at=excluded.observed_at,calls_json=excluded.calls_json,snapshot_sequence=excluded.snapshot_sequence
		 WHERE excluded.snapshot_sequence > remote_call_snapshots.snapshot_sequence`,
			now.Format(time.RFC3339Nano), string(raw), sequence, id)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		accepted = rows == 1
<<<<<<< HEAD
		if accepted {
			var parentID string
			if err := tx.QueryRowContext(ctx, `SELECT parent_account_id FROM devices WHERE id=? AND state='active'`, id).Scan(&parentID); err != nil {
				return err
			}
			current := make(map[string]string, len(calls))
			for _, call := range calls {
				current[call.CallID] = call.State
			}
			events, err := tx.QueryContext(ctx, `SELECT event_id,call_id FROM parent_call_events
				WHERE device_id=? AND status='active'`, id)
			if err != nil {
				return err
			}
			var expiredEvents []string
			for events.Next() {
				var eventID, callID string
				if err := events.Scan(&eventID, &callID); err != nil {
					events.Close()
					return err
				}
				if current[callID] != "RINGING" {
					expiredEvents = append(expiredEvents, eventID)
				}
			}
			if err := events.Err(); err != nil {
				events.Close()
				return err
			}
			if err := events.Close(); err != nil {
				return err
			}
			for _, eventID := range expiredEvents {
				if _, err := tx.ExecContext(ctx, `UPDATE parent_call_events SET status='expired' WHERE event_id=?`, eventID); err != nil {
					return err
				}
				if _, err := tx.ExecContext(ctx, `UPDATE parent_call_event_deliveries SET status='expired',
					lease_id=NULL,lease_expires_at_ns=NULL WHERE event_id=? AND status IN ('pending','leased')`, eventID); err != nil {
					return err
				}
			}
			for _, call := range calls {
				if call.State != "RINGING" || previous[call.CallID] == "RINGING" {
					continue
				}
				eventID := id + ":" + call.CallID
				if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO parent_call_events
					(event_id,device_id,parent_account_id,call_id,snapshot_sequence,observed_at,created_at_ns,expires_at_ns,status)
					VALUES(?,?,?,?,?,?,?,?, 'active')`,
					eventID, id, parentID, call.CallID, sequence, now.Format(time.RFC3339Nano),
					now.UnixNano(), now.Add(30*time.Second).UnixNano()); err != nil {
					return err
				}
			}
		}
=======
>>>>>>> origin/main
		return nil
	})
	return accepted, err
}

func (s *Service) RemoteCalls(ctx context.Context, parentID, deviceID string) (RemoteCalls, error) {
	result := RemoteCalls{ServerTime: s.now().UTC(), Calls: []RemoteCall{}}
	err := storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		if err := owned(ctx, tx, parentID, deviceID); err != nil {
			return err
		}
		var observed, raw string
		err := tx.QueryRowContext(ctx, `SELECT observed_at,calls_json FROM remote_call_snapshots
		 WHERE device_id=? AND parent_account_id=?`, deviceID, parentID).Scan(&observed, &raw)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		when, err := time.Parse(time.RFC3339Nano, observed)
		if err != nil {
			return err
		}
		result.ObservedAt = &when
		if err := tx.QueryRowContext(ctx, `SELECT snapshot_sequence FROM remote_call_snapshots WHERE device_id=? AND parent_account_id=?`,
			deviceID, parentID).Scan(&result.SnapshotSequence); err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(raw), &result.Calls); err != nil {
			return err
		}
		if !s.remoteCallsEnabled {
			for index := range result.Calls {
				result.Calls[index].RemoteActionsAllowed = false
			}
		}
		return nil
	})
	return result, err
}

func (s *Service) requireCurrentCall(ctx context.Context, tx *sql.Tx, deviceID, callID, action string, now time.Time) error {
	var observed, raw string
	var sequence int64
	err := tx.QueryRowContext(ctx, `SELECT observed_at,calls_json,snapshot_sequence FROM remote_call_snapshots
	 WHERE device_id=?`, deviceID).Scan(&observed, &raw, &sequence)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrCallUnavailable
	}
	if err != nil {
		return err
	}
	observedAt, err := time.Parse(time.RFC3339Nano, observed)
	if err != nil {
		return err
	}
	if now.Sub(observedAt) < 0 || now.Sub(observedAt) > 30*time.Second {
		return ErrCallUnavailable
	}
	if !s.remoteCallsEnabled {
		return ErrRemoteCallDisabled
	}
	var calls []RemoteCall
	if err := json.Unmarshal([]byte(raw), &calls); err != nil {
		return err
	}
	for _, call := range calls {
		if call.CallID != callID {
			continue
		}
		if !call.RemoteActionsAllowed {
			return ErrRemoteCallDisabled
		}
		if sequence < 1 {
			return ErrCallUnavailable
		}
		if action == "ANSWER_CALL" || action == "REJECT_CALL" {
			if call.State != "RINGING" {
				return ErrCallState
			}
			return nil
		}
		if call.State != "ACTIVE" && call.State != "HOLDING" {
			return ErrCallState
		}
		return nil
	}
	return ErrCallUnavailable
}
