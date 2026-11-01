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
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"appcontrol/server/internal/auth"
	"appcontrol/server/internal/control"
	"appcontrol/server/internal/pairing"
	"appcontrol/server/internal/storage"
)

func TestEnrolledDeviceCommandTransport(t *testing.T) {
	t.Run("handler", func(t *testing.T) { testEnrolledDeviceCommandTransport(t, false) })
	t.Run("https", func(t *testing.T) { testEnrolledDeviceCommandTransport(t, true) })
}

func testEnrolledDeviceCommandTransport(t *testing.T, overTLS bool) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "pairing.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	authentication, err := auth.NewService(db, auth.Options{})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := authentication.Register(ctx, "parent@example.com", "long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	service, err := pairing.NewService(db, pairing.Options{})
	if err != nil {
		t.Fatal(err)
	}
	fake := completeFakeAuth()
	fake.authenticate = func(_ context.Context, token string) (auth.Parent, string, error) {
		if token == "owner" {
			return parent, "", nil
		}
		return auth.Parent{ID: "another-parent"}, "", nil
	}
	handler := NewHandler(Options{Auth: fake, Pairing: service, Control: control.NewService(db)})
	var server *httptest.Server
	var client *http.Client
	if overTLS {
		server = httptest.NewTLSServer(handler)
		defer server.Close()
		client = server.Client()
		client.Timeout = 10 * time.Second
	}
	call := func(method, path, token, body string, expected int, target any) {
		t.Helper()
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		if overTLS {
			wire, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			wire.Header = request.Header.Clone()
			actual, err := client.Do(wire)
			if err != nil {
				t.Fatal(err)
			}
			payload, readErr := io.ReadAll(io.LimitReader(actual.Body, 1<<20))
			closeErr := actual.Body.Close()
			if readErr != nil || closeErr != nil {
				t.Fatal("HTTPS response could not be read and closed")
			}
			response.WriteHeader(actual.StatusCode)
			_, _ = response.Write(payload)
		} else {
			handler.ServeHTTP(response, request)
		}
		if response.Code != expected {
			t.Fatalf("%s: status=%d expected=%d", path, response.Code, expected)
		}
		if target != nil {
			if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
				t.Fatal(err)
			}
		}
	}
	call("POST", "/v1/pairing/codes", "", "", 401, nil)
	var code pairing.PairingCode
	call("POST", "/v1/pairing/codes", "owner", "", 201, &code)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	encode := base64.RawURLEncoding.EncodeToString
	start, _ := json.Marshal(map[string]string{"code": code.Code, "displayName": "Test device", "publicKey": encode(der), "keyAlgorithm": pairing.KeyAlgorithmECDSAP256SHA256})
	var enrollment pairing.Enrollment
	call("POST", "/v1/enrollments", "", string(start), 201, &enrollment)
	call("POST", "/v1/enrollments", "", string(start), 400, nil)
	challenge, _ := base64.RawURLEncoding.DecodeString(enrollment.Challenge)
	message := append([]byte("app-control-device-enrollment-v1\n"+enrollment.ID+"\n"), challenge...)
	digest := sha256.Sum256(message)
	signature, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	proof, _ := json.Marshal(map[string]string{"enrollmentId": enrollment.ID, "challenge": enrollment.Challenge, "signature": encode(signature)})
	var device pairing.Device
	call("POST", "/v1/enrollments/complete", "", string(proof), 200, &device)
	if device.ParentID != parent.ID || device.State != "active" {
		t.Fatal("wrong enrollment binding")
	}
	call("POST", "/v1/enrollments/complete", "", string(proof), 400, nil)
	call("GET", "/v1/devices/"+device.ID, "owner", "", 200, nil)
	call("GET", "/v1/devices/"+device.ID, "other", "", 404, nil)
	call("GET", "/v1/devices/"+device.ID, "", "", 401, nil)
	call("POST", "/v1/enrollments", "", `{"unknown":true}`, 400, nil)
	call("POST", "/v1/enrollments", "", strings.Repeat("x", 17000), 400, nil)
	var authChallenge pairing.AuthChallenge
	call("POST", "/v1/device-auth/challenges", "", `{"deviceId":"`+device.ID+`"}`, 201, &authChallenge)
	raw, _ := base64.RawURLEncoding.DecodeString(authChallenge.Challenge)
	authDigest := sha256.Sum256(append([]byte("app-control-device-auth-v1\n"+device.ID+"\n"+authChallenge.ID+"\n"), raw...))
	authSignature, err := ecdsa.SignASN1(rand.Reader, key, authDigest[:])
	if err != nil {
		t.Fatal(err)
	}
	authProof, _ := json.Marshal(map[string]string{"deviceId": device.ID, "challengeId": authChallenge.ID, "challenge": authChallenge.Challenge, "signature": encode(authSignature)})
	var session pairing.DeviceSession
	call("POST", "/v1/device-auth/sessions", "", string(authProof), 201, &session)
	call("POST", "/v1/device-auth/sessions", "", string(authProof), 401, nil)
	call("GET", "/v1/device-auth/me", session.AccessToken, "", 200, nil)
	var command control.Command
	call("POST", "/v1/devices/"+device.ID+"/commands", "owner", `{"type":"LOCK_DEVICE","ttlSeconds":60}`, 201, &command)
	call("GET", "/v1/device/commands", "", "", 401, nil)
	call("GET", "/v1/device/commands", "owner", "", 401, nil)
	var batch struct {
		Commands []control.Command `json:"commands"`
	}
	call("GET", "/v1/device/commands", session.AccessToken, "", 200, &batch)
	if len(batch.Commands) != 1 || batch.Commands[0].ID != command.ID {
		t.Fatal("enrolled device could not fetch command")
	}
	call("GET", "/v1/devices/"+device.ID+"/commands/"+command.ID, "owner", "", 200, &command)
	if command.Status != "delivered" {
		t.Fatal("success reported before acknowledgement")
	}
	ackPath := "/v1/device/commands/" + command.ID + "/ack"
	call("POST", ackPath, "", `{"result":"succeeded"}`, 401, nil)
	call("POST", ackPath, session.AccessToken, `{"result":"unknown"}`, 400, nil)
	call("POST", ackPath, session.AccessToken, `{"result":"succeeded"}`, 204, nil)
	call("POST", ackPath, session.AccessToken, `{"result":"succeeded"}`, 204, nil)
	call("POST", ackPath, session.AccessToken, `{"result":"failed"}`, 409, nil)

	call("POST", "/v1/devices/"+device.ID+"/revoke", "other", "", 404, nil)
	call("GET", "/v1/device-auth/me", session.AccessToken, "", 200, nil)
	call("POST", "/v1/devices/"+device.ID+"/revoke", "", "", 401, nil)
	call("POST", "/v1/devices/"+device.ID+"/revoke", "owner", "", 204, nil)
	call("GET", "/v1/device/commands", session.AccessToken, "", 401, nil)
	call("GET", "/v1/device/policy", session.AccessToken, "", 401, nil)
	call("POST", "/v1/device/policy/report", session.AccessToken, `{"version":1,"result":"applied"}`, 401, nil)
	call("PUT", "/v1/device/apps", session.AccessToken, `{"apps":[]}`, 401, nil)
	call("POST", ackPath, session.AccessToken, `{"result":"succeeded"}`, 401, nil)

	call("GET", "/v1/device-auth/me", session.AccessToken, "", 401, nil)
	call("POST", "/v1/device-auth/challenges", "", `{"deviceId":"`+device.ID+`"}`, 401, nil)
}
