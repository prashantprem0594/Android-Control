package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"appcontrol/server/internal/auth"
	"appcontrol/server/internal/control"
	"appcontrol/server/internal/storage"
)

type disconnectedResponse struct{ headers http.Header }

func (w *disconnectedResponse) Header() http.Header       { return w.headers }
func (w *disconnectedResponse) WriteHeader(int)           {}
func (w *disconnectedResponse) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestDeviceCommandsOverHTTPS(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "tls.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`INSERT INTO parent_accounts(id,email,password_hash,created_at,updated_at) VALUES('owner','owner@example.com','unused','2026-10-23T00:00:00Z','2026-10-23T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"device", "other"} {
		if _, err = db.Exec(`INSERT INTO devices(id,parent_account_id,display_name,public_key_der,key_algorithm,state,created_at) VALUES(?,'owner','test',X'00','ECDSA_P256_SHA256','active','2026-10-23T00:00:00Z')`, id); err != nil {
			t.Fatal(err)
		}
	}
	token := "dt_" + strings.Repeat("A", 43)
	hash := sha256.Sum256([]byte(token))
	now := time.Now().UTC()
	if _, err = db.Exec("INSERT INTO device_sessions(token_hash,device_id,expires_at,issued_at) VALUES(?,'device',?,?)", hash[:], now.Add(time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	s := control.NewService(db)
	command, err := s.QueueLock(ctx, "owner", "device", 60)
	if err != nil {
		t.Fatal(err)
	}
	fake := completeFakeAuth()
	fake.authenticate = func(_ context.Context, bearer string) (auth.Parent, string, error) {
		if bearer == "test-owner" {
			return auth.Parent{ID: "owner"}, "", nil
		}
		return auth.Parent{}, "", errors.New("invalid token")
	}
	server := httptest.NewTLSServer(NewHandler(Options{Auth: fake, Control: s}))
	defer server.Close()
	client := server.Client()
	client.Timeout = 10 * time.Second
	call := func(method, path, bearer, body string, want int, policyFormat ...string) []byte {
		t.Helper()
		r, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		if len(policyFormat) > 0 {
			r.Header.Set("X-Policy-Format", policyFormat[0])
		}
		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != want {
			t.Fatalf("%s status=%d want=%d", path, response.StatusCode, want)
		}
		var raw json.RawMessage
		if want == 200 {
			if err = json.NewDecoder(response.Body).Decode(&raw); err != nil {
				t.Fatal(err)
			}
		}
		return raw
	}
	call("GET", "/v1/device/commands", "", "", 401)
	call("PUT", "/v1/device/apps", "", `{"apps":[]}`, 401)
	call("PUT", "/v1/device/apps", token, `{"apps":null}`, 400)
	call("PUT", "/v1/device/apps", token, `{"apps":[{"packageName":"org.example.latest","label":"Latest","blockable":true}]}`, 204)
	call("GET", "/v1/device/policy", "", "", 401)
	if _, err = s.UpdatePolicy(ctx, "owner", "device", 0, true, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = s.UpdatePolicy(ctx, "owner", "device", 1, false, []string{"org.example.latest"}); err != nil {
		t.Fatal(err)
	}
	var policy control.Policy
	if err = json.Unmarshal(call("GET", "/v1/device/policy", token, "", 200), &policy); err != nil || policy.Version != 2 || policy.InternetPaused {
		t.Fatal("HTTPS did not return latest desired policy")
	}
	call("POST", "/v1/device/policy/report", "", `{"version":2,"result":"applied"}`, 401)
	call("POST", "/v1/device/policy/report", token, `{"version":1,"result":"applied"}`, 409)
	call("POST", "/v1/device/policy/report", token, `{"version":2,"result":"applied","reason":"private text"}`, 400)
	call("POST", "/v1/device/policy/report", token, `{"version":2,"result":"applied"}`, 204)
	reported, err := s.Policy(ctx, "owner", "device")
	if err != nil || reported.Reported == nil || reported.Reported.Version != 2 || reported.Reported.Result != "applied" {
		t.Fatal("HTTPS policy result missing")
	}
	var batch struct {
		Commands []control.Command `json:"commands"`
	}
	if err = json.Unmarshal(call("GET", "/v1/device/commands", token, "", 200), &batch); err != nil || len(batch.Commands) != 1 || batch.Commands[0].ID != command.ID {
		t.Fatal("HTTPS fetch failed")
	}
	pending, err := s.Command(ctx, "owner", "device", command.ID)
	if err != nil || pending.Status != "delivered" {
		t.Fatal("HTTPS fetch implied execution")
	}
	call("POST", "/v1/device/commands/"+command.ID+"/ack", token, `{"result":"failed"}`, 204)
	result, err := s.Command(ctx, "owner", "device", command.ID)
	if err != nil || result.Status != "failed" {
		t.Fatal("HTTPS result missing")
	}
	interrupted, err := s.QueueLock(ctx, "owner", "device", 60)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/v1/device/commands", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	NewHandler(Options{Auth: completeFakeAuth(), Control: s}).ServeHTTP(&disconnectedResponse{headers: make(http.Header)}, r)
	stillPending, err := s.Command(ctx, "owner", "device", interrupted.ID)
	if err != nil || stillPending.Status != "delivered" {
		t.Fatal("interrupted delivery reported terminal state")
	}
	if err = json.Unmarshal(call("GET", "/v1/device/commands", token, "", 200), &batch); err != nil || len(batch.Commands) != 1 || batch.Commands[0].ID != interrupted.ID {
		t.Fatal("interrupted response lost command")
	}
	call("POST", "/v1/device/commands/"+interrupted.ID+"/ack", token, `{"result":"succeeded"}`, 204)
	confirmed, err := s.Command(ctx, "owner", "device", interrupted.ID)
	if err != nil || confirmed.Status != "succeeded" {
		t.Fatal("parent did not reflect authenticated acknowledgement")
	}
	scheduledBody := `{"expectedVersion":2,"internetPaused":false,"blockedPackages":[],"alwaysAllowedPackages":["org.example.latest"],"schedules":[{"id":"school","timeZone":"UTC","daysOfWeek":[1,2,3,4,5,6,7],"startMinute":0,"endMinute":1440,"internetPaused":true,"blockedPackages":["org.example.latest"]}]}`
	call("PUT", "/v1/devices/device/policy", "test-owner", scheduledBody, 200)
	call("GET", "/v1/device/policy", token, "", 409)
	var scheduled control.Policy
	if err = json.Unmarshal(call("GET", "/v1/device/policy", token, "", 200, "2"), &scheduled); err != nil || scheduled.FormatVersion != 2 || len(scheduled.Schedules) != 1 {
		t.Fatal("HTTPS capability negotiation lost schedules")
	}
	call("POST", "/v1/device/policy/report", token, `{"version":3,"result":"applied"}`, 409)
	var ownerSnapshot control.Policy
	if err = json.Unmarshal(call("GET", "/v1/devices/device/policy", "test-owner", "", 200), &ownerSnapshot); err != nil || len(ownerSnapshot.EffectiveStateKey) != 64 {
		t.Fatal("parent effective-state key missing")
	}
	call("POST", "/v1/device/policy/report", token, `{"version":3,"result":"applied","stateKey":"`+ownerSnapshot.EffectiveStateKey+`"}`, 204)
	if err = json.Unmarshal(call("GET", "/v1/devices/device/policy", "test-owner", "", 200), &ownerSnapshot); err != nil || ownerSnapshot.Reported == nil || ownerSnapshot.Reported.StateKey != ownerSnapshot.EffectiveStateKey {
		t.Fatal("scheduled state not confirmed independently")
	}
}
