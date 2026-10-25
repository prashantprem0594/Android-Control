package httpapi

import (
	"appcontrol/server/internal/auth"
	"appcontrol/server/internal/control"
	"appcontrol/server/internal/storage"
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestCommandHistoryHTTPAuthorizationAndCursorValidation(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "history.db"))
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
	service := control.NewService(db)
	for index := 0; index < 2; index++ {
		if _, err := service.QueueLock(ctx, "owner", "device", 60); err != nil {
			t.Fatal(err)
		}
	}
	fake := completeFakeAuth()
	fake.authenticate = func(_ context.Context, token string) (auth.Parent, string, error) {
		if token == "owner" || token == "other" {
			return auth.Parent{ID: token}, "", nil
		}
		return auth.Parent{}, "", auth.ErrInvalidToken
	}
	handler := NewHandler(Options{Auth: fake, Control: service})
	call := func(query, token string, want int) control.CommandPage {
		t.Helper()
		r := httptest.NewRequest("GET", "/v1/devices/device/commands"+query, nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("history status=%d expected=%d", w.Code, want)
		}
		var page control.CommandPage
		if want == 200 {
			if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
				t.Fatal(err)
			}
		}
		return page
	}
	call("", "", 401)
	call("", "invalid", 401)
	call("", "other", 404)
	first := call("?limit=1", "owner", 200)
	if len(first.Commands) != 1 || first.NextCursor == "" || first.Commands[0].Status != "queued" {
		t.Fatal("first history page invalid")
	}
	second := call("?limit=1&after="+first.NextCursor, "owner", 200)
	if len(second.Commands) != 1 || second.NextCursor != "" || second.Commands[0].ID == first.Commands[0].ID {
		t.Fatal("history cursor lost or repeated command")
	}
	for _, query := range []string{"?limit=0", "?limit=101", "?limit=x", "?limit=1&limit=2", "?after=bad", "?after=", "?unknown=1", "?after=%zz"} {
		call(query, "owner", 400)
	}
}
