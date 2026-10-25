package pairing

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

func TestDeviceAuthenticationRevocationAndIsolation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	key, public := generateDeviceKey(t)
	code, err := f.service.CreateCode(ctx, f.parentA.ID)
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := f.service.StartEnrollment(ctx, code.Code, "Test device", public, KeyAlgorithmECDSAP256SHA256)
	if err != nil {
		t.Fatal(err)
	}
	device, err := f.service.CompleteEnrollment(ctx, enrollment.ID, enrollment.Challenge, signEnrollment(t, key, enrollment))
	if err != nil {
		t.Fatal(err)
	}
	sign := func(c AuthChallenge, k *ecdsa.PrivateKey) string {
		raw, err := base64.RawURLEncoding.DecodeString(c.Challenge)
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(authMessage(device.ID, c.ID, raw))
		signature, err := ecdsa.SignASN1(rand.Reader, k, hash[:])
		if err != nil {
			t.Fatal(err)
		}
		return base64.RawURLEncoding.EncodeToString(signature)
	}
	challenge := func() AuthChallenge {
		c, e := f.service.CreateAuthChallenge(ctx, device.ID)
		if e != nil {
			t.Fatal(e)
		}
		return c
	}
	authenticate := func() DeviceSession {
		c := challenge()
		s, e := f.service.CompleteAuthentication(ctx, device.ID, c.ID, c.Challenge, sign(c, key))
		if e != nil {
			t.Fatal(e)
		}
		return s
	}
	c := challenge()
	wrong, _ := generateDeviceKey(t)
	if _, err := f.service.CompleteAuthentication(ctx, device.ID, c.ID, c.Challenge, sign(c, wrong)); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("wrong key: %v", err)
	}
	if _, err := f.service.CompleteAuthentication(ctx, device.ID, c.ID, c.Challenge, sign(c, key)); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("consumed challenge: %v", err)
	}
	c = challenge()
	f.clock.Advance(time.Minute)
	if _, err := f.service.CompleteAuthentication(ctx, device.ID, c.ID, c.Challenge, sign(c, key)); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("expired challenge: %v", err)
	}
	c = challenge()
	_ = challenge()
	if _, err := f.service.CompleteAuthentication(ctx, device.ID, c.ID, c.Challenge, sign(c, key)); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("replaced challenge: %v", err)
	}
	old := authenticate()
	session := authenticate()
	if _, err := f.service.AuthenticateDevice(ctx, old.AccessToken); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("old session: %v", err)
	}
	if id, err := f.service.AuthenticateDevice(ctx, session.AccessToken); err != nil || id != device.ID {
		t.Fatalf("session: %s %v", id, err)
	}
	var hash []byte
	if err := f.service.database.QueryRow("SELECT token_hash FROM device_sessions").Scan(&hash); err != nil {
		t.Fatal(err)
	}
	expected := sha256.Sum256([]byte(session.AccessToken))
	if string(hash) != string(expected[:]) {
		t.Fatal("token must be stored as hash")
	}
	if err := f.service.RevokeDevice(ctx, f.parentB.ID, device.ID); !errors.Is(err, ErrDeviceNotFound) {
		t.Fatalf("other parent: %v", err)
	}
	if _, err := f.service.AuthenticateDevice(ctx, session.AccessToken); err != nil {
		t.Fatal("other parent revoked device")
	}
	f.clock.Advance(15 * time.Minute)
	if _, err := f.service.AuthenticateDevice(ctx, session.AccessToken); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("expired session: %v", err)
	}
	session = authenticate()
	c = challenge()
	if err := f.service.RevokeDevice(ctx, f.parentA.ID, device.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.AuthenticateDevice(ctx, session.AccessToken); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("revoked session: %v", err)
	}
	if _, err := f.service.CreateAuthChallenge(ctx, device.ID); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("revoked challenge: %v", err)
	}
	if _, err := f.service.CompleteAuthentication(ctx, device.ID, c.ID, c.Challenge, sign(c, key)); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("revoked pending proof: %v", err)
	}
	if err := f.service.RevokeDevice(ctx, f.parentA.ID, device.ID); err != nil {
		t.Fatalf("idempotent revoke: %v", err)
	}
	// A new service instance consults the same persisted revocation state.
	restarted, err := NewService(f.service.database, Options{Now: f.clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.AuthenticateDevice(ctx, session.AccessToken); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("restart: %v", err)
	}
}
