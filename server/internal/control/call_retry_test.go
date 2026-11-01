package control

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Parent retry recovery relies on these errors being returned only when the
// request key has no previously committed command, even after the call changes.
func TestCallStateRejectionsHaveNoCommittedRequestAndReplaysSurviveStateChanges(t *testing.T) {
	db, _ := fixture(t)
	s := NewService(db)
	s.SetRemoteCallsEnabled(true)
	ctx := context.Background()
	call := "a3d7a190-8503-4bf4-9c12-123456789abc"
	if _, err := s.QueueCallActionWithKey(ctx, "owner", "device", "ANSWER_CALL", call, 20, "missing-call-key-0001"); !errors.Is(err, ErrCallUnavailable) {
		t.Fatalf("missing call: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO remote_call_snapshots(device_id,parent_account_id,observed_at,calls_json,snapshot_sequence)
		VALUES('device','owner',?,?,1)`, time.Now().UTC().Format(time.RFC3339Nano),
		`[{"callId":"`+call+`","state":"ACTIVE","remoteActionsAllowed":true}]`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.QueueCallActionWithKey(ctx, "owner", "device", "ANSWER_CALL", call, 20, "state-conflict-key-01"); !errors.Is(err, ErrCallState) {
		t.Fatalf("wrong state: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM commands`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("a rejected call request was persisted: count=%d err=%v", count, err)
	}
	original, err := s.QueueCallActionWithKey(ctx, "owner", "device", "END_CALL", call, 20, "committed-call-key-01")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE remote_call_snapshots SET calls_json='[]' WHERE device_id='device'`); err != nil {
		t.Fatal(err)
	}
	replay, err := s.QueueCallActionWithKey(ctx, "owner", "device", "END_CALL", call, 20, "committed-call-key-01")
	if err != nil || replay.ID != original.ID {
		t.Fatalf("committed request lost after call removal: %v", err)
	}
	s.SetRemoteCallsEnabled(false)
	if _, err := s.QueueCallActionWithKey(ctx, "owner", "device", "END_CALL", call, 20, "committed-call-key-01"); !errors.Is(err, ErrRemoteCallDisabled) {
		t.Fatalf("disabled rollout: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM commands WHERE id=?`, original.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("disabled rollout lost a previously committed request")
	}
}
