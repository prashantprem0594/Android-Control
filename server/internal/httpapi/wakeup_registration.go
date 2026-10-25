package httpapi

import (
	"appcontrol/server/internal/control"
	"net/http"
)

func registerWakeupTokenRoutes(mux *http.ServeMux, store *control.WakeupTokens) {
	mux.HandleFunc("PUT /v1/device/wakeup-registration", func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			writeError(w, 401, "invalid_device_credential")
			return
		}
		var input struct {
			Registration string `json:"registrationToken"`
		}
		if decodeJSON(w, r, &input) != nil {
			writeError(w, 400, "invalid_request")
			return
		}
		if err := store.Register(r.Context(), token, input.Registration); err != nil {
			controlError(w, err)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("DELETE /v1/device/wakeup-registration", func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			writeError(w, 401, "invalid_device_credential")
			return
		}
		if err := store.Remove(r.Context(), token); err != nil {
			controlError(w, err)
			return
		}
		w.WriteHeader(204)
	})
}
