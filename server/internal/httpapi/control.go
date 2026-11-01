package httpapi

import (
	"appcontrol/server/internal/control"
	"errors"
	"net/http"
)

func registerControlRoutes(mux *http.ServeMux, auth AuthService, service *control.Service) {
	mux.HandleFunc("PUT /v1/device/apps", func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			writeError(w, 401, "invalid_device_credential")
			return
		}
		var input struct {
			Apps []control.InstalledApp `json:"apps"`
		}
		if decodeJSONLimit(w, r, &input, 1024*1024) != nil || input.Apps == nil {
			writeError(w, 400, "invalid_request")
			return
		}
		if err := service.UpdateInventory(r.Context(), token, input.Apps); err != nil {
			controlError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /v1/device/policy/report", func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			writeError(w, 401, "invalid_device_credential")
			return
		}
		var input struct {
			Version  int64  `json:"version"`
			Result   string `json:"result"`
			Reason   string `json:"reason"`
			StateKey string `json:"stateKey"`
		}
		if decodeJSON(w, r, &input) != nil {
			writeError(w, 400, "invalid_request")
			return
		}
		if err := service.ReportPolicyState(r.Context(), token, input.Version, input.Result, input.Reason, input.StateKey); err != nil {
			controlError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /v1/device/policy", func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			writeError(w, 401, "invalid_device_credential")
			return
		}
		maxFormat := 1
		if r.Header.Get("X-Policy-Format") == "2" {
			maxFormat = 2
		}
		policy, err := service.DevicePolicyWithFormat(r.Context(), token, maxFormat)
		if err != nil {
			controlError(w, err)
			return
		}
		writeJSON(w, 200, policy)
	})
	mux.HandleFunc("GET /v1/device/commands", func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			writeError(w, 401, "invalid_device_credential")
			return
		}
		commands, err := service.Fetch(r.Context(), token)
		if err != nil {
			controlError(w, err)
			return
		}
		writeJSON(w, 200, struct {
			Commands []control.Command `json:"commands"`
		}{commands})
	})
	mux.HandleFunc("POST /v1/device/commands/{commandID}/ack", func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			writeError(w, 401, "invalid_device_credential")
			return
		}
		var input struct {
			Result string `json:"result"`
		}
		if decodeJSON(w, r, &input) != nil {
			writeError(w, 400, "invalid_request")
			return
		}
		if err := service.Acknowledge(r.Context(), token, r.PathValue("commandID"), input.Result); err != nil {
			controlError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("PUT /v1/device/remote-assistance/screens/{commandID}", func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			writeError(w, 401, "invalid_device_credential")
			return
		}
		var input struct {
			ImageBase64 string `json:"imageBase64"`
		}
		if decodeJSONLimit(w, r, &input, 524288) != nil {
			writeError(w, 400, "invalid_request")
			return
		}
		if err := service.SaveRemoteScreen(r.Context(), token, r.PathValue("commandID"), input.ImageBase64); err != nil {
			controlError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
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
	mux.HandleFunc("GET /v1/devices/{deviceID}/policy", parentOnly(func(w http.ResponseWriter, r *http.Request, parentID string) {
		policy, err := service.Policy(r.Context(), parentID, r.PathValue("deviceID"))
		if err != nil {
			controlError(w, err)
			return
		}
		writeJSON(w, 200, policy)
	}))
	mux.HandleFunc("GET /v1/devices/{deviceID}/apps", parentOnly(func(w http.ResponseWriter, r *http.Request, parentID string) {
		inventory, err := service.Inventory(r.Context(), parentID, r.PathValue("deviceID"))
		if err != nil {
			controlError(w, err)
			return
		}
		writeJSON(w, 200, inventory)
	}))
	mux.HandleFunc("GET /v1/devices/{deviceID}/commands/{commandID}", parentOnly(func(w http.ResponseWriter, r *http.Request, parentID string) {
		command, err := service.Command(r.Context(), parentID, r.PathValue("deviceID"), r.PathValue("commandID"))
		if err != nil {
			controlError(w, err)
			return
		}
		writeJSON(w, 200, command)
	}))
	mux.HandleFunc("GET /v1/devices/{deviceID}/remote-assistance/screens/{commandID}", parentOnly(func(w http.ResponseWriter, r *http.Request, parentID string) {
		screen, err := service.RemoteScreen(r.Context(), parentID, r.PathValue("deviceID"), r.PathValue("commandID"))
		if err != nil {
			controlError(w, err)
			return
		}
		writeJSON(w, 200, screen)
	}))
	mux.HandleFunc("POST /v1/devices/{deviceID}/commands", parentOnly(func(w http.ResponseWriter, r *http.Request, parentID string) {
		var input struct {
			Type           string `json:"type"`
			TTLSeconds     int    `json:"ttlSeconds"`
			CallID         string `json:"callId,omitempty"`
			PackageName    string `json:"packageName,omitempty"`
			X              *int   `json:"x,omitempty"`
			Y              *int   `json:"y,omitempty"`
			EndX           *int   `json:"endX,omitempty"`
			EndY           *int   `json:"endY,omitempty"`
			DurationMillis *int   `json:"durationMillis,omitempty"`
			Action         string `json:"action,omitempty"`
		}
		if decodeJSONLimit(w, r, &input, 4096) != nil {
			writeError(w, 400, "invalid_request")
			return
		}
		var command control.Command
		var err error
		if input.Type == "LOCK_DEVICE" && input.CallID == "" {
			command, err = service.QueueLockWithKey(r.Context(), parentID, r.PathValue("deviceID"), input.TTLSeconds, r.Header.Get("Idempotency-Key"))
		} else if input.Type == "ANSWER_CALL" || input.Type == "REJECT_CALL" || input.Type == "END_CALL" {
			command, err = service.QueueCallActionWithKey(r.Context(), parentID, r.PathValue("deviceID"), input.Type, input.CallID, input.TTLSeconds, r.Header.Get("Idempotency-Key"))
		} else if input.Type == "OPEN_APP" && input.CallID == "" {
			command, err = service.QueueOpenApp(r.Context(), parentID, r.PathValue("deviceID"), input.PackageName, input.TTLSeconds, r.Header.Get("Idempotency-Key"))
		} else if input.Type == "OPEN_SETTINGS" && input.CallID == "" && input.PackageName == "" {
			command, err = service.QueueOpenSettings(r.Context(), parentID, r.PathValue("deviceID"), input.TTLSeconds, r.Header.Get("Idempotency-Key"))
		} else if input.Type == "REMOTE_SCREENSHOT" && input.CallID == "" && input.PackageName == "" && input.X == nil && input.Y == nil {
			command, err = service.QueueRemoteScreenshot(r.Context(), parentID, r.PathValue("deviceID"), input.TTLSeconds, r.Header.Get("Idempotency-Key"))
		} else if input.Type == "REMOTE_TAP" && input.CallID == "" && input.PackageName == "" && input.X != nil && input.Y != nil {
			command, err = service.QueueRemoteTap(r.Context(), parentID, r.PathValue("deviceID"), *input.X, *input.Y, input.TTLSeconds, r.Header.Get("Idempotency-Key"))
		} else if input.Type == "REMOTE_SWIPE" && input.CallID == "" && input.PackageName == "" && input.X != nil && input.Y != nil && input.EndX != nil && input.EndY != nil && input.DurationMillis != nil {
			command, err = service.QueueRemoteSwipe(r.Context(), parentID, r.PathValue("deviceID"), *input.X, *input.Y, *input.EndX, *input.EndY, *input.DurationMillis, input.TTLSeconds, r.Header.Get("Idempotency-Key"))
		} else if input.Type == "REMOTE_LONG_PRESS" && input.CallID == "" && input.PackageName == "" && input.X != nil && input.Y != nil && input.DurationMillis != nil {
			command, err = service.QueueRemoteLongPress(r.Context(), parentID, r.PathValue("deviceID"), *input.X, *input.Y, *input.DurationMillis, input.TTLSeconds, r.Header.Get("Idempotency-Key"))
		} else if input.Type == "REMOTE_GLOBAL" && input.CallID == "" && input.PackageName == "" && input.X == nil && input.Y == nil && input.EndX == nil && input.EndY == nil && input.DurationMillis == nil {
			command, err = service.QueueRemoteGlobal(r.Context(), parentID, r.PathValue("deviceID"), input.Action, input.TTLSeconds, r.Header.Get("Idempotency-Key"))
		} else {
			writeError(w, 400, "invalid_request")
			return
		}
		if err != nil {
			controlError(w, err)
			return
		}
		writeJSON(w, 201, command)
	}))
	mux.HandleFunc("PUT /v1/devices/{deviceID}/policy", parentOnly(func(w http.ResponseWriter, r *http.Request, parentID string) {
		var input struct {
			ExpectedVersion       *int64             `json:"expectedVersion"`
			InternetPaused        *bool              `json:"internetPaused"`
			BlockedPackages       *[]string          `json:"blockedPackages"`
			AlwaysAllowedPackages []string           `json:"alwaysAllowedPackages"`
			Schedules             []control.Schedule `json:"schedules"`
		}
		if decodeJSONLimit(w, r, &input, 64*1024) != nil || input.ExpectedVersion == nil || input.InternetPaused == nil || input.BlockedPackages == nil {
			writeError(w, 400, "invalid_request")
			return
		}
		policy, err := service.UpdatePolicyRules(r.Context(), parentID, r.PathValue("deviceID"), *input.ExpectedVersion, *input.InternetPaused, *input.BlockedPackages, input.AlwaysAllowedPackages, input.Schedules)
		if err != nil {
			controlError(w, err)
			return
		}
		writeJSON(w, 200, policy)
	}))
}

func controlError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, control.ErrPolicyFormat):
		writeError(w, 409, "unsupported_policy_format")
	case errors.Is(err, control.ErrCallUnavailable):
		writeError(w, 409, "call_not_current")
	case errors.Is(err, control.ErrCallState):
		writeError(w, 409, "call_state_conflict")
	case errors.Is(err, control.ErrRemoteCallDisabled):
		writeError(w, 409, "remote_calls_disabled")
	case errors.Is(err, control.ErrIdempotency):
		writeError(w, 409, "idempotency_conflict")
	case errors.Is(err, control.ErrUnauthorized):
		writeError(w, 401, "invalid_device_credential")
	case errors.Is(err, control.ErrCommandState):
		writeError(w, 409, "command_state_conflict")
	case errors.Is(err, control.ErrInvalid):
		writeError(w, 400, "invalid_request")
	case errors.Is(err, control.ErrNotFound):
		writeError(w, 404, "device_not_found")
	case errors.Is(err, control.ErrConflict):
		writeError(w, 409, "policy_version_conflict")
	default:
		writeError(w, 500, "server_error")
	}
}
