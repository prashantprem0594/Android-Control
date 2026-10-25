package httpapi

import (
	"context"
	"net/http"
	"strconv"

	"appcontrol/server/internal/audit"
)

func registerAuditRoutes(mux *http.ServeMux, auth AuthService, list func(context.Context, string, int64, int) ([]audit.Event, error)) {
	mux.HandleFunc("GET /v1/audit-events", func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			writeError(w, 401, "invalid_token")
			return
		}
		parent, _, err := auth.Authenticate(r.Context(), token)
		if err != nil {
			writeError(w, 401, "invalid_token")
			return
		}
		after := int64(0)
		limit := 50
		if value := r.URL.Query().Get("after"); value != "" {
			after, err = strconv.ParseInt(value, 10, 64)
			if err != nil || after < 0 {
				writeError(w, 400, "invalid_page")
				return
			}
		}
		if value := r.URL.Query().Get("limit"); value != "" {
			limit, err = strconv.Atoi(value)
			if err != nil || limit < 1 || limit > 100 {
				writeError(w, 400, "invalid_page")
				return
			}
		}
		events, err := list(r.Context(), parent.ID, after, limit)
		if err != nil {
			writeError(w, 500, "server_error")
			return
		}
		writeJSON(w, 200, struct {
			Events []audit.Event `json:"events"`
		}{events})
	})
}
