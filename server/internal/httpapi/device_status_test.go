package httpapi

import (
	"appcontrol/server/internal/auth"
	"appcontrol/server/internal/control"
	"appcontrol/server/internal/storage"
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDeviceStatusHTTPRejectsMissingAndCrossParentInputs(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "status.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, query := range []string{
		`INSERT INTO parent_accounts(id,email,password_hash,created_at,updated_at) VALUES('owner','owner@example.com','unused','2026-10-23T00:00:00Z','2026-10-23T00:00:00Z')`,
		`INSERT INTO devices(id,parent_account_id,display_name,public_key_der,key_algorithm,state,created_at) VALUES('device','owner','test',X'00','ECDSA_P256_SHA256','active','2026-10-23T00:00:00Z')`,
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	token := "dt_" + strings.Repeat("T", 43)
	hash := sha256.Sum256([]byte(token))
	if _, err := db.Exec("INSERT INTO device_sessions(token_hash,device_id,expires_at,issued_at) VALUES(?,?,?,?)", hash[:], "device", time.Now().Add(time.Hour).Format(time.RFC3339Nano), time.Now().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	fake := completeFakeAuth()
	fake.authenticate = func(_ context.Context, value string) (auth.Parent, string, error) {
		if value == "owner" || value == "other" {
			return auth.Parent{ID: value}, "", nil
		}
		return auth.Parent{}, "", auth.ErrInvalidToken
	}
	handler := NewHandler(Options{Auth: fake, Control: control.NewService(db)})
	call := func(method, path, token, body string, want int) []byte {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s status %d want %d", path, w.Code, want)
		}
		return w.Body.Bytes()
	}
	call("GET", "/v1/devices/device/status", "", "", 401)
	call("GET", "/v1/devices/device/status", "other", "", 404)
	var initial control.DeviceStatus
	if json.Unmarshal(call("GET", "/v1/devices/device/status", "owner", "", 200), &initial) != nil || initial.ServerTime.IsZero() || initial.ReceivedAt != nil || initial.Capabilities != nil {
		t.Fatal("unreported status fabricated capabilities or omitted server clock")
	}
<<<<<<< HEAD
	valid := `{"androidApi":37,"deviceOwner":true,"dialerRole":false,"vpnConsent":false,"exactAlarms":true,"networkConnected":true,"wifiConnected":true,"cellularConnected":false,"vpnActive":false}`
	call("POST", "/v1/device/status", "", valid, 401)
	call("POST", "/v1/device/status", "owner", valid, 401)
	for _, body := range []string{`null`, `{}`, `{"androidApi":37}`, strings.Replace(valid, `"dialerRole":false`, `"dialerRole":null`, 1), strings.Replace(valid, `37`, `101`, 1), strings.Replace(valid, `37`, `20`, 1), strings.TrimSuffix(valid, "}") + `,"parentId":"owner"}`, valid + valid} {
=======
	valid := `{"androidApi":37,"deviceOwner":true,"dialerRole":false,"vpnConsent":false,"exactAlarms":true}`
	call("POST", "/v1/device/status", "", valid, 401)
	call("POST", "/v1/device/status", "owner", valid, 401)
	for _, body := range []string{`null`, `{}`, `{"androidApi":37}`, strings.Replace(valid, `"dialerRole":false`, `"dialerRole":null`, 1), strings.Replace(valid, `37`, `101`, 1), strings.Replace(valid, `37`, `32`, 1), strings.TrimSuffix(valid, "}") + `,"parentId":"owner"}`, valid + valid} {
>>>>>>> origin/main
		call("POST", "/v1/device/status", token, body, 400)
	}
	call("POST", "/v1/device/status", token, valid, 200)
	var status control.DeviceStatus
<<<<<<< HEAD
	if err := json.Unmarshal(call("GET", "/v1/devices/device/status", "owner", "", 200), &status); err != nil || status.ReceivedAt == nil || status.Capabilities == nil || status.Capabilities.AndroidAPI != 37 || !status.Capabilities.NetworkConnected || !status.Capabilities.WiFiConnected || status.Capabilities.CellularConnected || status.Capabilities.VPNActive {
=======
	if err := json.Unmarshal(call("GET", "/v1/devices/device/status", "owner", "", 200), &status); err != nil || status.ReceivedAt == nil || status.Capabilities == nil || status.Capabilities.AndroidAPI != 37 {
>>>>>>> origin/main
		t.Fatal("missing reported status")
	}
	callID := "a3d7a190-8503-4bf4-9c12-123456789abc"
	call("POST", "/v1/device/calls/sequence", "", `{}`, 401)
	var sequence1, sequence2 struct {
		Sequence int64 `json:"sequence"`
	}
	if err := json.Unmarshal(call("POST", "/v1/device/calls/sequence", token, `{}`, 200), &sequence1); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(call("POST", "/v1/device/calls/sequence", token, `{}`, 200), &sequence2); err != nil ||
		sequence2.Sequence <= sequence1.Sequence {
		t.Fatal("server did not allocate increasing call snapshot sequences")
	}
	callReport := `{"sequence":` + strconv.FormatInt(sequence2.Sequence, 10) +
		`,"calls":[{"callId":"` + callID + `","state":"RINGING","remoteActionsAllowed":true}]}`
	call("POST", "/v1/device/calls", "", callReport, 401)
	for _, body := range []string{
		`{}`, `{"calls":[{"callId":"private","state":"RINGING"}]}`,
		`{"sequence":` + strconv.FormatInt(sequence2.Sequence, 10) + `,"calls":[{"callId":"` + callID + `","state":"RINGING"}]}`,
		`{"sequence":` + strconv.FormatInt(sequence2.Sequence, 10) + `,"calls":[{"callId":"` + callID + `","state":"RINGING","remoteActionsAllowed":false,"phoneNumber":"555"}]}`,
	} {
		call("POST", "/v1/device/calls", token, body, 400)
	}
	call("POST", "/v1/device/calls", token, callReport, 200)
	olderReport := `{"sequence":` + strconv.FormatInt(sequence1.Sequence, 10) +
		`,"calls":[{"callId":"` + callID + `","state":"DISCONNECTED","remoteActionsAllowed":true}]}`
	var olderResult struct {
		Accepted bool `json:"accepted"`
	}
	if err := json.Unmarshal(call("POST", "/v1/device/calls", token, olderReport, 200), &olderResult); err != nil || olderResult.Accepted {
		t.Fatal("out-of-order call snapshot overwrote a newer snapshot")
	}
	var calls control.RemoteCalls
	if err := json.Unmarshal(call("GET", "/v1/devices/device/calls", "owner", "", 200), &calls); err != nil ||
		len(calls.Calls) != 1 || calls.Calls[0].CallID != callID || calls.Calls[0].State != "RINGING" ||
		calls.Calls[0].RemoteActionsAllowed || calls.ObservedAt == nil || calls.SnapshotSequence != sequence2.Sequence {
		t.Fatal("call snapshot was not reported as opaque ID and state")
	}
	call("GET", "/v1/devices/device/calls", "other", "", 404)
	emptySequence, err := control.NewService(db).AllocateRemoteCallSequence(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	call("POST", "/v1/device/calls", token, `{"sequence":`+strconv.FormatInt(emptySequence, 10)+`,"calls":[]}`, 200)
	var cleared control.RemoteCalls
	if err := json.Unmarshal(call("GET", "/v1/devices/device/calls", "owner", "", 200), &cleared); err != nil || len(cleared.Calls) != 0 {
		t.Fatal("empty call snapshot did not clear prior calls")
	}
	if _, err := db.Exec("UPDATE devices SET state='revoked',revoked_at=?,revocation_reason='parent_revoked' WHERE id='device'", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	call("POST", "/v1/device/status", token, valid, 401)
	call("POST", "/v1/device/calls", token, callReport, 401)
}
