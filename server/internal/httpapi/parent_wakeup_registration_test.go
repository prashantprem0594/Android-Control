package httpapi

import (
	"appcontrol/server/internal/auth"
	"appcontrol/server/internal/parentpush"
	"appcontrol/server/internal/storage"
	"bytes"
	"context"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParentWakeupRegistrationUsesAuthenticatedAccountAndFamily(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "parent-wakeup.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	authentication, err := auth.NewService(db, auth.Options{
		Now: func() time.Time { return now },
		PasswordParameters: auth.PasswordParameters{
			MemoryKiB: 8192, Iterations: 1, Parallelism: 1, SaltBytes: 16, KeyBytes: 32,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := authentication.Register(ctx, "parent@example.com", "fixture-parent-password")
	if err != nil {
		t.Fatal(err)
	}
	_, session, err := authentication.Login(ctx, parent.Email, "fixture-parent-password")
	if err != nil {
		t.Fatal(err)
	}
	_, familyID, err := authentication.Authenticate(ctx, session.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	store, err := parentpush.NewStore(db, bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	handler := NewHandler(Options{Auth: authentication, ParentWakeupTokens: store,
		Logger: slog.New(slog.NewJSONHandler(&logs, nil))})
	call := func(method, path, bearer, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		if bearer != "" {
			request.Header.Set("Authorization", "Bearer "+bearer)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != want {
			t.Fatalf("%s %s returned %d, wanted %d", method, path, response.Code, want)
		}
		return response
	}

	const route = "/v1/parent/wakeup-registration"
	const registration = "private-parent-routing-fixture"
	call("PUT", route, "", `{"registrationToken":"`+registration+`"}`, 401)
	call("PUT", route, "dt_"+strings.Repeat("A", 43), `{"registrationToken":"`+registration+`"}`, 401)
	for _, body := range []string{
		`null`, `{}`, `{"registrationToken":""}`,
		`{"registrationToken":"` + registration + `","parentId":"another-account"}`,
		`{"registrationToken":"` + registration + `"}` + `{"registrationToken":"second"}`,
	} {
		call("PUT", route, session.AccessToken, body, 400)
	}
	response := call("PUT", route, session.AccessToken, `{"registrationToken":"`+registration+`"}`, 204)
	if strings.Contains(response.Body.String(), registration) {
		t.Fatal("registration token returned to caller")
	}
	if count := func() int {
		var value int
		if err := db.QueryRow(`SELECT COUNT(*) FROM parent_wakeup_registrations WHERE parent_account_id=? AND family_id=?`,
			parent.ID, familyID).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}(); count != 1 {
		t.Fatal("authenticated registration not persisted")
	}
	call("GET", route, session.AccessToken, "", 405)
	call("DELETE", route, "", "", 401)
	call("DELETE", route, session.AccessToken, "", 204)
	if strings.Contains(logs.String(), registration) || strings.Contains(logs.String(), session.AccessToken) {
		t.Fatal("parent registration or bearer token leaked to request logs")
	}
}
