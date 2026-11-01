package httpapi

import (
	"appcontrol/server/internal/parentpush"
	"errors"
	"net/http"
)

func registerParentWakeupRegistrationRoutes(mux *http.ServeMux, authentication AuthService, store *parentpush.Store) {
	registerParentCallEventRoutes(mux, authentication, store)
	mux.HandleFunc("PUT /v1/parent/wakeup-registration", func(w http.ResponseWriter, r *http.Request) {
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
		var input struct {
			Registration string `json:"registrationToken"`
		}
		if decodeJSONLimit(w, r, &input, 8192) != nil {
			writeError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		if err := store.Register(r.Context(), parent.ID, familyID, input.Registration); err != nil {
			writeParentPushError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /v1/parent/wakeup-registration", func(w http.ResponseWriter, r *http.Request) {
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
		if err := store.Remove(r.Context(), parent.ID, familyID); err != nil {
			writeParentPushError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func writeParentPushError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, parentpush.ErrInvalid):
		writeError(w, http.StatusBadRequest, "invalid_request")
	case errors.Is(err, parentpush.ErrUnauthorized):
		writeError(w, http.StatusUnauthorized, "invalid_token")
	default:
		writeError(w, http.StatusInternalServerError, "server_error")
	}
}
