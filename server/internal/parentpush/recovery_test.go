package parentpush

import (
	"appcontrol/server/internal/auth"
	"appcontrol/server/internal/storage"
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// This clean-directory recovery uses fixture keys and no provider.
// It does not establish deployed-host or real Firebase acceptance.
func TestEncryptedRestorePreservesParentPushRecoveryBoundaries(t *testing.T) {
	for _, outage := range []time.Duration{21 * time.Second, 31 * time.Second} {
		t.Run(outage.String(), func(t *testing.T) {
			ctx := context.Background()
			store, source := newWorkerFixture(t)
			clock := &pushClock{now: time.Now().UTC()}
			store.now = clock.Now
			before, err := store.Claim(ctx, 1)
			if err != nil || len(before) != 1 {
				t.Fatal("source lease missing")
			}
			defer clear(before[0].RegistrationToken)
			backupKey := bytes.Repeat([]byte{19}, 32)
			backup := filepath.Join(t.TempDir(), "recovery.acb")
			if err := storage.CreateEncryptedBackup(ctx, source, backup, backupKey); err != nil {
				t.Fatal(err)
			}
			if err := source.Close(); err != nil {
				t.Fatal(err)
			}
			restoredPath := filepath.Join(t.TempDir(), "restored.db")
			if err := storage.RestoreEncryptedBackup(ctx, backup, restoredPath, backupKey); err != nil {
				t.Fatal(err)
			}
			db, err := storage.Open(ctx, restoredPath)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			clock.now = clock.now.Add(outage)
			recovered := newPushStore(t, db, clock)
			if outage < eventFreshness {
				wrongKey, err := NewStore(db, bytes.Repeat([]byte{8}, 32))
				if err != nil {
					t.Fatal(err)
				}
				wrongKey.now = clock.Now
				if deliveries, err := wrongKey.Claim(ctx, 1); !errors.Is(err, ErrUnavailable) || len(deliveries) != 0 {
					t.Fatal("restored routing ciphertext accepted with a different key")
				}
			}
			deliveries, err := recovered.Claim(ctx, 1)
			if err != nil {
				t.Fatal(err)
			}
			feed, err := recovered.CallEvents(ctx, "parent", "family")
			if err != nil {
				t.Fatal(err)
			}
			if outage >= eventFreshness {
				if len(deliveries) != 0 || len(feed.Events) != 0 ||
					countRows(t, db, `SELECT COUNT(*) FROM parent_call_event_deliveries WHERE status='expired'`) != 1 {
					t.Fatal("outage beyond call freshness replayed a restored alert")
				}
				return
			}
			if len(deliveries) != 1 || len(feed.Events) != 1 {
				t.Fatal("eligible restored call did not recover")
			}
			delivery := deliveries[0]
			defer clear(delivery.RegistrationToken)
			if delivery.ID != before[0].ID || delivery.LeaseID == before[0].LeaseID ||
				delivery.Attempt != 2 || string(delivery.RegistrationToken) != fixtureFCMToken ||
				!delivery.ExpiresAt.Equal(before[0].ExpiresAt) {
				t.Fatal("restore lost delivery identity, retry history, routing key or original expiry")
			}
			if err := recovered.Finish(ctx, before[0].ID, before[0].LeaseID, true); !errors.Is(err, ErrUnauthorized) {
				t.Fatal("pre-outage lease completed the recovered attempt")
			}
			accounts, err := auth.NewService(db, auth.Options{Now: clock.Now})
			if err != nil {
				t.Fatal(err)
			}
			if err := accounts.RecoverPassword(ctx, "parent@example.com", "replacement-fixture-password"); err != nil {
				t.Fatal(err)
			}
			if _, eligible, err := recovered.ValidateLease(ctx, delivery); err != nil || eligible ||
				countRows(t, db, `SELECT COUNT(*) FROM parent_wakeup_registrations`) != 0 ||
				countRows(t, db, `SELECT COUNT(*) FROM parent_call_event_deliveries WHERE status='cancelled'`) != 1 {
				t.Fatal("recovered session revocation retained routing or delivery eligibility")
			}
			if _, err := recovered.CallEvents(ctx, "parent", "family"); !errors.Is(err, ErrUnauthorized) {
				t.Fatal("revoked recovered family read the event feed")
			}
			if countRows(t, db, `SELECT COUNT(*) FROM devices WHERE id='device' AND state='active'`) != 1 ||
				countRows(t, db, `SELECT COUNT(*) FROM audit_events WHERE action='parent_password_reset'`) != 1 {
				t.Fatal("restored password recovery lost active enrollment or immutable audit")
			}
		})
	}
}
