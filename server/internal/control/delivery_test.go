package control

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
)

func session(t *testing.T, db *sql.DB, deviceID, character string, now time.Time) string {
	t.Helper()
	token := "dt_" + strings.Repeat(character, 43)
	hash := sha256.Sum256([]byte(token))
	if _, err := db.Exec("INSERT INTO device_sessions(token_hash,device_id,expires_at,issued_at) VALUES(?,?,?,?)", hash[:], deviceID, now.Add(time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	return token
}

func TestDeliveryRequiresAckAndDeduplicatesResults(t *testing.T) {
	db, _ := fixture(t)
	s := NewService(db)
	ctx := context.Background()
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	token := session(t, db, "device", "A", now)
	c, err := s.QueueLock(ctx, "owner", "device", 60)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Acknowledge(ctx, token, c.ID, "succeeded"); !errors.Is(err, ErrCommandState) {
		t.Fatal("undelivered command acknowledged")
	}
	for i := 0; i < 2; i++ {
		fetched, err := s.Fetch(ctx, token)
		if err != nil || len(fetched) != 1 || fetched[0].ID != c.ID || fetched[0].Status != "delivered" {
			t.Fatalf("fetch/retry: %v %v", fetched, err)
		}
	}
	pending, err := s.Command(ctx, "owner", "device", c.ID)
	if err != nil || pending.Status != "delivered" {
		t.Fatal("delivery incorrectly implied success")
	}
	for i := 0; i < 2; i++ {
		if err = s.Acknowledge(ctx, token, c.ID, "succeeded"); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.Acknowledge(ctx, token, c.ID, "failed"); !errors.Is(err, ErrCommandState) {
		t.Fatal("conflicting terminal result accepted")
	}
	commands, err := s.Fetch(ctx, token)
	if err != nil || len(commands) != 0 {
		t.Fatal("completed command redelivered")
	}
	final, err := s.Command(ctx, "owner", "device", c.ID)
	if err != nil || final.Status != "succeeded" {
		t.Fatal("acknowledgement lost")
	}
	var delivered, succeeded int
	if err = db.QueryRow("SELECT COUNT(*) FROM audit_events WHERE target_id=? AND action='command_delivered'", c.ID).Scan(&delivered); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow("SELECT COUNT(*) FROM audit_events WHERE target_id=? AND action='command_succeeded'", c.ID).Scan(&succeeded); err != nil || delivered != 1 || succeeded != 1 {
		t.Fatal("duplicate status audit")
	}
	if _, err = db.Exec("UPDATE commands SET status='delivered' WHERE id=?", c.ID); err == nil {
		t.Fatal("terminal command reopened")
	}
}

func TestDisabledRemoteCallsExpirePendingCallActionsButPreserveLocks(t *testing.T) {
	db, _ := fixture(t)
	s := NewService(db)
	s.SetRemoteCallsEnabled(true)
	ctx := context.Background()
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	token := session(t, db, "device", "A", now)
	callID := "a3d7a190-8503-4bf4-9c12-123456789abc"
	if _, err := db.Exec(`INSERT INTO remote_call_snapshots(device_id,parent_account_id,observed_at,calls_json,snapshot_sequence)
		VALUES('device','owner',?,?,1)`, now.Format(time.RFC3339Nano),
		`[{"callId":"`+callID+`","state":"RINGING","remoteActionsAllowed":true}]`); err != nil {
		t.Fatal(err)
	}
	call, err := s.QueueCallAction(ctx, "owner", "device", "ANSWER_CALL", callID, 20)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := s.QueueLock(ctx, "owner", "device", 60)
	if err != nil {
		t.Fatal(err)
	}
	s.SetRemoteCallsEnabled(false)
	commands, err := s.Fetch(ctx, token)
	if err != nil || len(commands) != 1 || commands[0].ID != lock.ID || commands[0].Type != "LOCK_DEVICE" {
		t.Fatalf("disabled call action was delivered or lock was lost: %v %v", commands, err)
	}
	result, err := s.Command(ctx, "owner", "device", call.ID)
	if err != nil || result.Status != "expired" {
		t.Fatal("pending call command was not retired when feature disabled")
	}
}

func TestExpiryRejectsLateResultsAndUpdatesOfflineParent(t *testing.T) {
	db, _ := fixture(t)
	s := NewService(db)
	ctx := context.Background()
	now := time.Date(2026, 10, 23, 12, 0, 0, 123456789, time.UTC)
	s.now = func() time.Time { return now }
	token := session(t, db, "device", "A", now)
	queued, err := s.QueueLock(ctx, "owner", "device", 10)
	if err != nil {
		t.Fatal(err)
	}
	delivered, err := s.QueueLock(ctx, "owner", "device", 10)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Fetch(ctx, token); err != nil {
		t.Fatal(err)
	}
	// This third command remains offline, with no delivery attempt.
	offline, err := s.QueueLock(ctx, "owner", "device", 10)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(10 * time.Second)
	parent, err := s.Command(ctx, "owner", "device", offline.ID)
	if err != nil || parent.Status != "expired" {
		t.Fatal("offline parent still sees pending after expiry")
	}
	if err = s.Acknowledge(ctx, token, delivered.ID, "succeeded"); !errors.Is(err, ErrCommandState) {
		t.Fatal("late success accepted")
	}
	fetched, err := s.Fetch(ctx, token)
	if err != nil || len(fetched) != 0 {
		t.Fatal("expired command delivered")
	}
	for _, id := range []string{queued.ID, delivered.ID, offline.ID} {
		var count int
		if err = db.QueryRow("SELECT COUNT(*) FROM audit_events WHERE target_id=? AND action='command_expired'", id).Scan(&count); err != nil || count != 1 {
			t.Fatal("expiry audit missing or duplicated")
		}
	}
}

func TestDeliveryAuthorizationAndAuditFailure(t *testing.T) {
	db, _ := fixture(t)
	s := NewService(db)
	ctx := context.Background()
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	token := session(t, db, "device", "A", now)
	c, err := s.QueueLock(ctx, "owner", "device", 60)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Fetch(ctx, "bad"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("invalid token accepted")
	}
	if _, err = db.Exec(`INSERT INTO devices(id,parent_account_id,display_name,public_key_der,key_algorithm,state,created_at) VALUES('second','owner','second',X'00','ECDSA_P256_SHA256','active','2026-10-23T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	other := session(t, db, "second", "B", now)
	if commands, err := s.Fetch(ctx, other); err != nil || len(commands) != 0 {
		t.Fatal("another device fetched command")
	}
	if err = s.Acknowledge(ctx, other, c.ID, "succeeded"); !errors.Is(err, ErrNotFound) {
		t.Fatal("another device acknowledged command")
	}
	if _, err = db.Exec(`CREATE TRIGGER test_delivery_audit_failure BEFORE INSERT ON audit_events WHEN NEW.action='command_delivered' BEGIN SELECT RAISE(ABORT,'injected'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Fetch(ctx, token); err == nil {
		t.Fatal("unaudited delivery succeeded")
	}
	var status string
	if err = db.QueryRow("SELECT status FROM commands WHERE id=?", c.ID).Scan(&status); err != nil || status != "queued" {
		t.Fatal("delivery audit failure did not roll back")
	}
	if _, err = db.Exec("DROP TRIGGER test_delivery_audit_failure"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Fetch(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TRIGGER test_ack_audit_failure BEFORE INSERT ON audit_events WHEN NEW.action='command_succeeded' BEGIN SELECT RAISE(ABORT,'injected'); END`); err != nil {
		t.Fatal(err)
	}
	if err = s.Acknowledge(ctx, token, c.ID, "succeeded"); err == nil {
		t.Fatal("unaudited acknowledgement succeeded")
	}
	if _, err = db.Exec("UPDATE devices SET state='revoked',revoked_at='2026-10-23T00:00:00Z',revocation_reason='parent_revoked' WHERE id='device'"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Fetch(ctx, token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("revoked device fetched")
	}
	if err = s.Acknowledge(ctx, token, c.ID, "succeeded"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("revoked device acknowledged")
	}
	now = now.Add(2 * time.Hour)
	if _, err = s.Fetch(ctx, other); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("expired session accepted")
	}
}

func TestOfflinePolicyFetchReturnsLatestAndChecksRevocation(t *testing.T) {
	db, _ := fixture(t)
	s := NewService(db)
	ctx := context.Background()
	now := time.Now().UTC()
	token := session(t, db, "device", "A", now)
	empty, err := s.DevicePolicy(ctx, token)
	if err != nil || empty.Version != 0 || empty.BlockedPackages == nil {
		t.Fatal("invalid initial policy snapshot")
	}
	if _, err = s.UpdatePolicy(ctx, "owner", "device", 0, true, []string{"org.example.first"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.UpdatePolicy(ctx, "owner", "device", 1, false, []string{"org.example.latest"}); err != nil {
		t.Fatal(err)
	}
	latest, err := s.DevicePolicy(ctx, token)
	if err != nil || latest.Version != 2 || latest.InternetPaused || len(latest.BlockedPackages) != 1 || latest.BlockedPackages[0] != "org.example.latest" {
		t.Fatal("reconnect returned stale policy")
	}
	if _, err = s.DevicePolicy(ctx, "invalid"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("unauthenticated policy read accepted")
	}
	if _, err = db.Exec("DELETE FROM device_sessions WHERE device_id='device'"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DevicePolicy(ctx, token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("replaced session read policy")
	}
}
