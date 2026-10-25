package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"appcontrol/server/internal/audit"
	"appcontrol/server/internal/auth"
	"appcontrol/server/internal/control"
	"appcontrol/server/internal/storage"
)

func TestControlHTTPAuditAndAuthorization(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "http-controls.db"))
	if err != nil {
		t.Fatal(err)
	}

	defer db.Close()
	for _, query := range []string{
		`INSERT INTO parent_accounts(id,email,password_hash,created_at,updated_at) VALUES('owner','owner@example.com','unused','2026-10-23T00:00:00Z','2026-10-23T00:00:00Z')`,
		`INSERT INTO devices(id,parent_account_id,display_name,public_key_der,key_algorithm,state,created_at) VALUES('device','owner','test',X'00','ECDSA_P256_SHA256','active','2026-10-23T00:00:00Z')`,
	} {
		if _, err = db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	fake := completeFakeAuth()
	fake.authenticate = func(_ context.Context, token string) (auth.Parent, string, error) {
		if token == "owner" || token == "other" {
			return auth.Parent{ID: token}, "", nil
		}
		return auth.Parent{}, "", errors.New("bad token")
	}
	controlService := control.NewService(db)
	controlService.SetRemoteCallsEnabled(true)
	handler := NewHandler(Options{Auth: fake, Control: controlService, Audit: func(ctx context.Context, parent string, after int64, limit int) ([]audit.Event, error) {
		return audit.List(ctx, db, parent, after, limit)
	}})
	call := func(method, path, token, body string, want int) []byte {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		return w.Body.Bytes()
	}
	commandPath := "/v1/devices/device/commands"
	policyPath := "/v1/devices/device/policy"
	lock := `{"type":"LOCK_DEVICE","ttlSeconds":60}`
	callID := "a3d7a190-8503-4bf4-9c12-123456789abc"
	answerCall := `{"type":"ANSWER_CALL","callId":"` + callID + `","ttlSeconds":30}`
	if _, err = db.Exec(`INSERT INTO remote_call_snapshot_counters(device_id,sequence) VALUES('device',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO remote_call_snapshots(device_id,parent_account_id,observed_at,calls_json,snapshot_sequence)
		VALUES('device','owner',?,?,1)`, time.Now().UTC().Format(time.RFC3339Nano), `[{"callId":"`+callID+`","state":"RINGING","remoteActionsAllowed":true}]`); err != nil {
		t.Fatal(err)
	}
	policy := `{"expectedVersion":0,"internetPaused":true,"blockedPackages":["org.example.privateapp"]}`
	for _, token := range []string{"", "invalid"} {
		call("POST", commandPath, token, lock, 401)
		call("PUT", policyPath, token, policy, 401)
	}
	call("POST", commandPath, "other", lock, 404)
	call("POST", commandPath, "other", answerCall, 404)
	call("PUT", policyPath, "other", policy, 404)
	for _, body := range []string{
		`null`,
		`{"type":"ANSWER_CALL","ttlSeconds":60}`,
		`{"type":"END_CALL","callId":"` + callID + `","ttlSeconds":31}`,
		`{"type":"REJECT_CALL","callId":"not-a-call","ttlSeconds":10}`,
		`{"type":"END_CALL","callId":"` + callID + `","ttlSeconds":10,"phoneNumber":"+15555550123"}`,
		`{"type":"LOCK_DEVICE","ttlSeconds":0}`,
		`{"type":"LOCK_DEVICE","ttlSeconds":60,"parentId":"owner"}`,
		lock + lock, strings.Repeat("x", 17000),
	} {
		call("POST", commandPath, "owner", body, 400)
	}
	var callCommand control.Command
	if err = json.Unmarshal(call("POST", commandPath, "owner", answerCall, 201), &callCommand); err != nil ||
		callCommand.Type != "ANSWER_CALL" || string(callCommand.Payload) != `{"callId":"`+callID+`"}` {
		t.Fatal("call command contains unexpected fields")
	}
	var command control.Command
	if err = json.Unmarshal(call("POST", commandPath, "owner", lock, 201), &command); err != nil || command.Status != "queued" {
		t.Fatal("invalid command response")
	}
	call("GET", commandPath+"/"+command.ID, "owner", "", 200)
	call("GET", commandPath+"/"+command.ID, "other", "", 404)
	call("GET", policyPath, "owner", "", 200)
	call("GET", policyPath, "other", "", 404)
	call("PUT", policyPath, "owner", `{"expectedVersion":0}`, 400)
	call("PUT", policyPath, "owner", policy, 200)
	var desired control.Policy
	if err = json.Unmarshal(call("GET", policyPath, "owner", "", 200), &desired); err != nil || desired.Version != 1 || !desired.InternetPaused || len(desired.BlockedPackages) != 1 {
		t.Fatal("desired policy read failed")
	}
	call("PUT", policyPath, "owner", policy, 409)
	var originalID string
	for attempt := 0; attempt < 2; attempt++ {
		r := httptest.NewRequest("POST", commandPath, strings.NewReader(lock))
		r.Header.Set("Authorization", "Bearer owner")
		r.Header.Set("Idempotency-Key", "http-request-key-0001")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		var queued control.Command
		if err = json.Unmarshal(w.Body.Bytes(), &queued); err != nil || w.Code != 201 {
			t.Fatal("keyed HTTP command failed")
		}
		if attempt == 0 {
			originalID = queued.ID
		} else if queued.ID != originalID {
			t.Fatal("HTTP creation retry duplicated command")
		}
	}
	var history struct {
		Events []audit.Event `json:"events"`
	}
	if err = json.Unmarshal(call("GET", "/v1/audit-events", "owner", "", 200), &history); err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, event := range history.Events {
		found[event.Action] = true
	}
	if !found["command_created"] || !found["policy_updated"] {
		t.Fatal("control actions absent from audit endpoint")
	}
	if strings.Contains(string(call("GET", "/v1/audit-events", "owner", "", 200)), "org.example.privateapp") {
		t.Fatal("policy leaked to audit")
	}
	if err = json.Unmarshal(call("GET", "/v1/audit-events", "other", "", 200), &history); err != nil || len(history.Events) != 0 {
		t.Fatal("audit crossed accounts")
	}
	if _, err = db.Exec(`CREATE TRIGGER test_control_audit_failure BEFORE INSERT ON audit_events BEGIN SELECT RAISE(ABORT,'private error'); END`); err != nil {
		t.Fatal(err)
	}
	call("POST", commandPath, "owner", lock, 500)
	call("PUT", policyPath, "owner", `{"expectedVersion":1,"internetPaused":false,"blockedPackages":[]}`, 500)
}

func TestRemoteCallCommandHTTPAuthorizationAndPayload(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "remote-call.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, query := range []string{
		`INSERT INTO parent_accounts(id,email,password_hash,created_at,updated_at) VALUES('owner','owner@example.com','unused','2026-10-23T00:00:00Z','2026-10-23T00:00:00Z')`,
		`INSERT INTO devices(id,parent_account_id,display_name,public_key_der,key_algorithm,state,created_at) VALUES('device','owner','test',X'00','ECDSA_P256_SHA256','active','2026-10-23T00:00:00Z')`,
	} {
		if _, err = db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	callID := "a3d7a190-8503-4bf4-9c12-123456789abc"
	if _, err = db.Exec(`INSERT INTO remote_call_snapshots(device_id,parent_account_id,observed_at,calls_json,snapshot_sequence)
		VALUES('device','owner',?,?,1)`, time.Now().UTC().Format(time.RFC3339Nano), `[{"callId":"`+callID+`","state":"ACTIVE","remoteActionsAllowed":true}]`); err != nil {
		t.Fatal(err)
	}
	fake := completeFakeAuth()
	fake.authenticate = func(_ context.Context, token string) (auth.Parent, string, error) {
		switch token {
		case "owner":
			return auth.Parent{ID: "owner"}, "", nil
		case "other":
			return auth.Parent{ID: "other"}, "", nil
		default:
			return auth.Parent{}, "", errors.New("bad token")
		}
	}
	controlService := control.NewService(db)
	controlService.SetRemoteCallsEnabled(true)
	handler := NewHandler(Options{Auth: fake, Control: controlService})
	call := func(token, body string, want int) []byte {
		t.Helper()
		request := httptest.NewRequest("POST", "/v1/devices/device/commands", strings.NewReader(body))
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != want {
			t.Fatalf("status=%d want=%d body=%s", response.Code, want, response.Body.String())
		}
		return response.Body.Bytes()
	}
	body := `{"type":"END_CALL","callId":"` + callID + `","ttlSeconds":20}`
	call("", body, 401)
	call("other", body, 404)
	call("owner", `{"type":"ANSWER_CALL","callId":"`+callID+`","ttlSeconds":31}`, 400)
	call("owner", `{"type":"REJECT_CALL","callId":"`+callID+`","ttlSeconds":20,"contactName":"private"}`, 400)
	call("owner", `{"type":"ANSWER_CALL","callId":"`+callID+`","ttlSeconds":20}`, 409)
	var command control.Command
	if err = json.Unmarshal(call("owner", body, 201), &command); err != nil {
		t.Fatal(err)
	}
	if command.Type != "END_CALL" || command.Status != "queued" ||
		string(command.Payload) != `{"callId":"`+callID+`"}` ||
		command.ExpiresAt.Sub(command.IssuedAt) != 20*time.Second {
		t.Fatalf("unexpected command envelope: %+v", command)
	}
	if _, err = control.NewService(db).Command(ctx, "other", "device", command.ID); !errors.Is(err, control.ErrNotFound) {
		t.Fatal("cross-parent command inspection accepted")
	}
}
