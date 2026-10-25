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
)

func TestHealthEndpoint(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	NewHandler(Options{}).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("expected no-store cache policy")
	}
	if response.Body.String() != "{\"status\":\"ok\"}\n" {
		t.Fatalf("unexpected response body: %q", response.Body.String())
	}
}

func TestUnknownEndpoint(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/unknown", nil)
	response := httptest.NewRecorder()

	NewHandler(Options{}).ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, response.Code)
	}
}

func TestReadinessReportsUnavailableWithoutLeakingReason(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response := httptest.NewRecorder()

	NewHandler(Options{Ready: func(context.Context) error {
		return errors.New("database password must not be exposed")
	}}).ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, response.Code)
	}
	if response.Body.String() != "{\"status\":\"unavailable\"}\n" {
		t.Fatalf("unexpected response body: %q", response.Body.String())
	}
}

func TestRequestLogOmitsQueryValues(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	request := httptest.NewRequest(http.MethodGet, "/healthz?token=secret-value", nil)
	response := httptest.NewRecorder()

	NewHandler(Options{Logger: logger}).ServeHTTP(response, request)

	logged := output.String()
	if !strings.Contains(logged, `"path":"/healthz"`) {
		t.Fatalf("expected sanitized path in log: %s", logged)
	}
	if strings.Contains(logged, "secret-value") || strings.Contains(logged, "token") {
		t.Fatalf("query data leaked into request log: %s", logged)
	}
}
