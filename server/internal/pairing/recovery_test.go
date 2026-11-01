package pairing

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

func enrollRecoveryFixture(t *testing.T, f testFixture) (Device, *ecdsa.PrivateKey) {
	t.Helper()
	key, publicKey := generateDeviceKey(t)
	code, err := f.service.CreateCode(context.Background(), f.parentA.ID)
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := f.service.StartEnrollment(context.Background(), code.Code, "Permanent child", publicKey, KeyAlgorithmECDSAP256SHA256)
	if err != nil {
		t.Fatal(err)
	}
	device, err := f.service.CompleteEnrollment(context.Background(), enrollment.ID, enrollment.Challenge, signEnrollment(t, key, enrollment))
	if err != nil {
		t.Fatal(err)
	}
	return device, key
}

func recoveryHandle(seed string) string {
	hash := sha256.Sum256([]byte(seed))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

func signRecovery(t *testing.T, key *ecdsa.PrivateKey, start RecoveryStart) string {
	t.Helper()
	challenge, err := base64.RawURLEncoding.DecodeString(start.Challenge)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(recoveryMessage(start.RequestID, challenge))
	signature, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(signature)
}

func TestRecoveryRotatesExistingDeviceAndInvalidatesCredentials(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	device, oldKey := enrollRecoveryFixture(t, f)
	handle := recoveryHandle("same physical phone")
	if err := f.service.BindRecovery(ctx, device.ID, handle); err != nil {
		t.Fatal(err)
	}
	var storedBinding []byte
	if err := f.service.database.QueryRowContext(ctx, "SELECT binding_hash FROM device_recovery_bindings WHERE device_id=?", device.ID).Scan(&storedBinding); err != nil {
		t.Fatal(err)
	}
	rawHandle, _ := base64.RawURLEncoding.DecodeString(handle)
	if bytes.Equal(storedBinding, rawHandle) || bytes.Contains(storedBinding, []byte(handle)) {
		t.Fatal("raw recovery handle was stored")
	}
	challenge, err := f.service.CreateAuthChallenge(ctx, device.ID)
	if err != nil {
		t.Fatal(err)
	}
	challengeRaw, _ := base64.RawURLEncoding.DecodeString(challenge.Challenge)
	digest := sha256.Sum256(authMessage(device.ID, challenge.ID, challengeRaw))
	signature, _ := ecdsa.SignASN1(rand.Reader, oldKey, digest[:])
	session, err := f.service.CompleteAuthentication(ctx, device.ID, challenge.ID, challenge.Challenge, base64.RawURLEncoding.EncodeToString(signature))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.database.ExecContext(ctx, `INSERT INTO wakeup_registration_tokens(device_id,registration_ciphertext,updated_at) VALUES(?,?,?)`, device.ID, make([]byte, 29), formatTime(f.clock.Now())); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.database.ExecContext(ctx, `INSERT INTO commands(id,device_id,parent_account_id,type,payload,issued_at,expires_at,status,ttl_seconds,status_at)
		VALUES('cmd_recovery_stale',?,?, 'LOCK_DEVICE','{}',?,?,'queued',60,?)`, device.ID, f.parentA.ID,
		formatTime(f.clock.Now()), formatTime(f.clock.Now().Add(time.Minute)), formatTime(f.clock.Now())); err != nil {
		t.Fatal(err)
	}
	newKey, newPublic := generateDeviceKey(t)
	start, err := f.service.StartRecovery(ctx, handle, newPublic, KeyAlgorithmECDSAP256SHA256)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.service.ProveRecovery(ctx, start.RequestID, start.RequestToken, start.Challenge, signRecovery(t, newKey, start)); err != nil {
		t.Fatal(err)
	}
	if pending, err := f.service.PendingRecoveries(ctx, f.parentB.ID); err != nil || len(pending) != 0 {
		t.Fatalf("cross-parent request exposure: %v %#v", err, pending)
	}
	pending, err := f.service.PendingRecoveries(ctx, f.parentA.ID)
	if err != nil || len(pending) != 1 || pending[0].DeviceID != device.ID {
		t.Fatalf("pending request mismatch: %v %#v", err, pending)
	}
	if err = f.service.ResolveRecovery(ctx, f.parentB.ID, start.RequestID, true); !errors.Is(err, ErrRecoveryNotFound) {
		t.Fatalf("cross-parent approval: %v", err)
	}
	if err = f.service.ResolveRecovery(ctx, f.parentA.ID, start.RequestID, true); err != nil {
		t.Fatal(err)
	}
	status, err := f.service.RecoveryStatus(ctx, start.RequestID, start.RequestToken)
	if err != nil || status.State != "approved" || status.DeviceID != device.ID {
		t.Fatalf("status: %v %#v", err, status)
	}
	if _, err = f.service.AuthenticateDevice(ctx, session.AccessToken); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("old session survived: %v", err)
	}
	var devices, version, sessions, challenges, wakeup, audit int
	var commandStatus string
	if err = f.service.database.QueryRowContext(ctx, `SELECT COUNT(*),MAX(credential_version) FROM devices WHERE parent_account_id=?`, f.parentA.ID).Scan(&devices, &version); err != nil {
		t.Fatal(err)
	}
	if err = f.service.database.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM device_sessions WHERE device_id=?),(SELECT COUNT(*) FROM device_auth_challenges WHERE device_id=?),(SELECT COUNT(*) FROM wakeup_registration_tokens WHERE device_id=?)`, device.ID, device.ID, device.ID).Scan(&sessions, &challenges, &wakeup); err != nil {
		t.Fatal(err)
	}
	if err = f.service.database.QueryRowContext(ctx, "SELECT status FROM commands WHERE id='cmd_recovery_stale'").Scan(&commandStatus); err != nil {
		t.Fatal(err)
	}
	if err = f.service.database.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_events WHERE action='device_credential_rotated' AND target_id=?", device.ID).Scan(&audit); err != nil {
		t.Fatal(err)
	}
	if devices != 1 || version != 2 || sessions != 0 || challenges != 0 || wakeup != 0 || commandStatus != "expired" || audit != 1 {
		t.Fatalf("rotation not atomic: devices=%d version=%d sessions=%d challenges=%d wakeup=%d command=%s audit=%d", devices, version, sessions, challenges, wakeup, commandStatus, audit)
	}
	oldChallenge, err := f.service.CreateAuthChallenge(ctx, device.ID)
	if err != nil {
		t.Fatal(err)
	}
	signAuth := func(key *ecdsa.PrivateKey, challenge AuthChallenge) string {
		raw, _ := base64.RawURLEncoding.DecodeString(challenge.Challenge)
		digest := sha256.Sum256(authMessage(device.ID, challenge.ID, raw))
		signature, _ := ecdsa.SignASN1(rand.Reader, key, digest[:])
		return base64.RawURLEncoding.EncodeToString(signature)
	}
	if _, err = f.service.CompleteAuthentication(ctx, device.ID, oldChallenge.ID, oldChallenge.Challenge, signAuth(oldKey, oldChallenge)); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("old key authenticated after rotation: %v", err)
	}
	newChallenge, err := f.service.CreateAuthChallenge(ctx, device.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.CompleteAuthentication(ctx, device.ID, newChallenge.ID, newChallenge.Challenge, signAuth(newKey, newChallenge)); err != nil {
		t.Fatalf("new key did not authenticate: %v", err)
	}
}

func TestRecoveryHandleIsLocatorNotAuthorization(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	device, _ := enrollRecoveryFixture(t, f)
	matched := recoveryHandle("bound")
	if err := f.service.BindRecovery(ctx, device.ID, matched); err != nil {
		t.Fatal(err)
	}
	for _, handle := range []string{matched, recoveryHandle("unmatched")} {
		key, publicKey := generateDeviceKey(t)
		start, err := f.service.StartRecovery(ctx, handle, publicKey, KeyAlgorithmECDSAP256SHA256)
		if err != nil {
			t.Fatal(err)
		}
		if len(start.RequestID) != 26 || len(start.RequestToken) != 43 || len(start.Challenge) != 43 {
			t.Fatalf("distinguishable response shape: %#v", start)
		}
		if err = f.service.ProveRecovery(ctx, start.RequestID, start.RequestToken, start.Challenge, signRecovery(t, key, start)); err != nil {
			t.Fatal(err)
		}
	}
	pending, err := f.service.PendingRecoveries(ctx, f.parentA.ID)
	if err != nil || len(pending) != 1 {
		t.Fatalf("unmatched request reached parent: %v %#v", err, pending)
	}
}

func TestRecoveryCannotReviveRevokedOrExpiredDevice(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	device, _ := enrollRecoveryFixture(t, f)
	handle := recoveryHandle("revoked")
	if err := f.service.BindRecovery(ctx, device.ID, handle); err != nil {
		t.Fatal(err)
	}
	if err := f.service.RevokeDevice(ctx, f.parentA.ID, device.ID); err != nil {
		t.Fatal(err)
	}
	key, publicKey := generateDeviceKey(t)
	start, err := f.service.StartRecovery(ctx, handle, publicKey, KeyAlgorithmECDSAP256SHA256)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.service.ProveRecovery(ctx, start.RequestID, start.RequestToken, start.Challenge, signRecovery(t, key, start)); err != nil {
		t.Fatal(err)
	}
	if pending, err := f.service.PendingRecoveries(ctx, f.parentA.ID); err != nil || len(pending) != 0 {
		t.Fatalf("revoked device became recoverable: %v %#v", err, pending)
	}
	f.clock.Advance(6 * time.Minute)
	status, err := f.service.RecoveryStatus(ctx, start.RequestID, start.RequestToken)
	if err != nil || status.State != "expired" {
		t.Fatalf("expiry status: %v %#v", err, status)
	}
}
