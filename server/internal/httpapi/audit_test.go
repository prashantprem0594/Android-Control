package httpapi

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"appcontrol/server/internal/audit"
	"appcontrol/server/internal/auth"
)

func TestAuditEndpointUsesAuthenticatedParentAndBoundsPages(t *testing.T) {
	fake := completeFakeAuth()
	fake.authenticate = func(_ context.Context, token string) (auth.Parent, string, error) {
		if token != "valid" {
			return auth.Parent{}, "", errors.New("invalid token")
		}
		return auth.Parent{ID: "owner"}, "", nil
	}
	calls := 0
	handler := NewHandler(Options{Auth: fake, Audit: func(_ context.Context, parentID string, after int64, limit int) ([]audit.Event, error) {
		calls++
		if parentID != "owner" || after != 7 || limit != 2 {
			t.Fatalf("wrong page scope: %q %d %d", parentID, after, limit)
		}
		return []audit.Event{{ID: 8, Action: "parent_login"}}, nil
	}})
	for _, tc := range []struct {
		path, token string
		status      int
	}{
		{"/v1/audit-events?after=7&limit=2&parentId=other", "valid", 200},
		{"/v1/audit-events", "", 401},
		{"/v1/audit-events", "invalid", 401},
		{"/v1/audit-events?after=-1", "valid", 400},
		{"/v1/audit-events?after=invalid", "valid", 400},
		{"/v1/audit-events?limit=101", "valid", 400},
		{"/v1/audit-events?limit=0", "valid", 400},
	} {
		request := httptest.NewRequest("GET", tc.path, nil)
		if tc.token != "" {
			request.Header.Set("Authorization", "Bearer "+tc.token)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != tc.status {
			t.Fatalf("%s: got %d want %d", tc.path, response.Code, tc.status)
		}
		if response.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("audit response must not be cached")
		}
	}
	if calls != 1 {
		t.Fatalf("unauthorized or invalid page reached audit storage: %d", calls)
	}
}

func TestAuditStorageFailureReturnsGenericError(t *testing.T) {
	fake := completeFakeAuth()
	fake.authenticate = func(context.Context, string) (auth.Parent, string, error) {
		return auth.Parent{ID: "owner"}, "", nil
	}
	handler := NewHandler(Options{Auth: fake, Audit: func(_ context.Context, parentID string, after int64, limit int) ([]audit.Event, error) {
		if parentID != "owner" || after != 0 || limit != 50 {
			t.Fatal("incorrect default page or parent scope")
		}
		return nil, errors.New("private database diagnostic")
	}})
	request := httptest.NewRequest("GET", "/v1/audit-events", nil)
	request.Header.Set("Authorization", "Bearer valid")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 500 || response.Body.String() != "{\"error\":\"server_error\"}\n" {
		t.Fatalf("unexpected storage failure response: %d %s", response.Code, response.Body.String())
	}
}
