package control

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"appcontrol/server/internal/audit"
	"appcontrol/server/internal/storage"
)

func fixture(t *testing.T) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "controls.db")
	db, err := storage.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, statement := range []string{
		`INSERT INTO parent_accounts(id,email,password_hash,created_at,updated_at) VALUES('owner','owner@example.com','unused','2026-10-23T00:00:00Z','2026-10-23T00:00:00Z')`,
		`INSERT INTO devices(id,parent_account_id,display_name,public_key_der,key_algorithm,state,created_at) VALUES('device','owner','private-name',X'00','ECDSA_P256_SHA256','active','2026-10-23T00:00:00Z')`,
	} {
		if _, err = db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	return db, path
}

func TestControlIntentsAreAuditedDurableAndIsolated(t *testing.T) {
	ctx := context.Background()
	db, path := fixture(t)
	service := NewService(db)
	command, err := service.QueueLock(ctx, "owner", "device", 60)
	if err != nil {
		t.Fatal(err)
	}

	if command.Version != 1 || command.Type != "LOCK_DEVICE" || command.Status != "queued" || string(command.Payload) != "{}" || command.ExpiresAt.Sub(command.IssuedAt).Seconds() != 60 {
		t.Fatal("wrong command envelope")
	}
	policy, err := service.UpdatePolicy(ctx, "owner", "device", 0, true, []string{"org.example.privateapp"})
	if err != nil || policy.Version != 1 {
		t.Fatalf("policy: %v %v", policy, err)
	}
	if _, err = service.UpdatePolicy(ctx, "owner", "device", 0, false, nil); !errors.Is(err, ErrConflict) {
		t.Fatal("stale policy accepted")
	}
	if _, err = service.QueueLock(ctx, "other", "device", 60); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-parent command accepted")
	}
	if _, err = service.UpdatePolicy(ctx, "other", "device", 1, false, nil); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-parent policy accepted")
	}
	policy, err = service.UpdatePolicy(ctx, "owner", "device", 1, false, nil)
	if err != nil || policy.Version != 2 || policy.BlockedPackages == nil {
		t.Fatal("policy reset failed")
	}
	events, err := audit.List(ctx, db, "owner", 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, event := range events {
		counts[event.Action]++
		if event.Action == "command_created" || event.Action == "policy_updated" {
			if event.ActorType != "parent" || event.ActorID != "owner" || event.OccurredAt == "" {
				t.Fatal("wrong audit attribution")
			}
			if strings.Contains(event.TargetID, "privateapp") {
				t.Fatal("policy content in audit")
			}
		}
	}
	if counts["command_created"] != 1 || counts["policy_updated"] != 2 {
		t.Fatalf("unexpected events: %v", counts)
	}
	if _, err = db.Exec("UPDATE devices SET state='revoked',revoked_at='2026-10-23T00:00:00Z',revocation_reason='parent_revoked'"); err != nil {
		t.Fatal(err)
	}
	if _, err = service.QueueLock(ctx, "owner", "device", 60); !errors.Is(err, ErrNotFound) {
		t.Fatal("revoked device commanded")
	}
	if _, err = service.UpdatePolicy(ctx, "owner", "device", 2, false, nil); !errors.Is(err, ErrNotFound) {
		t.Fatal("revoked device policy updated")
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var status string
	if err = reopened.QueryRow("SELECT status FROM commands WHERE id=?", command.ID).Scan(&status); err != nil || status != "queued" {
		t.Fatal("queued command lost on restart")
	}
	var version int
	if err = reopened.QueryRow("SELECT version FROM device_policies WHERE device_id='device'").Scan(&version); err != nil || version != 2 {
		t.Fatal("policy lost on restart")
	}
	persisted, err := audit.List(ctx, reopened, "owner", 0, 100)
	if err != nil || len(persisted) != len(events)+1 {
		t.Fatal("audit history lost on restart")
	}
}

func TestAuditFailureRollsBackControlIntents(t *testing.T) {
	ctx := context.Background()
	db, _ := fixture(t)
	s := NewService(db)
	if _, err := s.UpdatePolicy(ctx, "owner", "device", 0, false, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER test_audit_failure BEFORE INSERT ON audit_events BEGIN SELECT RAISE(ABORT,'injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.QueueLock(ctx, "owner", "device", 60); err == nil {
		t.Fatal("unaudited command accepted")
	}
	if _, err := s.UpdatePolicy(ctx, "owner", "device", 1, true, []string{"org.example.app"}); err == nil {
		t.Fatal("unaudited policy accepted")
	}
	var count, version, paused int
	if err := db.QueryRow("SELECT COUNT(*) FROM commands").Scan(&count); err != nil || count != 0 {
		t.Fatal("failed command persisted")
	}
	if err := db.QueryRow("SELECT version,internet_paused FROM device_policies").Scan(&version, &paused); err != nil || version != 1 || paused != 0 {
		t.Fatal("failed policy update persisted")
	}
}

func TestPolicyConcurrentUpdatesHaveOneWinner(t *testing.T) {
	ctx := context.Background()
	db, _ := fixture(t)
	s := NewService(db)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.UpdatePolicy(ctx, "owner", "device", 0, true, nil)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("lost concurrency protection: %d %d", successes, conflicts)
	}
}

func TestControlValidation(t *testing.T) {
	db, _ := fixture(t)
	s := NewService(db)
	ctx := context.Background()
	for _, ttl := range []int{-1, 0, 3601} {
		if _, err := s.QueueLock(ctx, "owner", "device", ttl); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid expiry accepted")
		}
	}
	for _, pkgs := range [][]string{{"com.appcontrol.child"}, {"com.android.phone"}, {"com.google.android.gms"}, {"bad package"}, {".org.example"}, {"org..example"}, {"org.9example"}, {"org.example.app", "org.example.app"}} {
		if _, err := s.UpdatePolicy(ctx, "owner", "device", 0, false, pkgs); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid or protected package accepted")
		}
	}
}

func TestCommandCreationIdempotencySurvivesRetryAndExpiry(t *testing.T) {
	db, _ := fixture(t)
	s := NewService(db)
	ctx := context.Background()
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	const key = "test-request-key-0001"
	first, err := s.QueueLockWithKey(ctx, "owner", "device", 1, key)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.QueueLockWithKey(ctx, "owner", "device", 1, key)
	if err != nil || retry.ID != first.ID || retry.IssuedAt != first.IssuedAt {
		t.Fatal("retry created a different command")
	}
	if _, err = s.QueueLockWithKey(ctx, "owner", "device", 60, key); !errors.Is(err, ErrIdempotency) {
		t.Fatal("changed input accepted with same key")
	}
	now = now.Add(time.Second)
	expired, err := s.QueueLockWithKey(ctx, "owner", "device", 1, key)
	if err != nil || expired.ID != first.ID || expired.Status != "expired" {
		t.Fatal("expired retry recreated command")
	}
	var count int
	if err = db.QueryRow("SELECT COUNT(*) FROM commands").Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate queue entry")
	}
	if err = db.QueryRow("SELECT COUNT(*) FROM audit_events WHERE action='command_created'").Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate creation audit")
	}
	for _, invalid := range []string{"short", strings.Repeat("a", 129), "invalid key with spaces"} {
		if _, err = s.QueueLockWithKey(ctx, "owner", "device", 60, invalid); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid idempotency key accepted")
		}
	}
}

func TestConcurrentCommandCreationDeduplicates(t *testing.T) {
	db, _ := fixture(t)
	s := NewService(db)
	ctx := context.Background()
	ids := make(chan string, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := s.QueueLockWithKey(ctx, "owner", "device", 60, "concurrent-request-0001")
			ids <- c.ID
			errs <- err
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first string
	for id := range ids {
		if first == "" {
			first = id
		} else if first != id {
			t.Fatal("concurrent retries created distinct commands")
		}
	}
}

type wakeupFunc func(context.Context, string) error

func (f wakeupFunc) Wake(ctx context.Context, deviceID string) error { return f(ctx, deviceID) }

func TestWakeupRunsAfterCommitAndFailureDoesNotLoseCommand(t *testing.T) {
	db, _ := fixture(t)
	ctx := context.Background()
	calls := 0
	s := NewServiceWithWakeup(db, wakeupFunc(func(ctx context.Context, deviceID string) error {
		calls++
		var status string
		if err := db.QueryRowContext(ctx, "SELECT status FROM commands WHERE device_id=?", deviceID).Scan(&status); err != nil || status != "queued" {
			t.Fatal("wake-up preceded durable commit")
		}
		return errors.New("provider unavailable")
	}))
	first, err := s.QueueLockWithKey(ctx, "owner", "device", 60, "wakeup-request-key-0001")
	if err != nil {
		t.Fatal("provider failure failed committed creation")
	}
	retry, err := s.QueueLockWithKey(ctx, "owner", "device", 60, "wakeup-request-key-0001")
	if err != nil || retry.ID != first.ID || calls != 1 {
		t.Fatal("creation retry repeated wake-up or lost command")
	}
	if _, err = db.Exec(`CREATE TRIGGER test_wakeup_audit_failure BEFORE INSERT ON audit_events BEGIN SELECT RAISE(ABORT,'injected'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.QueueLock(ctx, "owner", "device", 60); err == nil || calls != 1 {
		t.Fatal("failed creation triggered wake-up")
	}
}

func TestPolicyWakeupRunsAfterCommit(t *testing.T) {
	db, _ := fixture(t)
	calls := 0
	ctx := context.Background()
	s := NewServiceWithWakeup(db, wakeupFunc(func(ctx context.Context, deviceID string) error {
		calls++
		var version int
		if err := db.QueryRowContext(ctx, "SELECT version FROM device_policies WHERE device_id=?", deviceID).Scan(&version); err != nil || version != calls {
			t.Fatal("policy wake-up preceded commit")
		}

		return errors.New("provider unavailable")
	}))
	if _, err := s.UpdatePolicy(ctx, "owner", "device", 0, true, nil); err != nil {
		t.Fatal("hint failure undid policy")
	}
	if _, err := s.UpdatePolicy(ctx, "owner", "device", 1, false, nil); err != nil || calls != 2 {
		t.Fatal("policy hint missing")
	}
	if _, err := s.UpdatePolicy(ctx, "owner", "device", 1, false, nil); !errors.Is(err, ErrConflict) || calls != 2 {
		t.Fatal("failed policy update triggered wake-up")
	}
}

func TestCallCommandsAreBoundedExactAndParentScoped(t *testing.T) {
	db, _ := fixture(t)
	s := NewService(db)
	s.SetRemoteCallsEnabled(true)
	now := time.Date(2026, 10, 24, 12, 0, 0, 123456789, time.UTC)
	s.now = func() time.Time { return now }
	ctx := context.Background()
	callID := "a3d7a190-8503-4bf4-9c12-123456789abc"
	disabled := NewService(db)
	if _, err := disabled.QueueCallActionWithKey(ctx, "owner", "device", "ANSWER_CALL", callID, 10, ""); !errors.Is(err, ErrRemoteCallDisabled) {
		t.Fatal("call command accepted while remote-call feature is disabled")
	}
	if _, err := disabled.QueueLock(ctx, "owner", "device", 60); err != nil {
		t.Fatal("disabling remote calls also disabled lock controls")
	}
	if _, err := db.Exec(`INSERT INTO remote_call_snapshots(device_id,parent_account_id,observed_at,calls_json,snapshot_sequence)
		VALUES('device','owner',?,?,1)`, now.Format(time.RFC3339Nano), `[{"callId":"`+callID+`","state":"RINGING","remoteActionsAllowed":true}]`); err != nil {
		t.Fatal(err)
	}

	command, err := s.QueueCallActionWithKey(ctx, "owner", "device", "ANSWER_CALL", callID, 30, "call-request-key-001")
	if err != nil {
		t.Fatal(err)
	}
	if command.Type != "ANSWER_CALL" || string(command.Payload) != `{"callId":"`+callID+`"}` ||
		command.ExpiresAt.Sub(command.IssuedAt) != 30*time.Second || command.Status != "queued" {
		t.Fatalf("invalid call command envelope: %+v", command)
	}
	retry, err := s.QueueCallActionWithKey(ctx, "owner", "device", "ANSWER_CALL", callID, 30, "call-request-key-001")
	if err != nil || retry.ID != command.ID {
		t.Fatal("identical call command retry was not idempotent")
	}
	if _, err = s.QueueCallActionWithKey(ctx, "owner", "device", "END_CALL", callID, 30, "call-request-key-001"); !errors.Is(err, ErrIdempotency) {
		t.Fatal("idempotency key reused for another action")
	}
	for _, ttl := range []int{0, 31} {
		if _, err = s.QueueCallActionWithKey(ctx, "owner", "device", "END_CALL", callID, ttl, ""); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid TTL %d accepted", ttl)
		}
	}
	for _, invalidID := range []string{"", "not-a-call", "A3D7A190-8503-4BF4-9C12-123456789ABC"} {
		if _, err = s.QueueCallActionWithKey(ctx, "owner", "device", "END_CALL", invalidID, 5, ""); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid call ID accepted: %q", invalidID)
		}
	}
	if _, err = s.QueueCallActionWithKey(ctx, "owner", "device", "LOCK_DEVICE", callID, 5, ""); !errors.Is(err, ErrInvalid) {
		t.Fatal("unsupported call action accepted")
	}
	if _, err = s.QueueCallActionWithKey(ctx, "other", "device", "REJECT_CALL", callID, 5, ""); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-parent call command accepted")
	}
	if _, err = s.QueueCallActionWithKey(ctx, "owner", "device", "END_CALL", callID, 5, ""); !errors.Is(err, ErrCallState) {
		t.Fatal("end action accepted for a ringing call")
	}
	if _, err = db.Exec(`UPDATE remote_call_snapshots SET calls_json=? WHERE device_id='device'`,
		`[{"callId":"`+callID+`","state":"RINGING","remoteActionsAllowed":false}]`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.QueueCallActionWithKey(ctx, "owner", "device", "ANSWER_CALL", callID, 5, "permission-check-key"); !errors.Is(err, ErrRemoteCallDisabled) {
		t.Fatal("call command accepted when snapshot denied remote actions")
	}
	if _, err = s.QueueCallActionWithKey(ctx, "owner", "device", "ANSWER_CALL", "b3d7a190-8503-4bf4-9c12-123456789abc", 5, ""); !errors.Is(err, ErrCallUnavailable) {
		t.Fatal("unreported call accepted")
	}
	now = now.Add(31 * time.Second)
	if _, err = s.QueueCallActionWithKey(ctx, "owner", "device", "ANSWER_CALL", callID, 5, ""); !errors.Is(err, ErrCallUnavailable) {
		t.Fatal("stale call snapshot accepted")
	}
	expired, err := s.Command(ctx, "owner", "device", command.ID)
	if err != nil || expired.Status != "expired" {
		t.Fatal("call command remained actionable after deadline")
	}
}
