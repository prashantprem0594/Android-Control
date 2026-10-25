package httpapi

import (
	"appcontrol/server/internal/pairing"
	"errors"
	"net/http"
	"net/url"
	"strconv"
)

func registerDeviceListRoute(mux *http.ServeMux, authentication AuthService, service *pairing.Service) {
	mux.HandleFunc("GET /v1/devices", func(w http.ResponseWriter, r *http.Request) {
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
		limit := 50
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			writeError(w, 400, "invalid_page")
			return
		}
		for key, values := range query {
			if (key != "after" && key != "limit") || len(values) != 1 || values[0] == "" {
				writeError(w, 400, "invalid_page")
				return
			}
		}
		if value := query.Get("limit"); value != "" {
			limit, err = strconv.Atoi(value)
			if err != nil {
				writeError(w, 400, "invalid_page")
				return
			}
		}
		page, err := service.Devices(r.Context(), parent.ID, query.Get("after"), limit)
		if errors.Is(err, pairing.ErrInvalidPage) {
			writeError(w, 400, "invalid_page")
			return
		}
		if err != nil {
			writeError(w, 500, "server_error")
			return
		}
		writeJSON(w, 200, page)
	})
}
