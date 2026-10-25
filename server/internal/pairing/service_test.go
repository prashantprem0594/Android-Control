package pairing

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"appcontrol/server/internal/auth"
	"appcontrol/server/internal/storage"
)

type testClock struct{ current time.Time }

func (clock *testClock) Now() time.Time                 { return clock.current }
func (clock *testClock) Advance(duration time.Duration) { clock.current = clock.current.Add(duration) }

type testFixture struct {
	service *Service
	clock   *testClock
	parentA auth.Parent
	parentB auth.Parent
}

func newFixture(t *testing.T) testFixture {
	t.Helper()
	database, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "pairing.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	clock := &testClock{current: time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)}
	authService, err := auth.NewService(database, auth.Options{
		Now: clock.Now,
		PasswordParameters: auth.PasswordParameters{
			MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1, SaltBytes: 16, KeyBytes: 32,
		},
	})
	if err != nil {
		t.Fatalf("create auth service: %v", err)
	}
	parentA, err := authService.Register(context.Background(), "first@example.com", "first-parent-password")
	if err != nil {
		t.Fatalf("register first parent: %v", err)
	}
	parentB, err := authService.Register(context.Background(), "second@example.com", "second-parent-password")
	if err != nil {
		t.Fatalf("register second parent: %v", err)
	}
	service, err := NewService(database, Options{
		Now: clock.Now, PairingCodeLifetime: 10 * time.Minute,
		EnrollmentLifetime: 5 * time.Minute, MaxVerificationTries: 3,
	})
	if err != nil {
		t.Fatalf("create pairing service: %v", err)
	}
	return testFixture{service: service, clock: clock, parentA: parentA, parentB: parentB}
}

func generateDeviceKey(t *testing.T) (*ecdsa.PrivateKey, string) {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	return privateKey, base64.RawURLEncoding.EncodeToString(der)
}

func signEnrollment(t *testing.T, privateKey *ecdsa.PrivateKey, enrollment Enrollment) string {
	t.Helper()
	challenge, err := base64.RawURLEncoding.DecodeString(enrollment.Challenge)
	if err != nil {
		t.Fatalf("decode challenge: %v", err)
	}
	digest := sha256.Sum256(enrollmentMessage(enrollment.ID, challenge))
	signature, err := ecdsa.SignASN1(rand.Reader, privateKey, digest[:])
	if err != nil {
		t.Fatalf("sign enrollment: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(signature)
}

func TestOneTimePairingBindsKeyToCreatingParent(t *testing.T) {
	fixture := newFixture(t)
	privateKey, publicKey := generateDeviceKey(t)
	code, err := fixture.service.CreateCode(context.Background(), fixture.parentA.ID)
	if err != nil {
		t.Fatalf("create pairing code: %v", err)
	}
	enrollment, err := fixture.service.StartEnrollment(context.Background(), code.Code, "Redmi test phone", publicKey, KeyAlgorithmECDSAP256SHA256)
	if err != nil {
		t.Fatalf("start enrollment: %v", err)
	}
	if _, err := fixture.service.StartEnrollment(context.Background(), code.Code, "Second phone", publicKey, KeyAlgorithmECDSAP256SHA256); !errors.Is(err, ErrInvalidPairingCode) {
		t.Fatalf("expected pairing code reuse rejection, got %v", err)
	}

	device, err := fixture.service.CompleteEnrollment(context.Background(), enrollment.ID, enrollment.Challenge, signEnrollment(t, privateKey, enrollment))
	if err != nil {
		t.Fatalf("complete enrollment: %v", err)
	}
	if device.State != "active" || device.ParentID != fixture.parentA.ID || device.EnrolledAt == nil {
		t.Fatalf("unexpected enrolled device: %+v", device)
	}
	if _, err := fixture.service.Device(context.Background(), fixture.parentA.ID, device.ID); err != nil {
		t.Fatalf("creating parent should access device: %v", err)
	}
	if _, err := fixture.service.Device(context.Background(), fixture.parentB.ID, device.ID); !errors.Is(err, ErrDeviceNotFound) {
		t.Fatalf("different parent should not access device, got %v", err)
	}
	if _, err := fixture.service.CompleteEnrollment(context.Background(), enrollment.ID, enrollment.Challenge, signEnrollment(t, privateKey, enrollment)); !errors.Is(err, ErrInvalidEnrollment) {
		t.Fatalf("expected completed enrollment replay rejection, got %v", err)
	}
}

func TestEnrollmentRejectsWrongPrivateKeyThenAcceptsCorrectProof(t *testing.T) {
	fixture := newFixture(t)
	correctKey, publicKey := generateDeviceKey(t)
	wrongKey, _ := generateDeviceKey(t)
	code, err := fixture.service.CreateCode(context.Background(), fixture.parentA.ID)
	if err != nil {
		t.Fatalf("create pairing code: %v", err)
	}
	enrollment, err := fixture.service.StartEnrollment(context.Background(), code.Code, "Redmi test phone", publicKey, KeyAlgorithmECDSAP256SHA256)
	if err != nil {
		t.Fatalf("start enrollment: %v", err)
	}
	if _, err := fixture.service.CompleteEnrollment(context.Background(), enrollment.ID, enrollment.Challenge, signEnrollment(t, wrongKey, enrollment)); !errors.Is(err, ErrInvalidProof) {
		t.Fatalf("expected invalid proof, got %v", err)
	}
	if _, err := fixture.service.CompleteEnrollment(context.Background(), enrollment.ID, enrollment.Challenge, signEnrollment(t, correctKey, enrollment)); err != nil {
		t.Fatalf("correct key should complete enrollment after one failed attempt: %v", err)
	}
}

func TestEnrollmentExpiryAndProofAttemptLimit(t *testing.T) {
	t.Run("expired code", func(t *testing.T) {
		fixture := newFixture(t)
		_, publicKey := generateDeviceKey(t)
		code, err := fixture.service.CreateCode(context.Background(), fixture.parentA.ID)
		if err != nil {
			t.Fatalf("create pairing code: %v", err)
		}
		fixture.clock.Advance(10 * time.Minute)
		if _, err := fixture.service.StartEnrollment(context.Background(), code.Code, "Redmi", publicKey, KeyAlgorithmECDSAP256SHA256); !errors.Is(err, ErrInvalidPairingCode) {
			t.Fatalf("expected code expiry rejection, got %v", err)
		}
	})

	t.Run("proof attempt limit", func(t *testing.T) {
		fixture := newFixture(t)
		_, publicKey := generateDeviceKey(t)
		wrongKey, _ := generateDeviceKey(t)
		code, _ := fixture.service.CreateCode(context.Background(), fixture.parentA.ID)
		enrollment, err := fixture.service.StartEnrollment(context.Background(), code.Code, "Redmi", publicKey, KeyAlgorithmECDSAP256SHA256)
		if err != nil {
			t.Fatalf("start enrollment: %v", err)
		}
		for attempt := 0; attempt < 3; attempt++ {
			if _, err := fixture.service.CompleteEnrollment(context.Background(), enrollment.ID, enrollment.Challenge, signEnrollment(t, wrongKey, enrollment)); !errors.Is(err, ErrInvalidProof) {
				t.Fatalf("attempt %d: expected invalid proof, got %v", attempt+1, err)
			}
		}
		if _, err := fixture.service.CompleteEnrollment(context.Background(), enrollment.ID, enrollment.Challenge, signEnrollment(t, wrongKey, enrollment)); !errors.Is(err, ErrInvalidEnrollment) {
			t.Fatalf("expected enrollment lockout, got %v", err)
		}
	})
}
