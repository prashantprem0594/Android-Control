package httpapi

import (
	"appcontrol/server/internal/control"
	"net/http"
)

func registerCallContactRoutes(mux *http.ServeMux, auth AuthService, service *control.Service) {
	mux.HandleFunc("GET /v1/device/call-contacts", func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			writeError(w, 401, "invalid_device_credential")
			return
		}
		result, err := service.DeviceCallContacts(r.Context(), token)
		if err != nil {
			controlError(w, err)
			return
		}
		writeJSON(w, 200, result)
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
	mux.HandleFunc("GET /v1/devices/{deviceID}/call-contacts", parentOnly(func(w http.ResponseWriter, r *http.Request, parentID string) {
		result, err := service.CallContacts(r.Context(), parentID, r.PathValue("deviceID"))
		if err != nil {
			controlError(w, err)
			return
		}
		writeJSON(w, 200, result)
	}))
	mux.HandleFunc("PUT /v1/devices/{deviceID}/call-contacts", parentOnly(func(w http.ResponseWriter, r *http.Request, parentID string) {
		var input struct {
			ExpectedVersion *int64 `json:"expectedVersion"`
			Contacts        *[]struct {
				Label    *string `json:"label"`
				Number   *string `json:"number"`
				IsParent *bool   `json:"isParent"`
			} `json:"contacts"`
		}
		if decodeJSONLimit(w, r, &input, 8*1024) != nil || input.ExpectedVersion == nil || input.Contacts == nil {
			writeError(w, 400, "invalid_request")
			return
		}
		contacts := make([]control.CallContact, 0, len(*input.Contacts))
		for _, c := range *input.Contacts {
			if c.Label == nil || c.Number == nil || c.IsParent == nil {
				writeError(w, 400, "invalid_request")
				return
			}
			contacts = append(contacts, control.CallContact{Label: *c.Label, Number: *c.Number, IsParent: *c.IsParent})
		}
		result, err := service.UpdateCallContacts(r.Context(), parentID, r.PathValue("deviceID"), *input.ExpectedVersion, contacts)
		if err != nil {
			controlError(w, err)
			return
		}
		writeJSON(w, 200, result)
	}))
}
