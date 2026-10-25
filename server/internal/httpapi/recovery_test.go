package httpapi

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"appcontrol/server/internal/auth"
	"appcontrol/server/internal/control"
	"appcontrol/server/internal/pairing"
	"appcontrol/server/internal/storage"
)

// This is a local recovery exercise with a software device key, not Android
// Keystore, platform pinning, or clean-host deployment acceptance.
func TestRecoveredServerReconnectsParentAndDeviceOverHTTPS(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := storage.Open(ctx, filepath.Join(dir, "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	authentication, err := auth.NewService(db, auth.Options{})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := authentication.Register(ctx, "recovery@example.com", "long-recovery-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = authentication.Register(ctx, "other-recovery@example.com", "long-recovery-test-password"); err != nil {
		t.Fatal(err)
	}
	devices, err := pairing.NewService(db, pairing.Options{})
	if err != nil {
		t.Fatal(err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	public, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	encode := base64.RawURLEncoding.EncodeToString
	code, err := devices.CreateCode(ctx, parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := devices.StartEnrollment(ctx, code.Code, "Recovery fixture", encode(public), pairing.KeyAlgorithmECDSAP256SHA256)
	if err != nil {
		t.Fatal(err)
	}
	sign := func(prefix, challenge string) string {
		t.Helper()
		raw, err := base64.RawURLEncoding.DecodeString(challenge)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(append([]byte(prefix), raw...))
		signature, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
		if err != nil {
			t.Fatal(err)
		}
		return encode(signature)
	}
	device, err := devices.CompleteEnrollment(ctx, enrollment.ID, enrollment.Challenge,
		sign("app-control-device-enrollment-v1\n"+enrollment.ID+"\n", enrollment.Challenge))
	if err != nil {
		t.Fatal(err)
	}
	controls := control.NewService(db)
	command, err := controls.QueueLockWithKey(ctx, parent.ID, device.ID, 3600, "recovery-request-0001")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = controls.UpdatePolicy(ctx, parent.ID, device.ID, 0, true, nil); err != nil {
		t.Fatal(err)
	}
	backupKey := make([]byte, 32)
	if _, err = rand.Read(backupKey); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(dir, "snapshot.acb")
	if err = storage.CreateEncryptedBackup(ctx, db, backup, backupKey); err != nil {
		t.Fatal(err)
	}
	// Stop the source and restore into a separate directory. No source DB handles
	// or source service instances are used for the reconnect checks below.
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	restoredPath := filepath.Join(t.TempDir(), "restored.db")
	if err = storage.RestoreEncryptedBackup(ctx, backup, restoredPath, backupKey); err != nil {
		t.Fatal(err)
	}
	restored, err := storage.Open(ctx, restoredPath)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	controls = control.NewService(restored)
	serverURL, client := startRecoveryProcess(t, restoredPath)
	call := func(method, path, token string, body any, want int, result any) {
		t.Helper()
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		request, err := http.NewRequest(method, serverURL+path, strings.NewReader(string(data)))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != want {
			t.Fatalf("%s: status %d, want %d", path, response.StatusCode, want)
		}
		if result != nil {
			if err := json.NewDecoder(response.Body).Decode(result); err != nil {
				t.Fatal(err)
			}
		}
	}
	var login struct {
		Tokens auth.TokenPair `json:"tokens"`
	}
	call("GET", "/readyz", "", nil, 200, nil)
	call("POST", "/v1/auth/login", "", map[string]string{"email": "recovery@example.com", "password": "long-recovery-test-password"}, 200, &login)
	call("GET", "/v1/devices/"+device.ID, login.Tokens.AccessToken, nil, 200, nil)
	var otherLogin struct {
		Tokens auth.TokenPair `json:"tokens"`
	}
	call("POST", "/v1/auth/login", "", map[string]string{"email": "other-recovery@example.com", "password": "long-recovery-test-password"}, 200, &otherLogin)
	call("GET", "/v1/devices/"+device.ID, otherLogin.Tokens.AccessToken, nil, 404, nil)
	var challenge pairing.AuthChallenge
	call("POST", "/v1/device-auth/challenges", "", map[string]string{"deviceId": device.ID}, 201, &challenge)
	var session pairing.DeviceSession
	call("POST", "/v1/device-auth/sessions", "", map[string]string{
		"deviceId": device.ID, "challengeId": challenge.ID, "challenge": challenge.Challenge,
		"signature": sign("app-control-device-auth-v1\n"+device.ID+"\n"+challenge.ID+"\n", challenge.Challenge),
	}, 201, &session)
	var policy control.Policy
	call("GET", "/v1/device/policy", session.AccessToken, nil, 200, &policy)
	if policy.Version != 1 || !policy.InternetPaused {
		t.Fatal("recovered desired policy differs")
	}
	call("POST", "/v1/device/policy/report", session.AccessToken, map[string]any{"version": 1, "result": "applied"}, 204, nil)
	reported, err := controls.Policy(ctx, parent.ID, device.ID)
	if err != nil || reported.Reported == nil || reported.Reported.Version != 1 || reported.Reported.Result != "applied" {
		t.Fatal("recovered server lost policy report")
	}
	var batch struct {
		Commands []control.Command `json:"commands"`
	}
	call("GET", "/v1/device/commands", session.AccessToken, nil, 200, &batch)
	if len(batch.Commands) != 1 || batch.Commands[0].ID != command.ID {
		t.Fatal("recovered queue differs")
	}
	pending, err := controls.Command(ctx, parent.ID, device.ID, command.ID)
	if err != nil || pending.Status != "delivered" {
		t.Fatal("recovery fetch incorrectly implied success")
	}
	call("POST", "/v1/device/commands/"+command.ID+"/ack", session.AccessToken, map[string]string{"result": "succeeded"}, 204, nil)
	confirmed, err := controls.Command(ctx, parent.ID, device.ID, command.ID)
	if err != nil || confirmed.Status != "succeeded" {
		t.Fatal("recovered server lost acknowledgement")
	}
	retried, err := controls.QueueLockWithKey(ctx, parent.ID, device.ID, 3600, "recovery-request-0001")
	if err != nil || retried.ID != command.ID {
		t.Fatal("restore lost parent request idempotency")
	}
	var count int
	if err = restored.QueryRow("SELECT count(*) FROM audit_events WHERE action='database_restored'").Scan(&count); err != nil || count != 1 {
		t.Fatal("recovery audit missing or duplicated")
	}
	call("POST", "/v1/devices/"+device.ID+"/revoke", otherLogin.Tokens.AccessToken, nil, 404, nil)
	call("GET", "/v1/device-auth/me", session.AccessToken, nil, 200, nil)
	call("POST", "/v1/devices/"+device.ID+"/revoke", login.Tokens.AccessToken, nil, 204, nil)
	call("GET", "/v1/device/commands", session.AccessToken, nil, 401, nil)
}
