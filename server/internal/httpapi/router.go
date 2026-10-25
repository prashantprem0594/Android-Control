package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"appcontrol/server/internal/audit"
	"appcontrol/server/internal/auth"
	"appcontrol/server/internal/control"
	"appcontrol/server/internal/pairing"
	"appcontrol/server/internal/release"
)

type statusResponse struct {
	Status string `json:"status"`
}

type Options struct {
	Release      *release.Document
	WakeupTokens *control.WakeupTokens
	Guard        *RequestGuard
	Control      *control.Service
	Audit        func(context.Context, string, int64, int) ([]audit.Event, error)
	Logger       *slog.Logger
	Ready        func(context.Context) error
	Auth         AuthService
	Pairing      *pairing.Service
}

type AuthService interface {
	Register(context.Context, string, string) (auth.Parent, error)
	Login(context.Context, string, string) (auth.Parent, auth.TokenPair, error)
	Refresh(context.Context, string) (auth.TokenPair, error)
	Authenticate(context.Context, string) (auth.Parent, string, error)
	Logout(context.Context, string) error
}

func NewHandler(options Options) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", statusHandler("ok"))
	mux.HandleFunc("GET /readyz", readinessHandler(options.Ready))
	if options.Release != nil {
		registerReleaseRoute(mux, options.Release)
	}
	if options.WakeupTokens != nil {
		registerWakeupTokenRoutes(mux, options.WakeupTokens)
	}
	if options.Auth != nil {
		if options.Control != nil {
			registerCommandHistoryRoute(mux, options.Auth, options.Control)
			registerDeviceStatusRoutes(mux, options.Auth, options.Control)
			registerCallContactRoutes(mux, options.Auth, options.Control)
			registerControlRoutes(mux, options.Auth, options.Control)
		}
		registerAuthRoutes(mux, options.Auth)
		if options.Audit != nil {
			registerAuditRoutes(mux, options.Auth, options.Audit)
		}
		if options.Pairing != nil {
			registerDeviceListRoute(mux, options.Auth, options.Pairing)
			registerPairingRoutes(mux, options.Auth, options.Pairing)
		}
	}
	guard := options.Guard
	if guard == nil {
		guard, _ = NewRequestGuard(GuardConfig{})
	}
	return requestLogging(loggerOrDiscard(options.Logger), securityHeaders(guard.wrap(mux)), mux)
}

func statusHandler(status string) http.HandlerFunc {
	return func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(response).Encode(statusResponse{Status: status})
	}
}

func readinessHandler(ready func(context.Context) error) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		status := "ready"
		statusCode := http.StatusOK
		if ready != nil && ready(request.Context()) != nil {
			status = "unavailable"
			statusCode = http.StatusServiceUnavailable
		}

		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(statusCode)
		_ = json.NewEncoder(response).Encode(statusResponse{Status: status})
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Cache-Control", "no-store")
		response.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(response, request)
	})
}

type responseMetrics struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (response *responseMetrics) WriteHeader(status int) {
	if response.status != 0 {
		return
	}
	response.status = status
	response.ResponseWriter.WriteHeader(status)
}

func (response *responseMetrics) Write(body []byte) (int, error) {
	if response.status == 0 {
		response.WriteHeader(http.StatusOK)
	}
	written, err := response.ResponseWriter.Write(body)
	response.bytes += written
	return written, err
}

func requestLogging(logger *slog.Logger, next http.Handler, mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		started := time.Now()
		metrics := &responseMetrics{ResponseWriter: response}
		path := "unmatched"
		completed := false
		defer func() {
			failure := recover()
			panicked := !completed
			aborted := false
			if panicked {
				if metrics.status == 0 && failure != http.ErrAbortHandler {
					writeError(metrics, 500, "server_error")
				} else {
					aborted = true
				}
			}
			status := metrics.status
			if status == 0 {
				status = http.StatusOK
			}
			method := "OTHER"
			switch request.Method {
			case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "CONNECT", "TRACE":
				method = request.Method
			}
			logger.InfoContext(request.Context(), "HTTP request completed",
				"method", method, "path", path, "status", status, "response_bytes", metrics.bytes,
				"duration_ms", time.Since(started).Milliseconds(), "aborted", aborted, "handler_panic", panicked)
			// Do not print the panic value or stack: either can contain request secrets.
			if aborted {
				panic(http.ErrAbortHandler)
			}
		}()
		if len(request.RequestURI) <= 8192 && request.Method != "CONNECT" {
			_, pattern := mux.Handler(request)
			if _, template, ok := strings.Cut(pattern, " "); ok {
				path = template
			}
		}
		next.ServeHTTP(metrics, request)
		completed = true
	})
}

func loggerOrDiscard(logger *slog.Logger) *slog.Logger {
	if logger != nil {
		return logger
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
