package httpapi

import (
	"appcontrol/server/internal/control"
	"appcontrol/server/internal/storage"
	"bytes"
	"context"
	"crypto/sha256"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWakeupRegistrationHTTPDeviceAuthorizationAndPrivateLogs(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "routing.db"))
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
	token := "dt_" + strings.Repeat("R", 43)
	hash := sha256.Sum256([]byte(token))
	if _, err := db.Exec("INSERT INTO device_sessions(token_hash,device_id,expires_at,issued_at) VALUES(?,?,?,?)", hash[:], "device", time.Now().Add(time.Hour).Format(time.RFC3339Nano), time.Now().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	store, err := control.NewWakeupTokens(db, bytes.Repeat([]byte{3}, 32))
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	handler := NewHandler(Options{WakeupTokens: store, Logger: slog.New(slog.NewJSONHandler(&logs, nil))})
	call := func(method, credential, body string, want int) {
		t.Helper()
		r := httptest.NewRequest(method, "/v1/device/wakeup-registration", strings.NewReader(body))
		if credential != "" {
			r.Header.Set("Authorization", "Bearer "+credential)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("registration status %d expected %d", w.Code, want)
		}
	}
	valid := `{"registrationToken":"private-routing-fixture"}`
	call("PUT", "", valid, 401)
	call("PUT", "invalid", valid, 401)
	for _, body := range []string{`null`, `{}`, `{"registrationToken":""}`, `{"registrationToken":"bad\nvalue"}`, `{"registrationToken":"x","deviceId":"another"}`, valid + valid} {
		call("PUT", token, body, 400)
	}
	call("PUT", token, valid, 204)
	if value, err := store.Registration(ctx, "device"); err != nil || value != "private-routing-fixture" {
		t.Fatal("routing registration unavailable")
	}
	call("DELETE", "invalid", "", 401)
	call("DELETE", token, "", 204)
	if _, err := store.Registration(ctx, "device"); err != control.ErrNotFound {
		t.Fatal("routing removal failed")
	}
	for _, private := range []string{token, "private-routing-fixture", "registrationToken"} {
		if strings.Contains(logs.String(), private) {
			t.Fatal("routing secret leaked to request log")
		}
	}
	disabled := NewHandler(Options{})
	r := httptest.NewRequest("PUT", "/v1/device/wakeup-registration", strings.NewReader(valid))
	w := httptest.NewRecorder()
	disabled.ServeHTTP(w, r)
	if w.Code != 404 {
		t.Fatal("registration endpoint enabled without configured provider")
	}
}
