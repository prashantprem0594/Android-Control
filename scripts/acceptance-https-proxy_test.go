package main

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestParentRoutingFixtureKeyIsFreshAndOutsideApkAssets(t *testing.T) {
	var previous []byte
	for range 2 {
		dir := t.TempDir()
		if err := generate(dir); err != nil {
			t.Fatal("fixture generation failed")
		}
		key, err := os.ReadFile(filepath.Join(dir, "parent-routing-key.bin"))
		if err != nil || len(key) != 32 || bytes.Equal(key, make([]byte, 32)) || bytes.Equal(key, previous) {
			t.Fatal("parent routing fixture key is missing, invalid or reused")
		}
		for _, relative := range []string{"assets/acceptance-root-ca.pem", "assets/acceptance-pin.txt", "generated/com/appcontrol/transport/AcceptanceTrustFixtureGenerated.java"} {
			public, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(relative)))
			if err != nil || bytes.Contains(public, key) || bytes.Contains(public, []byte(base64.StdEncoding.EncodeToString(key))) {
				t.Fatal("private routing key leaked into public fixture assets")
			}
		}
		if _, err := os.Stat(filepath.Join(dir, "assets", "parent-routing-key.bin")); !os.IsNotExist(err) {
			t.Fatal("routing key must not be packaged in APK assets")
		}
		clear(previous)
		previous = key
	}
	clear(previous)
}

func TestContactFailureIsolationAndRecovery(t *testing.T) {
	forwarded := 0
	handler := contactFailureFixture(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded++
		w.WriteHeader(http.StatusOK)
	}))
	check := func(method, path string, status, calls int) {
		t.Helper()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(method, path, nil))
		if response.Code != status || forwarded != calls {
			t.Fatalf("%s %s: status=%d forwarded=%d; want %d/%d", method, path, response.Code, forwarded, status, calls)
		}
	}
	check("GET", "/v1/device/call-contacts", 200, 1)
	check("GET", "/__acceptance/contacts-unavailable", 405, 1)
	check("GET", "/v1/device/call-contacts", 200, 2)
	check("POST", "/__acceptance/contacts-unavailable", 204, 2)
	check("GET", "/v1/device/call-contacts", 503, 2)
	check("GET", "/v1/device/commands", 200, 3)
	check("GET", "/v1/device/policy", 200, 4)
	check("PUT", "/v1/devices/fixture/call-contacts", 200, 5)
	check("GET", "/__acceptance/contacts-available", 405, 5)
	check("GET", "/v1/device/call-contacts", 503, 5)
	check("POST", "/__acceptance/contacts-available", 204, 5)
	check("GET", "/v1/device/call-contacts", 200, 6)
}
