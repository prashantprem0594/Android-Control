package httpapi

import (
	"appcontrol/server/internal/pairing"
	"errors"
	"net/http"
)

func registerPairingRoutes(mux *http.ServeMux, authService AuthService, service *pairing.Service) {
	parentOnly := func(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				writeError(w, 401, "invalid_token")
				return
			}
			parent, _, err := authService.Authenticate(r.Context(), token)
			if err != nil {
				writeError(w, 401, "invalid_token")
				return
			}
			next(w, r, parent.ID)
		}
	}
	deviceOnly := func(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				writeError(w, 401, "invalid_device_credential")
				return
			}
			deviceID, err := service.AuthenticateDevice(r.Context(), token)
			if err != nil {
				writeError(w, 401, "invalid_device_credential")
				return
			}
			next(w, r, deviceID)
		}
	}
	mux.HandleFunc("PUT /v1/device/recovery-binding", deviceOnly(func(w http.ResponseWriter, r *http.Request, deviceID string) {
		var input struct {
			RecoveryHandle string `json:"recoveryHandle"`
		}
		if decodeJSON(w, r, &input) != nil {
			writeError(w, 400, "invalid_request")
			return
		}
		if err := service.BindRecovery(r.Context(), deviceID, input.RecoveryHandle); err != nil {
			writeError(w, 400, "invalid_request")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("POST /v1/device-recovery/requests", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			RecoveryHandle string `json:"recoveryHandle"`
			PublicKey      string `json:"publicKey"`
			KeyAlgorithm   string `json:"keyAlgorithm"`
		}
		if decodeJSON(w, r, &input) != nil {
			writeError(w, 400, "invalid_request")
			return
		}
		result, err := service.StartRecovery(r.Context(), input.RecoveryHandle, input.PublicKey, input.KeyAlgorithm)
		if err != nil {
			writeError(w, 400, "invalid_request")
			return
		}
		writeJSON(w, 201, result)
	})
	mux.HandleFunc("POST /v1/device-recovery/requests/{requestID}/proof", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			RequestToken string `json:"requestToken"`
			Challenge    string `json:"challenge"`
			Signature    string `json:"signature"`
		}
		if decodeJSON(w, r, &input) != nil {
			writeError(w, 400, "invalid_request")
			return
		}
		if err := service.ProveRecovery(r.Context(), r.PathValue("requestID"), input.RequestToken, input.Challenge, input.Signature); err != nil {
			writeError(w, 400, "invalid_request")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /v1/device-recovery/requests/{requestID}/status", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			RequestToken string `json:"requestToken"`
		}
		if decodeJSON(w, r, &input) != nil {
			writeError(w, 400, "invalid_request")
			return
		}
		result, err := service.RecoveryStatus(r.Context(), r.PathValue("requestID"), input.RequestToken)
		if err != nil {
			writeError(w, 400, "invalid_request")
			return
		}
		writeJSON(w, 200, result)
	})
	mux.HandleFunc("GET /v1/device-recovery/requests", parentOnly(func(w http.ResponseWriter, r *http.Request, parentID string) {
		result, err := service.PendingRecoveries(r.Context(), parentID)
		if err != nil {
			writeError(w, 500, "server_error")
			return
		}
		writeJSON(w, 200, struct {
			Requests []pairing.PendingRecovery `json:"requests"`
		}{Requests: result})
	}))
	for _, action := range []struct {
		name    string
		approve bool
	}{{"approve", true}, {"reject", false}} {
		action := action
		mux.HandleFunc("POST /v1/device-recovery/requests/{requestID}/"+action.name, parentOnly(func(w http.ResponseWriter, r *http.Request, parentID string) {
			err := service.ResolveRecovery(r.Context(), parentID, r.PathValue("requestID"), action.approve)
			if errors.Is(err, pairing.ErrRecoveryNotFound) {
				writeError(w, 404, "recovery_request_not_found")
				return
			}
			if err != nil {
				writeError(w, 500, "server_error")
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}))
	}
	mux.HandleFunc("POST /v1/pairing/codes", parentOnly(func(w http.ResponseWriter, r *http.Request, parentID string) {
		code, err := service.CreateCode(r.Context(), parentID)
		if err != nil {
			writeError(w, 500, "server_error")
			return
		}
		writeJSON(w, 201, code)
	}))
	mux.HandleFunc("GET /v1/devices/{deviceID}", parentOnly(func(w http.ResponseWriter, r *http.Request, parentID string) {
		device, err := service.Device(r.Context(), parentID, r.PathValue("deviceID"))
		if errors.Is(err, pairing.ErrDeviceNotFound) {
			writeError(w, 404, "device_not_found")
			return
		}
		if err != nil {
			writeError(w, 500, "server_error")
			return
		}
		writeJSON(w, 200, device)
	}))
	mux.HandleFunc("POST /v1/devices/{deviceID}/revoke", parentOnly(func(w http.ResponseWriter, r *http.Request, parentID string) {
		err := service.RevokeDevice(r.Context(), parentID, r.PathValue("deviceID"))
		if errors.Is(err, pairing.ErrDeviceNotFound) {
			writeError(w, 404, "device_not_found")
			return
		}
		if err != nil {
			writeError(w, 500, "server_error")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("POST /v1/device-auth/challenges", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			DeviceID string `json:"deviceId"`
		}
		if decodeJSON(w, r, &input) != nil {
			writeError(w, 400, "invalid_request")
			return
		}
		challenge, err := service.CreateAuthChallenge(r.Context(), input.DeviceID)
		if err != nil {
			writeError(w, 401, "invalid_device_credential")
			return
		}
		writeJSON(w, 201, challenge)
	})
	mux.HandleFunc("POST /v1/device-auth/sessions", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			DeviceID    string `json:"deviceId"`
			ChallengeID string `json:"challengeId"`
			Challenge   string `json:"challenge"`
			Signature   string `json:"signature"`
		}
		if decodeJSON(w, r, &input) != nil {
			writeError(w, 400, "invalid_request")
			return
		}
		session, err := service.CompleteAuthentication(r.Context(), input.DeviceID, input.ChallengeID, input.Challenge, input.Signature)
		if err != nil {
			writeError(w, 401, "invalid_device_credential")
			return
		}
		writeJSON(w, 201, session)
	})
	mux.HandleFunc("GET /v1/device-auth/me", func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			writeError(w, 401, "invalid_device_credential")
			return
		}
		id, err := service.AuthenticateDevice(r.Context(), token)
		if err != nil {
			writeError(w, 401, "invalid_device_credential")
			return
		}
		writeJSON(w, 200, struct {
			DeviceID string `json:"deviceId"`
		}{id})
	})
	mux.HandleFunc("POST /v1/enrollments", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Code         string `json:"code"`
			DisplayName  string `json:"displayName"`
			PublicKey    string `json:"publicKey"`
			KeyAlgorithm string `json:"keyAlgorithm"`
		}
		if decodeJSON(w, r, &input) != nil {
			writeError(w, 400, "invalid_request")
			return
		}
		enrollment, err := service.StartEnrollment(r.Context(), input.Code, input.DisplayName, input.PublicKey, input.KeyAlgorithm)
		if err != nil {
			writeError(w, 400, "invalid_enrollment")
			return
		}
		writeJSON(w, 201, enrollment)
	})
	mux.HandleFunc("POST /v1/enrollments/complete", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			EnrollmentID string `json:"enrollmentId"`
			Challenge    string `json:"challenge"`
			Signature    string `json:"signature"`
		}
		if decodeJSON(w, r, &input) != nil {
			writeError(w, 400, "invalid_request")
			return
		}
		device, err := service.CompleteEnrollment(r.Context(), input.EnrollmentID, input.Challenge, input.Signature)
		if err != nil {
			writeError(w, 400, "invalid_enrollment")
			return
		}
		writeJSON(w, 200, device)
	})
}
