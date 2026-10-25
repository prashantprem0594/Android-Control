package httpapi

import (
	"appcontrol/server/internal/control"
	"net/http"
)

func registerDeviceStatusRoutes(mux *http.ServeMux, authentication AuthService, service *control.Service) {
	mux.HandleFunc("POST /v1/device/calls/sequence", func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			writeError(w, 401, "invalid_device_credential")
			return
		}
		var input struct{}
		if decodeJSONLimit(w, r, &input, 1024) != nil {
			writeError(w, 400, "invalid_request")
			return
		}
		sequence, err := service.AllocateRemoteCallSequence(r.Context(), token)
		if err != nil {
			controlError(w, err)
			return
		}
		writeJSON(w, 200, struct {
			Sequence int64 `json:"sequence"`
		}{sequence})
	})
	mux.HandleFunc("POST /v1/device/calls", func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			writeError(w, 401, "invalid_device_credential")
			return
		}
		var input struct {
			Sequence int64 `json:"sequence"`
			Calls    []struct {
				CallID              string `json:"callId"`
				State               string `json:"state"`
				RemoteActionsAllowed *bool  `json:"remoteActionsAllowed"`
			} `json:"calls"`
		}
		if decodeJSONLimit(w, r, &input, 4096) != nil || input.Calls == nil {
			writeError(w, 400, "invalid_request")
			return
		}
		calls := make([]control.RemoteCall, 0, len(input.Calls))
		for _, call := range input.Calls {
			if call.RemoteActionsAllowed == nil {
				writeError(w, 400, "invalid_request")
				return
			}
			calls = append(calls, control.RemoteCall{
				CallID: call.CallID, State: call.State, RemoteActionsAllowed: *call.RemoteActionsAllowed,
			})
		}
		accepted, err := service.ReportRemoteCalls(r.Context(), token, input.Sequence, calls)
		if err != nil {
			controlError(w, err)
			return
		}
		writeJSON(w, 200, struct {
			Status   string `json:"status"`
			Accepted bool   `json:"accepted"`
		}{"received", accepted})
	})
	mux.HandleFunc("GET /v1/devices/{deviceID}/calls", func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			writeError(w, 401, "invalid_token")
			return
		}
		parent, _, err := authentication.Authenticate(r.Context(), token)
		if err != nil {
			writeError(w, 401, "invalid_token")
			return
		}
		calls, err := service.RemoteCalls(r.Context(), parent.ID, r.PathValue("deviceID"))
		if err != nil {
			controlError(w, err)
			return
		}
		writeJSON(w, 200, calls)
	})
	mux.HandleFunc("POST /v1/device/status", func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			writeError(w, 401, "invalid_device_credential")
			return
		}
		// Pointers distinguish an explicitly false capability from a missing field.
		var input struct {
			AndroidAPI  *int  `json:"androidApi"`
			DeviceOwner *bool `json:"deviceOwner"`
			DialerRole  *bool `json:"dialerRole"`
			VPNConsent  *bool `json:"vpnConsent"`
			ExactAlarms *bool `json:"exactAlarms"`
		}
		if decodeJSON(w, r, &input) != nil || input.AndroidAPI == nil || input.DeviceOwner == nil || input.DialerRole == nil || input.VPNConsent == nil || input.ExactAlarms == nil {
			writeError(w, 400, "invalid_request")
			return
		}
		if err := service.ReportStatus(r.Context(), token, control.Capabilities{AndroidAPI: *input.AndroidAPI, DeviceOwner: *input.DeviceOwner, DialerRole: *input.DialerRole, VPNConsent: *input.VPNConsent, ExactAlarms: *input.ExactAlarms}); err != nil {
			controlError(w, err)
			return
		}
		writeJSON(w, 200, struct {
			Status string `json:"status"`
		}{"received"})
	})
	mux.HandleFunc("GET /v1/devices/{deviceID}/status", func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			writeError(w, 401, "invalid_token")
			return
		}
		parent, _, err := authentication.Authenticate(r.Context(), token)
		if err != nil {
			writeError(w, 401, "invalid_token")
			return
		}
		status, err := service.Status(r.Context(), parent.ID, r.PathValue("deviceID"))
		if err != nil {
			controlError(w, err)
			return
		}
		writeJSON(w, 200, status)
	})
}
