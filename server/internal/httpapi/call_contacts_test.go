package httpapi

import (
	"appcontrol/server/internal/auth"
	"appcontrol/server/internal/control"
	"appcontrol/server/internal/storage"
	"context"
	"crypto/sha256"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCallContactsHTTPStrictPayloadAndAuthorization(t *testing.T) {
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "contacts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, query := range []string{
		`INSERT INTO parent_accounts(id,email,password_hash,created_at,updated_at) VALUES('owner','owner@example.com','unused','2026-10-24T00:00:00Z','2026-10-24T00:00:00Z')`,
		`INSERT INTO devices(id,parent_account_id,display_name,public_key_der,key_algorithm,state,created_at) VALUES('device','owner','test',X'00','ECDSA_P256_SHA256','active','2026-10-24T00:00:00Z')`,
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	token := "dt_" + strings.Repeat("C", 43)
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
	call := func(method, path, token, body string, want int) {
		t.Helper()
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != want {
			t.Fatalf("%s %s: got %d want %d", method, path, recorder.Code, want)
		}
	}
	path := "/v1/devices/device/call-contacts"
	call("GET", path, "", "", 401)
	call("GET", path, "other", "", 404)
	call("GET", "/v1/device/call-contacts", "owner", "", 401)
	for _, body := range []string{
		`{}`, `{"expectedVersion":0,"contacts":null}`, `{"expectedVersion":0,"contacts":[],"unknown":1}`,
		`{"expectedVersion":0,"contacts":[{"label":"Parent","number":"+919876543210"}]}`,
		`{"expectedVersion":0,"contacts":[{"label":"Parent","number":"+919876543210","isParent":false,"extra":1}]}`,
	} {
		call("PUT", path, "owner", body, 400)
	}
	body := `{"expectedVersion":0,"contacts":[{"label":"Parent","number":"+919876543210","isParent":true}]}`
	call("PUT", path, "other", body, 404)
	call("PUT", path, "owner", body, 200)
	call("PUT", path, "owner", body, 409)
	call("GET", "/v1/device/call-contacts", token, "", 200)
	call("PUT", path, token, `{"expectedVersion":1,"contacts":[]}`, 401)
}
