package httpapi

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"appcontrol/server/internal/auth"
)

type fakeAuthService struct {
	register     func(context.Context, string, string) (auth.Parent, error)
	login        func(context.Context, string, string) (auth.Parent, auth.TokenPair, error)
	refresh      func(context.Context, string) (auth.TokenPair, error)
	authenticate func(context.Context, string) (auth.Parent, string, error)
	logout       func(context.Context, string) error
}

func (fake fakeAuthService) Register(ctx context.Context, email, password string) (auth.Parent, error) {
	return fake.register(ctx, email, password)
}
func (fake fakeAuthService) Login(ctx context.Context, email, password string) (auth.Parent, auth.TokenPair, error) {
	return fake.login(ctx, email, password)
}
func (fake fakeAuthService) Refresh(ctx context.Context, token string) (auth.TokenPair, error) {
	return fake.refresh(ctx, token)
}
func (fake fakeAuthService) Authenticate(ctx context.Context, token string) (auth.Parent, string, error) {
	return fake.authenticate(ctx, token)
}
func (fake fakeAuthService) Logout(ctx context.Context, token string) error {
	return fake.logout(ctx, token)
}

func completeFakeAuth() fakeAuthService {
	return fakeAuthService{
		register: func(context.Context, string, string) (auth.Parent, error) { return auth.Parent{}, nil },
		login: func(context.Context, string, string) (auth.Parent, auth.TokenPair, error) {
			return auth.Parent{}, auth.TokenPair{}, nil
		},
		refresh:      func(context.Context, string) (auth.TokenPair, error) { return auth.TokenPair{}, nil },
		authenticate: func(context.Context, string) (auth.Parent, string, error) { return auth.Parent{}, "", nil },
		logout:       func(context.Context, string) error { return nil },
	}
}

func TestLoginEndpointReturnsSessionWithoutLoggingCredentialsOrTokens(t *testing.T) {
	const password = "secret-password-value"
	const accessToken = "at_secret-access-value"
	const refreshToken = "rt_secret-refresh-value"
	var logs bytes.Buffer
	fake := completeFakeAuth()
	fake.login = func(_ context.Context, email, suppliedPassword string) (auth.Parent, auth.TokenPair, error) {
		if email != "parent@example.com" || suppliedPassword != password {
			t.Fatalf("handler supplied wrong credentials")
		}
		return auth.Parent{ID: "par_1", Email: email}, auth.TokenPair{
			AccessToken: accessToken, AccessExpiresAt: time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC),
			RefreshToken: refreshToken, RefreshExpiresAt: time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC),
		}, nil
	}
	handler := NewHandler(Options{Auth: fake, Logger: slog.New(slog.NewJSONHandler(&logs, nil))})
	request := httptest.NewRequest(http.MethodPost, "/v1/auth/login?debug=secret-query", strings.NewReader(`{"email":"parent@example.com","password":"`+password+`"}`))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), accessToken) || !strings.Contains(response.Body.String(), refreshToken) {
		t.Fatalf("unexpected login response: status=%d body=%s", response.Code, response.Body.String())
	}
	logged := logs.String()
	for _, secret := range []string{password, accessToken, refreshToken, "secret-query", "parent@example.com"} {
		if strings.Contains(logged, secret) {
			t.Fatalf("request log leaked sensitive value %q: %s", secret, logged)
		}
	}
}

func TestLoginEndpointMapsInvalidCredentialsAndRateLimit(t *testing.T) {
	for name, testCase := range map[string]struct {
		err    error
		status int
	}{
		"invalid": {auth.ErrInvalidCredentials, http.StatusUnauthorized},
		"limited": {auth.ErrLoginRateLimited, http.StatusTooManyRequests},
	} {
		t.Run(name, func(t *testing.T) {
			fake := completeFakeAuth()
			fake.login = func(context.Context, string, string) (auth.Parent, auth.TokenPair, error) {
				return auth.Parent{}, auth.TokenPair{}, testCase.err
			}
			request := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(`{"email":"parent@example.com","password":"bad-password-value"}`))
			response := httptest.NewRecorder()
			NewHandler(Options{Auth: fake}).ServeHTTP(response, request)
			if response.Code != testCase.status {
				t.Fatalf("expected %d, got %d", testCase.status, response.Code)
			}
		})
	}
}

func TestAuthEndpointsRejectMalformedOrUnboundedInput(t *testing.T) {
	fake := completeFakeAuth()
	tests := []struct {
		name string
		body string
	}{
		{"unknown field", `{"email":"p@example.com","password":"long-enough-password","extra":true}`},
		{"multiple objects", `{"email":"p@example.com","password":"long-enough-password"}{}`},
		{"oversized", `{"email":"p@example.com","password":"` + strings.Repeat("x", maxAuthRequestBytes) + `"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/v1/auth/register", strings.NewReader(test.body))
			response := httptest.NewRecorder()
			NewHandler(Options{Auth: fake}).ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("expected %d, got %d", http.StatusBadRequest, response.Code)
			}
		})
	}
}

func TestLogoutRequiresBearerTokenAndDelegatesRevocation(t *testing.T) {
	const token = "at_access-value"
	fake := completeFakeAuth()
	fake.logout = func(_ context.Context, supplied string) error {
		if supplied != token {
			return errors.New("wrong token")
		}
		return nil
	}
	handler := NewHandler(Options{Auth: fake})

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/v1/auth/logout", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing bearer token rejection, got %d", unauthorized.Code)
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/auth/logout", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("expected logout status %d, got %d", http.StatusNoContent, response.Code)
	}
}
