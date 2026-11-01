package parentpush

import (
	"appcontrol/server/internal/control"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

type senderFunc func(context.Context, []byte, time.Duration) (SendResult, error)

func (f senderFunc) Send(ctx context.Context, token []byte, ttl time.Duration) (SendResult, error) {
	return f(ctx, token, ttl)
}

func newWorkerFixture(t *testing.T) (*Store, *sql.DB) {
	t.Helper()
	ctx := context.Background()
	db := openPushDB(t)
	now := time.Now().UTC()
	seedParentAndFamily(t, db, "parent", "family", now.Add(time.Hour))
	deviceToken := seedCallSession(t, db, now)
	store := newPushStore(t, db, nil)
	if err := store.Register(ctx, "parent", "family", fixtureFCMToken); err != nil {
		t.Fatal(err)
	}
	service := control.NewService(db)
	sequence, err := service.AllocateRemoteCallSequence(ctx, deviceToken)
	if err != nil {
		t.Fatal(err)
	}
	callID := "a3d7a190-8503-4bf4-9c12-123456789abc"
	if _, err := service.ReportRemoteCalls(ctx, deviceToken, sequence,
		[]control.RemoteCall{{CallID: callID, State: "RINGING"}}); err != nil {
		t.Fatal(err)
	}
	return store, db
}

func TestWorkerAcknowledgementRetriesAndExplicitInvalidation(t *testing.T) {
	t.Run("accepted", func(t *testing.T) {
		store, db := newWorkerFixture(t)
		worker, err := NewWorker(store, senderFunc(func(_ context.Context, token []byte, ttl time.Duration) (SendResult, error) {
			if string(token) != fixtureFCMToken || ttl <= 0 || ttl > maxMessageTTL {
				t.Fatal("worker sent an invalid registration or TTL")
			}
			return SendAccepted, nil
		}), nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := worker.ProcessOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
		if countRows(t, db, `SELECT COUNT(*) FROM parent_call_event_deliveries WHERE status='sent'`) != 1 {
			t.Fatal("transport acceptance did not complete the outbox lease")
		}
		if countRows(t, db, `SELECT COUNT(*) FROM parent_wakeup_registrations WHERE family_id='family'`) != 1 {
			t.Fatal("accepted send removed the registration")
		}
	})

	t.Run("generic-provider-error", func(t *testing.T) {
		store, db := newWorkerFixture(t)
		var logs bytes.Buffer
		worker, err := NewWorker(store, senderFunc(func(context.Context, []byte, time.Duration) (SendResult, error) {
			return 0, errors.New("private-parent-routing-token private-provider-response")
		}), slog.New(slog.NewJSONHandler(&logs, nil)))
		if err != nil {
			t.Fatal(err)
		}
		if err := worker.ProcessOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
		if countRows(t, db, `SELECT COUNT(*) FROM parent_call_event_deliveries WHERE status='pending'`) != 1 ||
			countRows(t, db, `SELECT COUNT(*) FROM parent_wakeup_registrations WHERE family_id='family'`) != 1 {
			t.Fatal("generic HTTP/provider error deleted token or did not schedule retry")
		}
		if strings.Contains(logs.String(), "private-parent-routing-token") ||
			strings.Contains(logs.String(), "private-provider-response") ||
			strings.Contains(logs.String(), "a3d7a190") || strings.Contains(logs.String(), "device") {
			t.Fatal("worker logs disclosed token, response, device or call details")
		}
	})

	t.Run("explicit-unregistered", func(t *testing.T) {
		store, db := newWorkerFixture(t)
		worker, err := NewWorker(store, senderFunc(func(context.Context, []byte, time.Duration) (SendResult, error) {
			return SendUnregistered, nil
		}), nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := worker.ProcessOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
		if countRows(t, db, `SELECT COUNT(*) FROM parent_wakeup_registrations WHERE family_id='family'`) != 0 ||
			countRows(t, db, `SELECT COUNT(*) FROM parent_call_event_deliveries WHERE status='cancelled'`) != 1 {
			t.Fatal("explicit invalid-token classification did not conditionally clean up")
		}
	})
}

func TestWorkerLateInvalidResultCannotDeleteRotatedOrReRegisteredToken(t *testing.T) {
	store, db := newWorkerFixture(t)
	worker, err := NewWorker(store, senderFunc(func(context.Context, []byte, time.Duration) (SendResult, error) {
		if err := store.Register(context.Background(), "parent", "family", "private-new-registration"); err != nil {
			t.Fatal(err)
		}
		return SendUnregistered, nil
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.ProcessOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	var version int64
	if err := db.QueryRow(`SELECT registration_version FROM parent_wakeup_registrations WHERE family_id='family'`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 2 || countRows(t, db, `SELECT COUNT(*) FROM parent_call_event_deliveries WHERE status='pending'`) != 1 {
		t.Fatal("late invalid-token response corrupted a rotated registration or retry")
	}
}

func TestWorkerRejectsStaleOrExpiredCallBeforeNetwork(t *testing.T) {
	for _, scenario := range []string{"ended", "expired-family", "cross-owner"} {
		t.Run(scenario, func(t *testing.T) {
			store, db := newWorkerFixture(t)
			switch scenario {
			case "ended":
				if _, err := db.Exec(`UPDATE remote_call_snapshots SET calls_json='[]' WHERE device_id='device'`); err != nil {
					t.Fatal(err)
				}
			case "expired-family":
				if _, err := db.Exec(`UPDATE auth_session_families SET expires_at=? WHERE id='family'`,
					time.Now().Add(-time.Second).UTC().Format(time.RFC3339Nano)); err != nil {
					t.Fatal(err)
				}
			case "cross-owner":
				if _, err := db.Exec(`INSERT INTO parent_accounts(id,email,password_hash,created_at,updated_at)
					VALUES('other','other@example.com','unused',?,?)`,
					time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`UPDATE devices SET parent_account_id='other' WHERE id='device'`); err != nil {
					t.Fatal(err)
				}
			}
			called := false
			worker, err := NewWorker(store, senderFunc(func(context.Context, []byte, time.Duration) (SendResult, error) {
				called = true
				return SendAccepted, nil
			}), nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := worker.ProcessOnce(context.Background()); err != nil {
				t.Fatal(err)
			}
			if called {
				t.Fatal("ineligible event reached the sender")
			}
			if countRows(t, db, `SELECT COUNT(*) FROM parent_call_event_deliveries WHERE status='leased'`) != 0 {
				t.Fatal("ineligible event kept a network lease")
			}
		})
	}
}

func TestWorkerUnregisterDuringSendInvalidatesTheOldLease(t *testing.T) {
	store, db := newWorkerFixture(t)
	worker, err := NewWorker(store, senderFunc(func(ctx context.Context, _ []byte, _ time.Duration) (SendResult, error) {
		if err := store.Remove(ctx, "parent", "family"); err != nil {
			t.Fatal(err)
		}
		return SendAccepted, nil
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.ProcessOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if countRows(t, db, `SELECT COUNT(*) FROM parent_wakeup_registrations WHERE family_id='family'`) != 0 ||
		countRows(t, db, `SELECT COUNT(*) FROM parent_call_event_deliveries WHERE status='sent'`) != 0 ||
		countRows(t, db, `SELECT COUNT(*) FROM parent_call_event_deliveries WHERE status='cancelled'`) != 1 {
		t.Fatal("unregister during network send did not invalidate the old lease")
	}
}

func TestWorkerCancellationReleasesCurrentLeaseWithoutLoggingSecrets(t *testing.T) {
	store, db := newWorkerFixture(t)
	started := make(chan struct{})
	worker, err := NewWorker(store, senderFunc(func(ctx context.Context, _ []byte, _ time.Duration) (SendResult, error) {
		close(started)
		<-ctx.Done()
		return 0, ctx.Err()
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = worker.ProcessOnce(ctx)
	}()
	<-started
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop promptly after cancellation")
	}
	if countRows(t, db, `SELECT COUNT(*) FROM parent_call_event_deliveries WHERE status='pending'`) != 1 {
		t.Fatal("cancelled send did not release its lease for retry")
	}
}
