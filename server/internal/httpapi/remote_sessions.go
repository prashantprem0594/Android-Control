package httpapi

import (
	"appcontrol/server/internal/control"
	"encoding/json"
	"net/http"
)

func registerRemoteSessionRoutes(mux *http.ServeMux, auth AuthService, service *control.Service) {
	parentOnly := func(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
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
			next(w, r, parent.ID)
		}
	}
	mux.HandleFunc("POST /v1/devices/{deviceID}/remote-assistance/sessions", parentOnly(func(w http.ResponseWriter, r *http.Request, parentID string) {
		var input struct {
			TTLSeconds int `json:"ttlSeconds"`
		}
		if decodeJSONLimit(w, r, &input, 4096) != nil {
			writeError(w, 400, "invalid_request")
			return
		}
		result, err := service.StartRemoteSession(r.Context(), parentID, r.PathValue("deviceID"), input.TTLSeconds)
		if err != nil {
			controlError(w, err)
			return
		}
		writeJSON(w, 201, result)
	}))
	mux.HandleFunc("GET /v1/devices/{deviceID}/remote-assistance/sessions/{sessionID}", parentOnly(func(w http.ResponseWriter, r *http.Request, parentID string) {
		result, err := service.RemoteSession(r.Context(), parentID, r.PathValue("deviceID"), r.PathValue("sessionID"))
		if err != nil {
			controlError(w, err)
			return
		}
		writeJSON(w, 200, result)
	}))
	mux.HandleFunc("PUT /v1/devices/{deviceID}/remote-assistance/sessions/{sessionID}/offer", parentOnly(func(w http.ResponseWriter, r *http.Request, parentID string) {
		var input struct {
			SDP string `json:"sdp"`
		}
		if decodeJSONLimit(w, r, &input, 256*1024) != nil {
			writeError(w, 400, "invalid_request")
			return
		}
		if err := service.UpdateRemoteOffer(r.Context(), parentID, r.PathValue("deviceID"), r.PathValue("sessionID"), input.SDP); err != nil {
			controlError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("POST /v1/devices/{deviceID}/remote-assistance/sessions/{sessionID}/close", parentOnly(func(w http.ResponseWriter, r *http.Request, parentID string) {
		if err := service.CloseRemoteSession(r.Context(), parentID, r.PathValue("deviceID"), r.PathValue("sessionID")); err != nil {
			controlError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("GET /v1/device/remote-assistance/session", func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			writeError(w, 401, "invalid_device_credential")
			return
		}
		result, err := service.ActiveRemoteSession(r.Context(), token)
		if err != nil {
			controlError(w, err)
			return
		}
		writeJSON(w, 200, result)
	})
	mux.HandleFunc("PUT /v1/device/remote-assistance/sessions/{sessionID}/answer", func(w http.ResponseWriter, r *http.Request) { signalDevice(w, r, service, "answer_sdp") })
	mux.HandleFunc("PUT /v1/device/remote-assistance/sessions/{sessionID}/ice", func(w http.ResponseWriter, r *http.Request) { signalDevice(w, r, service, "ice_json") })
}

func signalDevice(w http.ResponseWriter, r *http.Request, service *control.Service, field string) {
	token, ok := bearerToken(r)
	if !ok {
		writeError(w, 401, "invalid_device_credential")
		return
	}
	var input struct {
		SDP        string   `json:"sdp"`
		Candidates []string `json:"candidates"`
	}
	if decodeJSONLimit(w, r, &input, 256*1024) != nil {
		writeError(w, 400, "invalid_request")
		return
	}
	value := input.SDP
	if field == "ice_json" {
		if input.Candidates == nil {
			writeError(w, 400, "invalid_request")
			return
		}
		valueBytes, _ := json.Marshal(input.Candidates)
		value = string(valueBytes)
	}
	if err := service.UpdateRemoteSignal(r.Context(), token, r.PathValue("sessionID"), field, value); err != nil {
		controlError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
