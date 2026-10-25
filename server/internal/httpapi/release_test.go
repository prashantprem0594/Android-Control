package httpapi

import (
	"appcontrol/server/internal/release"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func releaseDocument(t *testing.T, now time.Time) (*release.Document, []byte) {
	t.Helper()
	origin := "https://updates.example.org"
	certificate := strings.Repeat("a", 64)
	manifest := release.Manifest{Version: 1, Sequence: 1, IssuedAt: now, ExpiresAt: now.Add(time.Hour), Origin: origin}
	for _, name := range []string{"parent", "child"} {
		manifest.Apps = append(manifest.Apps, release.Artifact{Package: "com.appcontrol." + name, VersionCode: 2, VersionName: "0.1.1", MinSDK: 33, TargetSDK: 37, URL: origin + "/releases/1/" + name + "-release.apk", Bytes: 1024, SHA256: strings.Repeat("b", 64), SigningCertificateSHA256: certificate})
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := release.Sign(manifest, private, now)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	document, err := release.NewDocument(raw, der, origin, certificate, 1)
	if err != nil {
		t.Fatal(err)
	}
	return document, raw
}
func TestReleaseMetadataRoutePublicReadonlyAndExpiryBounded(t *testing.T) {
	document, raw := releaseDocument(t, time.Now().UTC())
	handler := NewHandler(Options{Release: document})
	for _, method := range []string{"GET", "HEAD"} {
		r := httptest.NewRequest(method, "/v1/releases/current", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "application/json" || w.Header().Get("Content-Length") != strconv.Itoa(len(raw)) {
			t.Fatal("public release metadata response invalid")
		}
		if method == "GET" && !bytes.Equal(w.Body.Bytes(), raw) || method == "HEAD" && w.Body.Len() != 0 {
			t.Fatal("release metadata body differs")
		}
	}
	r := httptest.NewRequest("POST", "/v1/releases/current", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 405 {
		t.Fatal("public release route accepted writes")
	}
	expired, _ := releaseDocument(t, time.Now().UTC().Add(-2*time.Hour))
	r = httptest.NewRequest("GET", "/v1/releases/current", nil)
	w = httptest.NewRecorder()
	NewHandler(Options{Release: expired}).ServeHTTP(w, r)
	if w.Code != 503 || bytes.Contains(w.Body.Bytes(), []byte("payload")) {
		t.Fatal("expired metadata served or reflected")
	}
	r = httptest.NewRequest("GET", "/healthz", nil)
	w = httptest.NewRecorder()
	NewHandler(Options{Release: expired}).ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal("expired optional metadata disabled server health")
	}
	r = httptest.NewRequest("GET", "/v1/releases/current", nil)
	w = httptest.NewRecorder()
	NewHandler(Options{}).ServeHTTP(w, r)
	if w.Code != 404 {
		t.Fatal("unconfigured release route enabled")
	}
}
