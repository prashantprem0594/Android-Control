package parentpush

import (
	"appcontrol/server/internal/auth"
	"appcontrol/server/internal/control"
	"appcontrol/server/internal/storage"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type pushClock struct{ now time.Time }

func (c *pushClock) Now() time.Time { return c.now }

func openPushDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "parent-push.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func seedParentAndFamily(t *testing.T, db *sql.DB, parentID, familyID string, expiry time.Time) {
	t.Helper()
	for _, query := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO parent_accounts(id,email,password_hash,created_at,updated_at) VALUES(?,?, 'unused',?,?)`,
			[]any{parentID, parentID + "@example.com", time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano)}},
		{`INSERT INTO auth_session_families(id,parent_account_id,created_at,expires_at) VALUES(?,?,?,?)`,
			[]any{familyID, parentID, time.Now().UTC().Format(time.RFC3339Nano), expiry.UTC().Format(time.RFC3339Nano)}},
	} {
		if _, err := db.Exec(query.sql, query.args...); err != nil {
			t.Fatal(err)
		}
	}
}

func newPushStore(t *testing.T, db *sql.DB, clock *pushClock) *Store {
	t.Helper()
	store, err := NewStore(db, bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	if clock != nil {
		store.now = clock.Now
	}
	return store
}

func countRows(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var count int
	if err := db.QueryRow(query, args...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestRegistrationEncryptsBindsRotatesAndRejectsTampering(t *testing.T) {
	ctx := context.Background()
	db := openPushDB(t)
	now := time.Now().UTC()
	seedParentAndFamily(t, db, "parent-a", "family-a", now.Add(time.Hour))
	seedParentAndFamily(t, db, "parent-b", "family-b", now.Add(time.Hour))
	store := newPushStore(t, db, &pushClock{now: now})

	if err := store.Register(ctx, "parent-b", "family-a", "foreign-registration"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("cross-account family registration accepted")
	}
	for _, invalid := range []string{"", "has space", "bad\nvalue", strings.Repeat("x", 4097)} {
		if err := store.Register(ctx, "parent-a", "family-a", invalid); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid registration token accepted")
		}
	}
	first := "private-routing-fixture-1"
	if err := store.Register(ctx, "parent-a", "family-a", first); err != nil {
		t.Fatal(err)
	}
	var sealed, firstCiphertext []byte
	var version int
	if err := db.QueryRow(`SELECT token_ciphertext,registration_version FROM parent_wakeup_registrations
		WHERE family_id='family-a'`).Scan(&sealed, &version); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte(first)) || version != 1 {
		t.Fatal("routing token not encrypted or initial version incorrect")
	}
	firstCiphertext = append([]byte(nil), sealed...)
	if err := store.Register(ctx, "parent-a", "family-a", "private-routing-fixture-2"); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT token_ciphertext,registration_version FROM parent_wakeup_registrations
		WHERE family_id='family-a'`).Scan(&sealed, &version); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(firstCiphertext, sealed) || version != 2 {
		t.Fatal("rotation did not replace ciphertext and advance version")
	}
	if bytes.Equal(registrationAAD("parent-a", "family-a"), registrationAAD("parent-b", "family-a")) ||
		bytes.Equal(registrationAAD("parent-a", "family-a"), registrationAAD("parent-a", "family-b")) {
		t.Fatal("registration associated data did not distinguish account and session family")
	}
	nonceSize := store.cipher.NonceSize()
	if _, err := store.cipher.Open(nil, firstCiphertext[:nonceSize], firstCiphertext[nonceSize:],
		registrationAAD("parent-b", "family-b")); err == nil {
		t.Fatal("ciphertext accepted for another account/family")
	}
	tampered := append([]byte(nil), sealed...)
	tampered[len(tampered)-1] ^= 0x80
	if _, err := store.cipher.Open(nil, tampered[:nonceSize], tampered[nonceSize:],
		registrationAAD("parent-a", "family-a")); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
	if err := store.Remove(ctx, "parent-a", "family-a"); err != nil {
		t.Fatal(err)
	}
	if err := store.Register(ctx, "parent-a", "family-a", "private-routing-fixture-3"); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT registration_version FROM parent_wakeup_registrations WHERE family_id='family-a'`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 3 {
		t.Fatal("remove and re-register reused an earlier registration version")
	}
	if removed, err := store.RemoveInvalidRegistration(ctx, "parent-a", "family-a", 2); err != nil || removed {
		t.Fatal("late send failure removed a re-registered token")
	}
}

func TestRegistrationRequiresUnexpiredFamilyAndConcurrentLogoutWins(t *testing.T) {
	ctx := context.Background()
	db := openPushDB(t)
	now := time.Now().UTC()
	clock := &pushClock{now: now}
	authentication := newAuthService(t, db, clock)
	parent, err := authentication.Register(ctx, "concurrent@example.com", "fixture-password")
	if err != nil {
		t.Fatal(err)
	}
	_, pair, err := authentication.Login(ctx, parent.Email, "fixture-password")
	if err != nil {
		t.Fatal(err)
	}
	_, familyID, err := authentication.Authenticate(ctx, pair.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	store := newPushStore(t, db, nil)

	var wg sync.WaitGroup
	start := make(chan struct{})
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 20; i++ {
			err := store.Register(ctx, parent.ID, familyID, "rotating-registration")
			if err != nil && !errors.Is(err, ErrUnauthorized) {
				t.Errorf("concurrent registration: %v", err)
			}
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		if err := authentication.Logout(ctx, pair.AccessToken); err != nil {
			t.Errorf("revoke family: %v", err)
		}
	}()
	close(start)
	wg.Wait()
	if countRows(t, db, "SELECT COUNT(*) FROM parent_wakeup_registrations WHERE family_id=?", familyID) != 0 {
		t.Fatal("registration survived concurrent logout")
	}
	if err := store.Register(ctx, parent.ID, familyID, "after-logout"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("revoked family registered again")
	}

	seedParentAndFamily(t, db, "parent-expired", "family-expired", now.Add(-time.Second))
	if err := store.Register(ctx, "parent-expired", "family-expired", "expired-registration"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("expired family registered")
	}
}

func newAuthService(t *testing.T, db *sql.DB, clock *pushClock) *auth.Service {
	t.Helper()
	service, err := auth.NewService(db, auth.Options{
		Now: clock.Now,
		PasswordParameters: auth.PasswordParameters{
			MemoryKiB: 8192, Iterations: 1, Parallelism: 1, SaltBytes: 16, KeyBytes: 32,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestRefreshContinuityAndRevocationCleanup(t *testing.T) {
	for _, scenario := range []string{"logout", "refresh_reuse", "operator_recovery"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			db := openPushDB(t)
			clock := &pushClock{now: time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)}
			authentication := newAuthService(t, db, clock)
			parent, err := authentication.Register(ctx, "parent-"+scenario+"@example.com", "fixture-password-1")
			if err != nil {
				t.Fatal(err)
			}
			_, pair, err := authentication.Login(ctx, parent.Email, "fixture-password-1")
			if err != nil {
				t.Fatal(err)
			}
			_, familyID, err := authentication.Authenticate(ctx, pair.AccessToken)
			if err != nil {
				t.Fatal(err)
			}
			store := newPushStore(t, db, clock)
			if err := store.Register(ctx, parent.ID, familyID, "private-family-token"); err != nil {
				t.Fatal(err)
			}

			rotated, err := authentication.Refresh(ctx, pair.RefreshToken)
			if err != nil {
				t.Fatal(err)
			}
			_, rotatedFamily, err := authentication.Authenticate(ctx, rotated.AccessToken)
			if err != nil || rotatedFamily != familyID || countRows(t, db,
				"SELECT COUNT(*) FROM parent_wakeup_registrations WHERE family_id=?", familyID) != 1 {
				t.Fatal("refresh did not preserve family registration")
			}
			switch scenario {
			case "logout":
				err = authentication.Logout(ctx, rotated.AccessToken)
			case "refresh_reuse":
				_, err = authentication.Refresh(ctx, pair.RefreshToken)
				if !errors.Is(err, auth.ErrRefreshReuse) {
					t.Fatal("refresh reuse was not detected")
				}
			case "operator_recovery":
				err = authentication.RecoverPassword(ctx, parent.Email, "fixture-password-2")
			}
			if scenario != "refresh_reuse" && err != nil {
				t.Fatal(err)
			}
			if countRows(t, db, "SELECT COUNT(*) FROM parent_wakeup_registrations WHERE family_id=?", familyID) != 0 {
				t.Fatal("session-family revocation did not delete push registration")
			}
		})
	}
}

func seedCallSession(t *testing.T, db *sql.DB, now time.Time) string {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO devices(id,parent_account_id,display_name,public_key_der,key_algorithm,state,created_at)
		VALUES('device','parent','fixture',X'00','ECDSA_P256_SHA256','active',?)`, now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	deviceToken := "dt_" + strings.Repeat("A", 43)
	hash := sha256.Sum256([]byte(deviceToken))
	if _, err := db.Exec(`INSERT INTO device_sessions(token_hash,device_id,expires_at,issued_at)
		VALUES(?,?,?,?)`, hash[:], "device", now.Add(time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	return deviceToken
}

func TestAcceptedCallSnapshotsCreateOneFreshExactCallEvent(t *testing.T) {
	ctx := context.Background()
	db := openPushDB(t)
	now := time.Now().UTC()
	seedParentAndFamily(t, db, "parent", "family", now.Add(time.Hour))
	deviceToken := seedCallSession(t, db, now)
	service := control.NewService(db)
	service.SetRemoteCallsEnabled(true)
	store := newPushStore(t, db, nil)
	if err := store.Register(ctx, "parent", "family", "private-parent-fcm-token"); err != nil {
		t.Fatal(err)
	}
	callID := "a3d7a190-8503-4bf4-9c12-123456789abc"

	firstSequence, err := service.AllocateRemoteCallSequence(ctx, deviceToken)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := service.ReportRemoteCalls(ctx, deviceToken, firstSequence, []control.RemoteCall{{CallID: callID, State: "RINGING"}})
	if err != nil || !accepted {
		t.Fatal("initial ringing snapshot rejected")
	}
	if countRows(t, db, "SELECT COUNT(*) FROM parent_call_events WHERE device_id='device' AND call_id=?", callID) != 1 {
		t.Fatal("accepted ringing snapshot did not create event")
	}
	claims, err := store.Claim(ctx, 10)
	if err != nil || len(claims) != 1 || claims[0].CallID != callID ||
		string(claims[0].RegistrationToken) != "private-parent-fcm-token" || claims[0].Attempt != 1 {
		t.Fatalf("eligible event was not leased: %v %v", claims, err)
	}
	claim := claims[0]
	clear(claim.RegistrationToken)
	if err := store.Finish(ctx, claim.ID, claim.LeaseID, false); err != nil {
		t.Fatal(err)
	}
	if err := store.Finish(ctx, claim.ID, claim.LeaseID, false); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("expired lease could be finished twice")
	}

	if accepted, err = service.ReportRemoteCalls(ctx, deviceToken, firstSequence, []control.RemoteCall{{CallID: callID, State: "RINGING"}}); err != nil || accepted {
		t.Fatal("equal snapshot accepted")
	}
	secondSequence, err := service.AllocateRemoteCallSequence(ctx, deviceToken)
	if err != nil {
		t.Fatal(err)
	}
	if accepted, err = service.ReportRemoteCalls(ctx, deviceToken, secondSequence, []control.RemoteCall{{CallID: callID, State: "RINGING"}}); err != nil || !accepted {
		t.Fatal("new repeated ringing snapshot rejected")
	}
	if accepted, err = service.ReportRemoteCalls(ctx, deviceToken, firstSequence, []control.RemoteCall{{CallID: callID, State: "DISCONNECTED"}}); err != nil || accepted {
		t.Fatal("stale snapshot accepted")
	}
	if countRows(t, db, "SELECT COUNT(*) FROM parent_call_events WHERE device_id='device' AND call_id=?", callID) != 1 {
		t.Fatal("duplicate exact-call alert event created")
	}

	thirdSequence, err := service.AllocateRemoteCallSequence(ctx, deviceToken)
	if err != nil {
		t.Fatal(err)
	}
	if accepted, err = service.ReportRemoteCalls(ctx, deviceToken, thirdSequence, []control.RemoteCall{}); err != nil || !accepted {
		t.Fatal("call end snapshot rejected")
	}
	if countRows(t, db, "SELECT COUNT(*) FROM parent_call_events WHERE device_id='device' AND call_id=? AND status='expired'", callID) != 1 {
		t.Fatal("ended call event remained eligible")
	}
	fourthSequence, err := service.AllocateRemoteCallSequence(ctx, deviceToken)
	if err != nil {
		t.Fatal(err)
	}
	if accepted, err = service.ReportRemoteCalls(ctx, deviceToken, fourthSequence, []control.RemoteCall{{CallID: callID, State: "RINGING"}}); err != nil || !accepted {
		t.Fatal("later call snapshot rejected")
	}
	if countRows(t, db, "SELECT COUNT(*) FROM parent_call_events WHERE device_id='device' AND call_id=?", callID) != 1 {
		t.Fatal("same call identity generated another alert event")
	}
	claims, err = store.Claim(ctx, 10)
	if err != nil || len(claims) != 0 {
		t.Fatal("expired exact-call event was reactivated")
	}
}

func TestCallEventRollbackExpiryOwnershipAndVersionedInvalidation(t *testing.T) {
	ctx := context.Background()
	db := openPushDB(t)
	now := time.Now().UTC()
	seedParentAndFamily(t, db, "parent", "family", now.Add(time.Hour))
	seedParentAndFamily(t, db, "other", "other-family", now.Add(time.Hour))
	deviceToken := seedCallSession(t, db, now)
	service := control.NewService(db)
	store := newPushStore(t, db, nil)
	if err := store.Register(ctx, "parent", "family", "private-token-one"); err != nil {
		t.Fatal(err)
	}
	callID := "a3d7a190-8503-4bf4-9c12-123456789abc"
	sequence, err := service.AllocateRemoteCallSequence(ctx, deviceToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER fail_parent_call_event BEFORE INSERT ON parent_call_events
		BEGIN SELECT RAISE(ABORT,'injected rollback'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReportRemoteCalls(ctx, deviceToken, sequence, []control.RemoteCall{{CallID: callID, State: "RINGING"}}); err == nil {
		t.Fatal("injected event failure did not roll back snapshot")
	}
	if countRows(t, db, "SELECT COUNT(*) FROM remote_call_snapshots WHERE device_id='device'") != 0 ||
		countRows(t, db, "SELECT COUNT(*) FROM parent_call_events") != 0 {
		t.Fatal("snapshot committed without its call event")
	}
	if _, err := db.Exec("DROP TRIGGER fail_parent_call_event"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReportRemoteCalls(ctx, deviceToken, sequence, []control.RemoteCall{{CallID: callID, State: "RINGING"}}); err != nil {
		t.Fatal(err)
	}

	claims, err := store.Claim(ctx, 10)
	if err != nil || len(claims) != 1 {
		t.Fatal("initial registration did not receive event")
	}
	clear(claims[0].RegistrationToken)
	if err := store.Finish(ctx, claims[0].ID, claims[0].LeaseID, false); err != nil {
		t.Fatal(err)
	}
	if err := store.Register(ctx, "parent", "family", "private-token-two"); err != nil {
		t.Fatal(err)
	}
	if removed, err := store.RemoveInvalidRegistration(ctx, "parent", "family", 1); err != nil || removed {
		t.Fatal("late send failure removed a rotated registration")
	}
	store.now = func() time.Time { return now.Add(2 * time.Second) }
	claims, err = store.Claim(ctx, 10)
	if err != nil || len(claims) != 1 || string(claims[0].RegistrationToken) != "private-token-two" {
		t.Fatal("rotated registration was not used for the event")
	}
	clear(claims[0].RegistrationToken)

	if _, err := db.Exec(`DROP TRIGGER revoke_parent_push_device_events`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE devices SET parent_account_id='other' WHERE id='device'`); err != nil {
		t.Fatal(err)
	}
	if err := store.Finish(ctx, claims[0].ID, claims[0].LeaseID, false); err != nil {
		t.Fatal(err)
	}
	clock := store.now().UTC()
	store.now = func() time.Time { return clock.Add(2 * time.Second) }
	claims, err = store.Claim(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(claims) != 0 {
		t.Fatal("event resolved after device owner changed")
	}
	if countRows(t, db, `SELECT COUNT(*) FROM parent_call_events WHERE call_id=? AND status='active'`, callID) != 1 {
		t.Fatal("owner resolution test did not retain active event")
	}

	if _, err := db.Exec(`UPDATE devices SET parent_account_id='parent' WHERE id='device'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE parent_call_events SET expires_at_ns=? WHERE call_id=?`,
		now.Add(2*time.Minute).UnixNano(), callID); err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return now.Add(31 * time.Second) }
	if claims, err = store.Claim(ctx, 10); err != nil || len(claims) != 0 {
		t.Fatal("stale call event was claimable")
	}
	if countRows(t, db, `SELECT COUNT(*) FROM parent_call_events WHERE call_id=? AND status='expired'`, callID) != 1 {
		t.Fatal("stale call event was not expired")
	}

	// A family can expire after registration; it must not accept another token.
	if _, err := db.Exec(`UPDATE auth_session_families SET expires_at=? WHERE id='family'`,
		now.Add(-time.Second).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if err := store.Register(ctx, "parent", "family", "expired-token"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("expired session family accepted registration")
	}
}

func TestClaimExpiresSessionFamilyAndStopsAfterRetryBudget(t *testing.T) {
	ctx := context.Background()
	db := openPushDB(t)
	now := time.Now().UTC()
	seedParentAndFamily(t, db, "parent", "family", now.Add(time.Hour))
	deviceToken := seedCallSession(t, db, now)
	service := control.NewService(db)
	store := newPushStore(t, db, nil)
	if err := store.Register(ctx, "parent", "family", "private-retry-token"); err != nil {
		t.Fatal(err)
	}
	sequence, err := service.AllocateRemoteCallSequence(ctx, deviceToken)
	if err != nil {
		t.Fatal(err)
	}
	callID := "a3d7a190-8503-4bf4-9c12-123456789abc"
	if _, err := service.ReportRemoteCalls(ctx, deviceToken, sequence, []control.RemoteCall{{CallID: callID, State: "RINGING"}}); err != nil {
		t.Fatal(err)
	}

	base := time.Now().UTC()
	elapsed := time.Duration(0)
	store.now = func() time.Time { return base.Add(elapsed) }
	delays := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		claimed, err := store.Claim(ctx, 1)
		if err != nil || len(claimed) != 1 || claimed[0].Attempt != attempt {
			t.Fatalf("attempt %d not claimed: %v %v", attempt, claimed, err)
		}
		item := claimed[0]
		clear(item.RegistrationToken)
		if err := store.Finish(ctx, item.ID, item.LeaseID, false); err != nil {
			t.Fatalf("finish attempt %d: %v", attempt, err)
		}
		if attempt <= len(delays) {
			elapsed += delays[attempt-1]
		}
	}
	if claimed, err := store.Claim(ctx, 1); err != nil || len(claimed) != 0 {
		t.Fatal("retry budget was not terminal")
	}
	if countRows(t, db, `SELECT COUNT(*) FROM parent_call_event_deliveries WHERE status='failed'`) != 1 {
		t.Fatal("exhausted delivery did not enter terminal failure")
	}

	// Expiry is checked independently of the app registration row's retention.
	if _, err := db.Exec(`UPDATE auth_session_families SET expires_at=? WHERE id='family'`,
		base.Add(-time.Second).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if err := store.Register(ctx, "parent", "family", "expired-token"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("expired family accepted another registration")
	}
}

func TestClaimFailsClosedOnCiphertextTampering(t *testing.T) {
	ctx := context.Background()
	db := openPushDB(t)
	now := time.Now().UTC()
	seedParentAndFamily(t, db, "parent", "family", now.Add(time.Hour))
	deviceToken := seedCallSession(t, db, now)
	service := control.NewService(db)
	store := newPushStore(t, db, nil)
	if err := store.Register(ctx, "parent", "family", "private-tamper-token"); err != nil {
		t.Fatal(err)
	}
	sequence, err := service.AllocateRemoteCallSequence(ctx, deviceToken)
	if err != nil {
		t.Fatal(err)
	}
	callID := "a3d7a190-8503-4bf4-9c12-123456789abc"
	if _, err := service.ReportRemoteCalls(ctx, deviceToken, sequence, []control.RemoteCall{{CallID: callID, State: "RINGING"}}); err != nil {
		t.Fatal(err)
	}
	var sealed []byte
	if err := db.QueryRow(`SELECT token_ciphertext FROM parent_wakeup_registrations WHERE family_id='family'`).Scan(&sealed); err != nil {
		t.Fatal(err)
	}
	sealed[len(sealed)-1] ^= 0x80
	if _, err := db.Exec(`UPDATE parent_wakeup_registrations SET token_ciphertext=? WHERE family_id='family'`, sealed); err != nil {
		t.Fatal(err)
	}
	if deliveries, err := store.Claim(ctx, 1); !errors.Is(err, ErrUnavailable) || len(deliveries) != 0 {
		t.Fatal("tampered ciphertext produced a routable token")
	}
}

func TestClaimBoundedEventAndFamilyFanoutProgress(t *testing.T) {
	ctx := context.Background()
	db := openPushDB(t)
	now := time.Now().UTC()
	seedParentAndFamily(t, db, "parent", "family-000", now.Add(time.Hour))
	store := newPushStore(t, db, nil)
	callID := "a3d7a190-8503-4bf4-9c12-123456789abc"

	for i := 0; i < 45; i++ {
		deviceID := fmt.Sprintf("device-%03d", i)
		eventID := fmt.Sprintf("%s:%s", deviceID, callID)
		observed := now.Add(time.Duration(i) * time.Millisecond).Format(time.RFC3339Nano)
		if _, err := db.Exec(`INSERT INTO devices(id,parent_account_id,display_name,public_key_der,key_algorithm,state,created_at)
			VALUES(?, 'parent','fixture',X'00','ECDSA_P256_SHA256','active',?)`,
			deviceID, now.Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO remote_call_snapshots(device_id,parent_account_id,observed_at,calls_json,snapshot_sequence)
			VALUES(?, 'parent', ?, ?, 1)`, deviceID, observed,
			`[{"callId":"`+callID+`","state":"RINGING"}]`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO parent_call_events(event_id,device_id,parent_account_id,call_id,
			snapshot_sequence,observed_at,created_at_ns,expires_at_ns,status)
			VALUES(?,?, 'parent',?,1,?,?,?,'active')`, eventID, deviceID, callID, observed,
			now.Add(time.Duration(i)*time.Millisecond).UnixNano(), now.Add(30*time.Second).UnixNano()); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Register(ctx, "parent", "family-000", "private-parent-routing-token"); err != nil {
		t.Fatal(err)
	}

	seen := make(map[string]bool, 45)
	for i := 0; i < 90 && len(seen) < 45; i++ {
		deliveries, err := store.Claim(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(deliveries) == 0 {
			continue
		}
		delivery := deliveries[0]
		seen[delivery.DeviceID] = true
		clear(delivery.RegistrationToken)
		if err := store.Finish(ctx, delivery.ID, delivery.LeaseID, true); err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) != 45 {
		t.Fatalf("bounded event cursor starved devices: processed %d of 45", len(seen))
	}
	if _, err := db.Exec(`UPDATE parent_call_events SET status='expired' WHERE event_id LIKE 'device-%:%'`); err != nil {
		t.Fatal(err)
	}

	// More registrations than one fan-out page must progress across claims.
	for i := 1; i < 56; i++ {
		familyID := fmt.Sprintf("family-%03d", i)
		if _, err := db.Exec(`INSERT INTO auth_session_families(id,parent_account_id,created_at,expires_at)
			VALUES(?,'parent',?,?)`, familyID, now.Format(time.RFC3339Nano),
			now.Add(time.Hour).Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
		if err := store.Register(ctx, "parent", familyID, "private-family-token"); err != nil {
			t.Fatal(err)
		}
	}
	// Add another event so each family can be fanned out from an unprocessed row.
	deviceID := "device-fanout"
	eventID := deviceID + ":" + callID
	observed := time.Now().UTC()
	if _, err := db.Exec(`INSERT INTO devices(id,parent_account_id,display_name,public_key_der,key_algorithm,state,created_at)
		VALUES(?, 'parent','fixture',X'00','ECDSA_P256_SHA256','active',?)`,
		deviceID, observed.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO remote_call_snapshots(device_id,parent_account_id,observed_at,calls_json,snapshot_sequence)
		VALUES(?, 'parent', ?, ?, 1)`, deviceID, observed.Format(time.RFC3339Nano),
		`[{"callId":"`+callID+`","state":"RINGING"}]`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO parent_call_events(event_id,device_id,parent_account_id,call_id,
		snapshot_sequence,observed_at,created_at_ns,expires_at_ns,status)
		VALUES(?,?, 'parent',?,1,?,?,?,'active')`, eventID, deviceID, callID,
		observed.Format(time.RFC3339Nano), observed.UnixNano(), observed.Add(30*time.Second).UnixNano()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 60; i++ {
		deliveries, err := store.Claim(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		for _, delivery := range deliveries {
			clear(delivery.RegistrationToken)
			if err := store.Finish(ctx, delivery.ID, delivery.LeaseID, true); err != nil {
				t.Fatal(err)
			}
		}
		if countRows(t, db, `SELECT COUNT(*) FROM parent_call_event_deliveries WHERE event_id=?`, eventID) == 56 {
			break
		}
	}
	if countRows(t, db, `SELECT COUNT(*) FROM parent_call_event_deliveries WHERE event_id=?`, eventID) != 56 {
		t.Fatal("bounded family fan-out failed to reach every registered family")
	}
}

func TestMaintenanceExpiresAndPrunesInBoundedBatches(t *testing.T) {
	ctx := context.Background()
	db := openPushDB(t)
	now := time.Now().UTC()
	seedParentAndFamily(t, db, "parent", "family", now.Add(time.Hour))
	deviceToken := seedCallSession(t, db, now)
	_ = deviceToken
	for i := 0; i < 23; i++ {
		callID := fmt.Sprintf("00000000-0000-4000-8000-%012d", i)
		eventID := "device:" + callID
		observed := now.Add(-time.Hour).Format(time.RFC3339Nano)
		if _, err := db.Exec(`INSERT INTO parent_call_events(event_id,device_id,parent_account_id,call_id,
			snapshot_sequence,observed_at,created_at_ns,expires_at_ns,status)
			VALUES(?, 'device','parent',?,1,?,?,?,'active')`, eventID, callID, observed,
			now.Add(-time.Hour).UnixNano(), now.Add(-time.Minute).UnixNano()); err != nil {
			t.Fatal(err)
		}
	}
	if err := Maintain(ctx, db, 5); err != nil {
		t.Fatal(err)
	}
	if countRows(t, db, `SELECT COUNT(*) FROM parent_call_events WHERE status='expired'`) != 5 {
		t.Fatal("expiry maintenance exceeded or missed its batch bound")
	}
	for i := 0; i < 5; i++ {
		if err := Maintain(ctx, db, 5); err != nil {
			t.Fatal(err)
		}
	}
	if countRows(t, db, `SELECT COUNT(*) FROM parent_call_events WHERE status='active'`) != 0 {
		t.Fatal("expired events were starved across maintenance batches")
	}
	maintenanceContext, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()
	if err := RunMaintenance(maintenanceContext, db, nil); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(`UPDATE parent_call_events SET expires_at_ns=?`,
		now.Add(-eventRetention-time.Second).UnixNano()); err != nil {
		t.Fatal(err)
	}
	if err := Maintain(ctx, db, 4); err != nil {
		t.Fatal(err)
	}
	if countRows(t, db, `SELECT COUNT(*) FROM parent_call_events`) != 19 {
		t.Fatal("retention cleanup did not honor its bounded batch")
	}
}
