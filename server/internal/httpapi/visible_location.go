package httpapi

import (
	"appcontrol/server/internal/control"
	"net/http"
	"time"
)

func registerVisibleLocationRoutes(mux *http.ServeMux, authentication AuthService, service *control.Service) {
	// Device-only upload. This endpoint records an already visible, child-started
	// session; it does not create or extend one.
	mux.HandleFunc("POST /v1/device/visible-location", func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			writeError(w, 401, "invalid_device_credential")
			return
		}
		var input struct {
			LatitudeE7     int64     `json:"latitudeE7"`
			LongitudeE7    int64     `json:"longitudeE7"`
			AccuracyMeters float64   `json:"accuracyMeters"`
			CapturedAt     time.Time `json:"capturedAt"`
		}
		if decodeJSONLimit(w, r, &input, 1024) != nil {
			writeError(w, 400, "invalid_request")
			return
		}
		if err := service.RecordVisibleLocation(r.Context(), token, control.VisibleLocation{
			LatitudeE7: input.LatitudeE7, LongitudeE7: input.LongitudeE7, AccuracyMeters: input.AccuracyMeters, CapturedAt: input.CapturedAt,
		}); err != nil {
			controlError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Status string `json:"status"`
		}{"received"})
	})
	// Parent-only read. Deliberately no POST/command counterpart exists.
	mux.HandleFunc("GET /v1/devices/{deviceID}/visible-location", func(w http.ResponseWriter, r *http.Request) {
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
		location, err := service.VisibleLocation(r.Context(), parent.ID, r.PathValue("deviceID"))
		if err != nil {
			controlError(w, err)
			return
		}
		if location == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeJSON(w, http.StatusOK, location)
	})
}
