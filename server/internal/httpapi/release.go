package httpapi

import (
	"appcontrol/server/internal/release"
	"net/http"
	"strconv"
	"time"
)

func registerReleaseRoute(mux *http.ServeMux, document *release.Document) {
	mux.HandleFunc("GET /v1/releases/current", func(w http.ResponseWriter, r *http.Request) {
		raw, err := document.Bytes(time.Now().UTC())
		if err != nil {
			writeError(w, 503, "release_unavailable")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
		w.WriteHeader(200)
		if r.Method != "HEAD" {
			_, _ = w.Write(raw)
		}
	})
}
