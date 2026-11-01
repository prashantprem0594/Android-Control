package httpapi

import (
	"appcontrol/server/internal/parentpush"
	"net/http"
)

func registerParentCallEventRoutes(mux *http.ServeMux, authentication AuthService, store *parentpush.Store) {
	mux.HandleFunc("GET /v1/parent/call-events", func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "invalid_token")
			return
		}
		parent, familyID, err := authentication.Authenticate(r.Context(), token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid_token")
			return
		}
		if r.URL.RawQuery != "" {
			writeError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		feed, err := store.CallEvents(r.Context(), parent.ID, familyID)
		if err != nil {
			writeParentPushError(w, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, feed)
	})
}
