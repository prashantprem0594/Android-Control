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
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"appcontrol/server/internal/auth"
	"appcontrol/server/internal/pairing"
	"appcontrol/server/internal/storage"
)

func TestDeviceRecoveryHTTPContract(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "recovery-http.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	authentication, err := auth.NewService(db, auth.Options{})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := authentication.Register(ctx, "recovery-parent@example.com", "long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	oldKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	oldDER, _ := x509.MarshalPKIXPublicKey(&oldKey.PublicKey)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err = db.ExecContext(ctx, `INSERT INTO devices(id,parent_account_id,display_name,public_key_der,key_algorithm,state,created_at,enrolled_at)
		VALUES('dev_AAAAAAAAAAAAAAAAAAAAAA',?,'Child',?,'ECDSA_P256_SHA256','active',?,?)`, parent.ID, oldDER, now, now); err != nil {
		t.Fatal(err)
	}
	deviceToken := "dt_" + base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	deviceTokenHash := sha256.Sum256([]byte(deviceToken))
	if _, err = db.ExecContext(ctx, `INSERT INTO device_sessions(token_hash,device_id,expires_at,issued_at) VALUES(?,'dev_AAAAAAAAAAAAAAAAAAAAAA',?,?)`, deviceTokenHash[:], time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano), now); err != nil {
		t.Fatal(err)
	}
	service, err := pairing.NewService(db, pairing.Options{RecoveryHMACKey: []byte("0123456789abcdef0123456789abcdef")})
	if err != nil {
		t.Fatal(err)
	}
	fake := completeFakeAuth()
	fake.authenticate = func(_ context.Context, token string) (auth.Parent, string, error) { return parent, token, nil }
	handler := NewHandler(Options{Auth: fake, Pairing: service})
	call := func(method, path, token string, body any, expected int, output any) {
		t.Helper()
		raw := ""
		if body != nil {
			encoded, marshalErr := json.Marshal(body)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			raw = string(encoded)
		}
		request := httptest.NewRequest(method, path, strings.NewReader(raw))
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != expected {
			t.Fatalf("%s %s: status=%d body=%s", method, path, response.Code, response.Body.String())
		}
		if output != nil {
			if err := json.Unmarshal(response.Body.Bytes(), output); err != nil {
				t.Fatal(err)
			}
		}
	}
	handleHash := sha256.Sum256([]byte("same-device-locator"))
	handle := base64.RawURLEncoding.EncodeToString(handleHash[:])
	call(http.MethodPut, "/v1/device/recovery-binding", deviceToken, map[string]string{"recoveryHandle": handle}, 204, nil)
	newKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	newDER, _ := x509.MarshalPKIXPublicKey(&newKey.PublicKey)
	var start pairing.RecoveryStart
	call(http.MethodPost, "/v1/device-recovery/requests", "", map[string]string{"recoveryHandle": handle, "publicKey": base64.RawURLEncoding.EncodeToString(newDER), "keyAlgorithm": pairing.KeyAlgorithmECDSAP256SHA256}, 201, &start)
	challenge, _ := base64.RawURLEncoding.DecodeString(start.Challenge)
	digest := sha256.Sum256(append([]byte("app-control-device-recovery-v1\n"+start.RequestID+"\n"), challenge...))
	signature, _ := ecdsa.SignASN1(rand.Reader, newKey, digest[:])
	call(http.MethodPost, "/v1/device-recovery/requests/"+start.RequestID+"/proof", "", map[string]string{"requestToken": start.RequestToken, "challenge": start.Challenge, "signature": base64.RawURLEncoding.EncodeToString(signature)}, 204, nil)
	var pending struct {
		Requests []pairing.PendingRecovery `json:"requests"`
	}
	call(http.MethodGet, "/v1/device-recovery/requests", "owner", nil, 200, &pending)
	if len(pending.Requests) != 1 || pending.Requests[0].DeviceID != "dev_AAAAAAAAAAAAAAAAAAAAAA" {
		t.Fatalf("pending recovery missing: %#v", pending)
	}
	call(http.MethodPost, "/v1/device-recovery/requests/"+start.RequestID+"/approve", "owner", nil, 204, nil)
	var status pairing.RecoveryStatus
	call(http.MethodPost, "/v1/device-recovery/requests/"+start.RequestID+"/status", "", map[string]string{"requestToken": start.RequestToken}, 200, &status)
	if status.State != "approved" || status.DeviceID != "dev_AAAAAAAAAAAAAAAAAAAAAA" {
		t.Fatalf("unexpected status: %#v", status)
	}
	call(http.MethodPost, "/v1/device-recovery/requests", "", map[string]string{"unknown": "field"}, 400, nil)
}

func TestPublicRecoveryEndpointsUseDedicatedRateGroup(t *testing.T) {
	for _, path := range []string{
		"/v1/device-recovery/requests",
		"/v1/device-recovery/requests/drr_AAAAAAAAAAAAAAAAAAAAAA/proof",
		"/v1/device-recovery/requests/drr_AAAAAAAAAAAAAAAAAAAAAA/status",
	} {
		request := httptest.NewRequest(http.MethodPost, path, nil)
		if group := requestGroup(request); group != "device-recovery" {
			t.Fatalf("%s used rate group %q", path, group)
		}
	}
	peer, global := rates("device-recovery", 1)
	if peer.burst != 5 || peer.perMinute != 10 || global.burst != 30 || global.perMinute != 120 {
		t.Fatal("public recovery rate limits changed")
	}
}
