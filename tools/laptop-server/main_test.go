package main

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fixture(t *testing.T) (string, *state) {
	t.Helper()
	directory := filepath.Join(t.TempDir(), "state")
	if err := initialize(directory, 8443, 18080, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	current, err := loadState(directory, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return directory, current
}
func TestInitializationIsPrivatePersistentAndRefusesIdentityOverwrite(t *testing.T) {
	directory, current := fixture(t)
	if current.CurrentPin == current.BackupPin || current.HTTPSOrigin != "https://localhost:8443" || current.BackendAddress != "127.0.0.1:18080" {
		t.Fatal("incorrect public identity/ports")
	}
	before := map[string]string{}
	if err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			before[path] = fingerprint(raw)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := initialize(directory, 8443, 18080, time.Now()); err == nil {
		t.Fatal("existing identity overwritten")
	}
	for path, expected := range before {
		raw, err := os.ReadFile(path)
		if err != nil || fingerprint(raw) != expected {
			t.Fatal("failed reinitialization modified state")
		}
	}
	raw, err := os.ReadFile(filepath.Join(directory, metadataName))
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"PRIVATE KEY", "shutdown-token", "routing-key", "backup-key"} {
		if strings.Contains(string(raw), private) {
			t.Fatal("private material leaked into public metadata")
		}
	}
	if originForPort(443) != "https://localhost" {
		t.Fatal("default HTTPS port not canonical")
	}
}
func TestCorruptExpiredAndRedirectedStateFailsClosed(t *testing.T) {
	for _, mode := range []string{"pin", "root-path", "key-length", "key-mismatch", "expired", "incomplete"} {
		t.Run(mode, func(t *testing.T) {
			directory, current := fixture(t)
			now := time.Now()
			switch mode {
			case "pin":
				current.CurrentPin = current.BackupPin
				writeMetadata(t, directory, current.metadata)
			case "root-path":
				current.RootCertificateFile = "../root-ca.pem"
				writeMetadata(t, directory, current.metadata)
			case "key-length":
				if err := os.WriteFile(filepath.Join(directory, "private", "backup-key.bin"), []byte("short"), 0600); err != nil {
					t.Fatal(err)
				}
			case "key-mismatch":
				raw, err := os.ReadFile(filepath.Join(directory, "private", "backup-leaf.key"))
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(directory, "private", "leaf.key"), raw, 0600); err != nil {
					t.Fatal(err)
				}
			case "expired":
				now = now.Add(91 * 24 * time.Hour)
			case "incomplete":
				if err := os.WriteFile(filepath.Join(directory, ".initializing"), []byte("unfinished"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := loadState(directory, now); err == nil {
				t.Fatal("invalid state accepted")
			}
		})
	}
}
func writeMetadata(t *testing.T, directory string, value metadata) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, metadataName), raw, 0600); err != nil {
		t.Fatal(err)
	}
}
func TestPinnedHTTPSProxyAndAuthenticatedShutdown(t *testing.T) {
	_, current := fixture(t)
	var incoming atomic.Bool
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("X-Forwarded-For"), "untrusted") || r.Header.Get("Forwarded") != "" || r.Header.Get("X-Real-IP") != "" || r.Header.Get("X-Forwarded-Proto") != "https" {
			t.Error("untrusted proxy headers retained")
		}
		incoming.Store(true)
		io.WriteString(w, "ready")
	}))
	defer backend.Close()
	current.BackendAddress = strings.TrimPrefix(backend.URL, "http://")
	var router http.Handler
	proxy := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { router.ServeHTTP(w, r) }))
	port := proxy.Listener.Addr().(*net.TCPAddr).Port
	current.HTTPSOrigin = originForPort(port)
	var stopped atomic.Bool
	var err error
	router, err = handler(current, func() { stopped.Store(true) })
	if err != nil {
		t.Fatal(err)
	}
	proxy.TLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{current.pair}}
	proxy.StartTLS()
	defer proxy.Close()
	c := client(current)
	defer c.CloseIdleConnections()
	call, _ := http.NewRequest(http.MethodGet, current.HTTPSOrigin+"/readyz", nil)
	call.Header.Set("X-Forwarded-For", "untrusted")
	call.Header.Set("Forwarded", "for=untrusted")
	call.Header.Set("X-Real-IP", "untrusted")
	response, err := c.Do(call)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 || !incoming.Load() {
		t.Fatal("pinned upstream request did not pass")
	}
	call, _ = http.NewRequest(http.MethodGet, current.HTTPSOrigin+"/readyz", nil)
	call.Host = "wrong.invalid"
	response, err = c.Do(call)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusMisdirectedRequest {
		t.Fatal("foreign HTTP Host accepted")
	}
	call, _ = http.NewRequest(http.MethodPost, current.HTTPSOrigin+shutdownPath, nil)
	response, err = c.Do(call)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 404 || stopped.Load() {
		t.Fatal("unauthenticated shutdown accepted")
	}
	call.Header.Set("X-App-Control-Laptop-Token", base64.RawURLEncoding.EncodeToString(current.shutdownToken))
	response, err = c.Do(call)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("private shutdown refused")
	}
	deadline := time.Now().Add(time.Second)
	for !stopped.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !stopped.Load() {
		t.Fatal("shutdown callback missing")
	}
	changed := *current
	changed.CurrentPin = changed.BackupPin
	wrong := client(&changed)
	defer wrong.CloseIdleConnections()
	if _, err := wrong.Get(current.HTTPSOrigin + "/readyz"); err == nil {
		t.Fatal("wrong pin accepted")
	}
	_, other := fixture(t)
	changed = *current
	changed.root = other.root
	untrusted := client(&changed)
	defer untrusted.CloseIdleConnections()
	if _, err := untrusted.Get(current.HTTPSOrigin + "/readyz"); err == nil {
		t.Fatal("wrong CA accepted")
	}
}
func TestServerBindsLoopbackAndStopsWithinBoundedContext(t *testing.T) {
	_, current := fixture(t)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	current.HTTPSOrigin = "https://localhost:" + strconv.Itoa(port)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, current, io.Discard) }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("localhost helper did not stop within bound")
	}
}
