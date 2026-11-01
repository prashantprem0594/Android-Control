package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"appcontrol/server/internal/auth"
)

const maxAuthRequestBytes = 16 * 1024

type credentialsRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

type sessionResponse struct {
	Parent auth.Parent    `json:"parent"`
	Tokens auth.TokenPair `json:"tokens"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func registerAuthRoutes(mux *http.ServeMux, service AuthService) {
	mux.HandleFunc("POST /v1/auth/register", registerHandler(service))
	mux.HandleFunc("POST /v1/auth/login", loginHandler(service))
	mux.HandleFunc("POST /v1/auth/refresh", refreshHandler(service))
	mux.HandleFunc("POST /v1/auth/logout", logoutHandler(service))
	mux.HandleFunc("GET /v1/auth/me", meHandler(service))
}

func registerHandler(service AuthService) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		var credentials credentialsRequest
		if err := decodeJSON(response, request, &credentials); err != nil {
			writeError(response, http.StatusBadRequest, "invalid_request")
			return
		}
		parent, err := service.Register(request.Context(), credentials.Email, credentials.Password)
		switch {
		case errors.Is(err, auth.ErrEmailExists):
			writeError(response, http.StatusConflict, "email_exists")
		case err != nil:
			writeError(response, http.StatusBadRequest, "invalid_request")
		default:
			writeJSON(response, http.StatusCreated, parent)
		}
	}
}

func loginHandler(service AuthService) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		var credentials credentialsRequest
		if err := decodeJSON(response, request, &credentials); err != nil {
			writeError(response, http.StatusBadRequest, "invalid_request")
			return
		}
		parent, tokens, err := service.Login(request.Context(), credentials.Email, credentials.Password)
		switch {
		case errors.Is(err, auth.ErrInvalidCredentials):
			writeError(response, http.StatusUnauthorized, "invalid_credentials")
		case errors.Is(err, auth.ErrLoginRateLimited):
<<<<<<< HEAD
			response.Header().Set("Retry-After", "5")
=======
			response.Header().Set("Retry-After", "60")
>>>>>>> origin/main
			writeError(response, http.StatusTooManyRequests, "rate_limited")
		case err != nil:
			writeError(response, http.StatusInternalServerError, "server_error")
		default:
			writeJSON(response, http.StatusOK, sessionResponse{Parent: parent, Tokens: tokens})
		}
	}
}

func refreshHandler(service AuthService) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		var input refreshRequest
		if err := decodeJSON(response, request, &input); err != nil {
			writeError(response, http.StatusBadRequest, "invalid_request")
			return
		}
		tokens, err := service.Refresh(request.Context(), input.RefreshToken)
		if err != nil {
			writeError(response, http.StatusUnauthorized, "invalid_token")
			return
		}
		writeJSON(response, http.StatusOK, tokens)
	}
}

func logoutHandler(service AuthService) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		token, ok := bearerToken(request)
		if !ok {
			writeError(response, http.StatusUnauthorized, "invalid_token")
			return
		}
		if err := service.Logout(request.Context(), token); err != nil {
			writeError(response, http.StatusUnauthorized, "invalid_token")
			return
		}
		response.WriteHeader(http.StatusNoContent)
	}
}

func meHandler(service AuthService) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		token, ok := bearerToken(request)
		if !ok {
			writeError(response, http.StatusUnauthorized, "invalid_token")
			return
		}
		parent, _, err := service.Authenticate(request.Context(), token)
		if err != nil {
			writeError(response, http.StatusUnauthorized, "invalid_token")
			return
		}
		writeJSON(response, http.StatusOK, parent)
	}
}

func decodeJSON(response http.ResponseWriter, request *http.Request, target any) error {
	return decodeJSONLimit(response, request, target, maxAuthRequestBytes)
}

func decodeJSONLimit(response http.ResponseWriter, request *http.Request, target any, limit int64) error {
	request.Body = http.MaxBytesReader(response, request.Body, limit)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request must contain one JSON object")
	}
	return nil
}

func bearerToken(request *http.Request) (string, bool) {
	header := request.Header.Get("Authorization")
	if len(header) < 8 || !strings.EqualFold(header[:7], "Bearer ") {
		return "", false
	}
	token := strings.TrimSpace(header[7:])
	return token, token != "" && !strings.ContainsAny(token, " \t\r\n")
}

func writeError(response http.ResponseWriter, status int, code string) {
	writeJSON(response, status, errorResponse{Error: code})
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
