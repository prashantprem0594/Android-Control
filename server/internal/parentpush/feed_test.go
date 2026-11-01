package parentpush

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func seedFeedEvent(t *testing.T, db *sql.DB, parent, device, call string, observed time.Time, state string) string {
	t.Helper()
	stamp := observed.UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO devices(id,parent_account_id,display_name,public_key_der,key_algorithm,state,created_at)
		VALUES(?,?,'fixture',X'00','ECDSA_P256_SHA256','active',?)`, device, parent, stamp); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal([]map[string]any{{"callId": call, "state": state, "callerName": "private-caller-fixture"}})
	if _, err := db.Exec(`INSERT INTO remote_call_snapshots(device_id,parent_account_id,observed_at,calls_json,snapshot_sequence)
		VALUES(?,?,?,?,1)`, device, parent, stamp, string(raw)); err != nil {
		t.Fatal(err)
	}
	id := device + ":" + call
	if _, err := db.Exec(`INSERT INTO parent_call_events(event_id,device_id,parent_account_id,call_id,snapshot_sequence,
		observed_at,created_at_ns,expires_at_ns,status) VALUES(?,?,?,?,1,?,?,?,'active')`,
		id, device, parent, call, stamp, observed.UnixNano(), observed.Add(eventFreshness).UnixNano()); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestParentCallFeedOwnershipAndFamilyAuthorization(t *testing.T) {
	db := openPushDB(t)
	now := time.Now().UTC()
	seedParentAndFamily(t, db, "owner", "owner-family", now.Add(time.Hour))
	seedParentAndFamily(t, db, "other", "other-family", now.Add(time.Hour))
	store := newPushStore(t, db, &pushClock{now: now})
	id := seedFeedEvent(t, db, "owner", "device", "a3d7a190-8503-4bf4-9c12-123456789abc", now.Add(-time.Second), "RINGING")
	feed, err := store.CallEvents(context.Background(), "owner", "owner-family")
	if err != nil || len(feed.Events) != 1 || feed.Events[0].EventID != id || !feed.ServerTime.Equal(now) {
		t.Fatal("fresh owned event unavailable")
	}
	if !feed.Events[0].ExpiresAt.Equal(now.Add(29*time.Second)) || feed.Events[0].SnapshotSequence != 1 {
		t.Fatal("event deadline or sequence changed")
	}
	if _, err := db.Exec(`UPDATE remote_call_snapshots SET observed_at=?,snapshot_sequence=2 WHERE device_id='device'`, now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	feed, err = store.CallEvents(context.Background(), "owner", "owner-family")
	if err != nil || len(feed.Events) != 1 || feed.Events[0].SnapshotSequence != 2 || !feed.Events[0].ObservedAt.Equal(now) || !feed.Events[0].ExpiresAt.Equal(now.Add(29*time.Second)) {
		t.Fatal("latest snapshot metadata was incoherent or extended first-ring expiry")
	}
	feed, err = store.CallEvents(context.Background(), "other", "other-family")
	if err != nil || len(feed.Events) != 0 || feed.Events == nil {
		t.Fatal("cross-owner event exposure or null empty feed")
	}
	for _, scope := range [][2]string{{"owner", "other-family"}, {"other", "owner-family"}, {"", "owner-family"}, {"owner", "missing"}} {
		if _, err := store.CallEvents(context.Background(), scope[0], scope[1]); !errors.Is(err, ErrUnauthorized) {
			t.Fatal("unauthorized family accepted")
		}
	}
}

func TestParentCallFeedRejectsRevokedStaleEndedAndReorderedState(t *testing.T) {
	mutations := map[string]string{
		"revoked-family":     `UPDATE auth_session_families SET revoked_at=?,revocation_reason='logout' WHERE id='family'`,
		"expired-family":     `UPDATE auth_session_families SET expires_at=? WHERE id='family'`,
		"revoked-device":     `UPDATE devices SET state='revoked',revoked_at=?,revocation_reason='parent_revoked' WHERE id='device'`,
		"changed-owner":      `UPDATE devices SET parent_account_id='other' WHERE id='device'`,
		"ended":              `UPDATE remote_call_snapshots SET calls_json='[]' WHERE device_id='device'`,
		"reordered":          `UPDATE remote_call_snapshots SET snapshot_sequence=0 WHERE device_id='device'`,
		"old-snapshot":       `UPDATE remote_call_snapshots SET observed_at=? WHERE device_id='device'`,
		"future-snapshot":    `UPDATE remote_call_snapshots SET observed_at=? WHERE device_id='device'`,
		"expired-event":      `UPDATE parent_call_events SET expires_at_ns=? WHERE device_id='device'`,
		"malformed-snapshot": `UPDATE remote_call_snapshots SET calls_json='invalid' WHERE device_id='device'`,
	}
	for name, mutation := range mutations {
		t.Run(name, func(t *testing.T) {
			db := openPushDB(t)
			now := time.Now().UTC()
			seedParentAndFamily(t, db, "owner", "family", now.Add(time.Hour))
			if name == "changed-owner" {
				seedParentAndFamily(t, db, "other", "other-family", now.Add(time.Hour))
			}
			store := newPushStore(t, db, &pushClock{now: now})
			seedFeedEvent(t, db, "owner", "device", "a3d7a190-8503-4bf4-9c12-123456789abc", now, "RINGING")
			var args []any
			switch name {
			case "revoked-family", "expired-family", "revoked-device":
				args = []any{now.Format(time.RFC3339Nano)}
			case "old-snapshot":
				args = []any{now.Add(-31 * time.Second).Format(time.RFC3339Nano)}
			case "future-snapshot":
				args = []any{now.Add(time.Second).Format(time.RFC3339Nano)}
			case "expired-event":
				args = []any{now.UnixNano()}
			}
			if _, err := db.Exec(mutation, args...); err != nil {
				t.Fatal(err)
			}
			feed, err := store.CallEvents(context.Background(), "owner", "family")
			if name == "revoked-family" || name == "expired-family" {
				if !errors.Is(err, ErrUnauthorized) {
					t.Fatal("inactive family accepted")
				}
			} else if err != nil || len(feed.Events) != 0 {
				t.Fatal("ineligible event exposed")
			}
		})
	}
}

func TestParentCallFeedBoundedNewestFirstAndFiltersBeforeLimit(t *testing.T) {
	db := openPushDB(t)
	now := time.Now().UTC()
	seedParentAndFamily(t, db, "owner", "family", now.Add(time.Hour))
	store := newPushStore(t, db, &pushClock{now: now})
	for i := 0; i < 225; i++ {
		state := "RINGING"
		if i >= 125 {
			state = "ACTIVE"
		}
		seedFeedEvent(t, db, "owner", fmt.Sprintf("device-%03d", i), fmt.Sprintf("00000000-0000-4000-8000-%012d", i), now.Add(time.Duration(i-225)*time.Millisecond), state)
	}
	feed, err := store.CallEvents(context.Background(), "owner", "family")
	if err != nil || len(feed.Events) != maxFeedEvents || feed.Events[0].DeviceID != "device-124" || feed.Events[99].DeviceID != "device-025" {
		t.Fatal("feed bound, pre-limit filtering or deterministic order failed")
	}
	if _, err := db.Exec(`UPDATE parent_call_events SET parent_account_id='owner',expires_at_ns=? WHERE device_id='device-124'`, now.Add(time.Hour).UnixNano()); err != nil {
		t.Fatal(err)
	}
	feed, err = store.CallEvents(context.Background(), "owner", "family")
	if err != nil || len(feed.Events) == 0 || feed.Events[0].ExpiresAt.After(feed.Events[0].ObservedAt.Add(eventFreshness)) {
		t.Fatal("event freshness could be extended past the observation")
	}
}

func TestParentCallFeedMalformedMembersFailClosedWithoutQueryErrors(t *testing.T) {
	const callID = "a3d7a190-8503-4bf4-9c12-123456789abc"
	valid := `{"callId":"` + callID + `","state":"RINGING"}`
	otherID := "b3d7a190-8503-4bf4-9c12-123456789abc"
	cases := map[string]string{
		"invalid-json":       `[{`,
		"root-null":          `null`,
		"root-number":        `1`,
		"root-string":        `"text"`,
		"root-object":        `{"wrapped":` + valid + `}`,
		"null-member":        `[` + valid + `,null]`,
		"string-member":      `["text",` + valid + `]`,
		"number-member":      `[` + valid + `,7]`,
		"array-member":       `[` + valid + `,[]]`,
		"missing-id":         `[` + valid + `,{"state":"ACTIVE"}]`,
		"missing-state":      `[` + valid + `,{"callId":"` + otherID + `"}]`,
		"non-string-state":   `[` + valid + `,{"callId":"` + otherID + `","state":7}]`,
		"unknown-state":      `[` + valid + `,{"callId":"` + otherID + `","state":"UNKNOWN"}]`,
		"non-canonical-id":   `[` + valid + `,{"callId":"invalid","state":"ACTIVE"}]`,
		"duplicate-call":     `[` + valid + `,` + valid + `]`,
		"null-action-policy": `[{"callId":"` + callID + `","state":"RINGING","remoteActionsAllowed":null}]`,
		"bad-action-policy":  `[{"callId":"` + callID + `","state":"RINGING","remoteActionsAllowed":"true"}]`,
		"too-large":          `[{"callId":"` + callID + `","state":"RINGING","extra":"` + strings.Repeat("x", 4096) + `"}]`,
	}
	var tooMany []map[string]string
	for i := 0; i < 9; i++ {
		id := callID
		if i > 0 {
			id = fmt.Sprintf("00000000-0000-4000-8000-%012d", i)
		}
		tooMany = append(tooMany, map[string]string{"callId": id, "state": "RINGING"})
	}
	encoded, err := json.Marshal(tooMany)
	if err != nil {
		t.Fatal(err)
	}
	cases["too-many-members"] = string(encoded)
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			db := openPushDB(t)
			now := time.Now().UTC()
			seedParentAndFamily(t, db, "owner", "family", now.Add(time.Hour))
			store := newPushStore(t, db, &pushClock{now: now})
			seedFeedEvent(t, db, "owner", "device", callID, now, "RINGING")
			if _, err := db.Exec(`UPDATE remote_call_snapshots SET calls_json=? WHERE device_id='device'`, raw); err != nil {
				t.Fatal(err)
			}
			feed, err := store.CallEvents(context.Background(), "owner", "family")
			if err != nil || len(feed.Events) != 0 {
				t.Fatal("malformed snapshot qualified or caused a query error")
			}
		})
	}
}

func TestParentCallFeedClockRollbackAndObservationBounds(t *testing.T) {
	for _, name := range []string{"clock-rollback", "latest-before-first-ring", "latest-future", "first-ring-future", "old-first-ring", "bad-first-ring-time", "bad-latest-time"} {
		t.Run(name, func(t *testing.T) {
			db := openPushDB(t)
			now := time.Now().UTC()
			seedParentAndFamily(t, db, "owner", "family", now.Add(time.Hour))
			clock := &pushClock{now: now}
			store := newPushStore(t, db, clock)
			seedFeedEvent(t, db, "owner", "device", "a3d7a190-8503-4bf4-9c12-123456789abc", now, "RINGING")
			var mutation string
			var value any
			switch name {
			case "clock-rollback":
				clock.now = now.Add(-time.Second)
			case "latest-before-first-ring":
				mutation = `UPDATE remote_call_snapshots SET observed_at=?,snapshot_sequence=2 WHERE device_id='device'`
				value = now.Add(-time.Second).Format(time.RFC3339Nano)
			case "latest-future":
				mutation = `UPDATE remote_call_snapshots SET observed_at=?,snapshot_sequence=2 WHERE device_id='device'`
				value = now.Add(time.Second).Format(time.RFC3339Nano)
			case "first-ring-future":
				mutation = `UPDATE parent_call_events SET observed_at=?`
				value = now.Add(time.Second).Format(time.RFC3339Nano)
			case "old-first-ring":
				mutation = `UPDATE parent_call_events SET observed_at=?`
				value = now.Add(-31 * time.Second).Format(time.RFC3339Nano)
			case "bad-first-ring-time":
				mutation = `UPDATE parent_call_events SET observed_at=?`
				value = "invalid"
			case "bad-latest-time":
				mutation = `UPDATE remote_call_snapshots SET observed_at=?`
				value = "invalid"
			}
			if mutation != "" {
				if _, err := db.Exec(mutation, value); err != nil {
					t.Fatal(err)
				}
			}
			feed, err := store.CallEvents(context.Background(), "owner", "family")
			if err != nil || len(feed.Events) != 0 {
				t.Fatal("rollback, future or malformed observation qualified")
			}
		})
	}
}
