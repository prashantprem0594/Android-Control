package httpapi

import (
	"appcontrol/server/internal/auth"
	"appcontrol/server/internal/parentpush"
	"appcontrol/server/internal/storage"
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Revocation occurs after HTTP authentication but before the feed transaction.
// This exercises the real authorization boundary without timing-dependent sleeps.
type feedChangingAuthentication struct {
	AuthService
	afterAuthenticate func(context.Context, string)
}

func (a feedChangingAuthentication) Authenticate(ctx context.Context, token string) (auth.Parent, string, error) {
	parent, family, err := a.AuthService.Authenticate(ctx, token)
	if err == nil {
		a.afterAuthenticate(ctx, family)
	}
	return parent, family, err
}

func TestParentCallFeedRouteAuthenticatedOwnerOnlyAndPrivate(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "feed.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	authentication, err := auth.NewService(db, auth.Options{PasswordParameters: auth.PasswordParameters{MemoryKiB: 8192, Iterations: 1, Parallelism: 1, SaltBytes: 16, KeyBytes: 32}})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := authentication.Register(ctx, "owner@example.test", "fixture-owner-password")
	if err != nil {
		t.Fatal(err)
	}
	_, ownerSession, err := authentication.Login(ctx, owner.Email, "fixture-owner-password")
	if err != nil {
		t.Fatal(err)
	}
	other, err := authentication.Register(ctx, "other@example.test", "fixture-other-password")
	if err != nil {
		t.Fatal(err)
	}
	_, otherSession, err := authentication.Login(ctx, other.Email, "fixture-other-password")
	if err != nil {
		t.Fatal(err)
	}
	store, err := parentpush.NewStore(db, bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	stamp := now.Format(time.RFC3339Nano)
	callID := "a3d7a190-8503-4bf4-9c12-123456789abc"
	queries := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO devices(id,parent_account_id,display_name,public_key_der,key_algorithm,state,created_at) VALUES('device',?,'private-device-name',X'00','ECDSA_P256_SHA256','active',?)`, []any{owner.ID, stamp}},
		{`INSERT INTO remote_call_snapshots(device_id,parent_account_id,observed_at,calls_json,snapshot_sequence) VALUES('device',?,?,?,1)`, []any{owner.ID, stamp, `[{"callId":"` + callID + `","state":"RINGING","callerName":"private-caller-name"}]`}},
		{`INSERT INTO parent_call_events(event_id,device_id,parent_account_id,call_id,snapshot_sequence,observed_at,created_at_ns,expires_at_ns,status) VALUES('event','device',?,?,1,?,?,?,'active')`, []any{owner.ID, callID, stamp, now.UnixNano(), now.Add(30 * time.Second).UnixNano()}},
	}
	for _, q := range queries {
		if _, err := db.Exec(q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	var logs bytes.Buffer
	handler := NewHandler(Options{Auth: authentication, ParentWakeupTokens: store, Logger: slog.New(slog.NewJSONHandler(&logs, nil))})
	call := func(path, token string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("GET", path, nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("feed status %d, wanted %d", w.Code, want)
		}
		return w
	}
	const route = "/v1/parent/call-events"
	call(route, "", 401)
	call(route, "dt_"+strings.Repeat("A", 43), 401)
	call(route+"?parentId="+owner.ID, ownerSession.AccessToken, 400)
	w := call(route, ownerSession.AccessToken, 200)
	var feed parentpush.CallEventFeed
	if err := json.Unmarshal(w.Body.Bytes(), &feed); err != nil || len(feed.Events) != 1 || feed.Events[0].CallID != callID {
		t.Fatal("fresh owner feed missing")
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("feed cache policy missing")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &fields); err != nil || len(fields) != 2 || fields["serverTime"] == nil || fields["events"] == nil {
		t.Fatal("unexpected top-level event feed fields")
	}
	var events []map[string]json.RawMessage
	if err := json.Unmarshal(fields["events"], &events); err != nil || len(events) != 1 || len(events[0]) != 6 {
		t.Fatal("unexpected event metadata fields")
	}
	for _, field := range []string{"eventId", "deviceId", "callId", "snapshotSequence", "observedAt", "expiresAt"} {
		if events[0][field] == nil {
			t.Fatal("required event field missing or replaced")
		}
	}
	for _, private := range []string{"private-device-name", "private-caller-name", ownerSession.AccessToken} {
		if strings.Contains(w.Body.String(), private) || strings.Contains(logs.String(), private) {
			t.Fatal("private data exposed")
		}
	}
	w = call(route, otherSession.AccessToken, 200)
	if err := json.Unmarshal(w.Body.Bytes(), &feed); err != nil || len(feed.Events) != 0 {
		t.Fatal("cross-account feed exposed")
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("empty feed cache policy missing")
	}
	for _, scenario := range []string{"revoked-after-authentication", "expired-after-authentication"} {
		t.Run(scenario, func(t *testing.T) {
			_, session, err := authentication.Login(ctx, owner.Email, "fixture-owner-password")
			if err != nil {
				t.Fatal(err)
			}
			changed := false
			changing := feedChangingAuthentication{AuthService: authentication, afterAuthenticate: func(ctx context.Context, family string) {
				changed = true
				query := `UPDATE auth_session_families SET revoked_at=?,revocation_reason='logout' WHERE id=?`
				if scenario == "expired-after-authentication" {
					query = `UPDATE auth_session_families SET expires_at=? WHERE id=?`
				}
				if _, err := db.ExecContext(ctx, query, time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano), family); err != nil {
					t.Fatal(err)
				}
			}}
			r := httptest.NewRequest("GET", route, nil)
			r.Header.Set("Authorization", "Bearer "+session.AccessToken)
			w := httptest.NewRecorder()
			NewHandler(Options{Auth: changing, ParentWakeupTokens: store}).ServeHTTP(w, r)
			if !changed || w.Code != 401 || strings.Contains(w.Body.String(), callID) {
				t.Fatal("feed missed family state change after HTTP authentication")
			}
		})
	}
	if err := authentication.Logout(ctx, ownerSession.AccessToken); err != nil {
		t.Fatal(err)
	}
	call(route, ownerSession.AccessToken, 401)
	plain := NewHandler(Options{Auth: authentication})
	r := httptest.NewRequest("GET", route, nil)
	r.Header.Set("Authorization", "Bearer "+otherSession.AccessToken)
	missing := httptest.NewRecorder()
	plain.ServeHTTP(missing, r)
	if missing.Code != 404 {
		t.Fatal("feed enabled without parent registry configuration")
	}
}
