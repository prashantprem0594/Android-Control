package httpapi

import (
	"appcontrol/server/internal/control"
	"net/http"
	"net/url"
	"strconv"
)

func registerCommandHistoryRoute(mux *http.ServeMux, authentication AuthService, service *control.Service) {
	mux.HandleFunc("GET /v1/devices/{deviceID}/commands", func(w http.ResponseWriter, r *http.Request) {
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
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			writeError(w, 400, "invalid_page")
			return
		}
		for key, values := range query {
			if (key != "limit" && key != "after") || len(values) != 1 || values[0] == "" {
				writeError(w, 400, "invalid_page")
				return
			}
		}
		limit := 50
		if value := query.Get("limit"); value != "" {
			limit, err = strconv.Atoi(value)
			if err != nil {
				writeError(w, 400, "invalid_page")
				return
			}
		}
		page, err := service.Commands(r.Context(), parent.ID, r.PathValue("deviceID"), query.Get("after"), limit)
		if err != nil {
			controlError(w, err)
			return
		}
		writeJSON(w, 200, page)
	})
}
