package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"time"

	"appcontrol/server/internal/auth"
)

func testGuard(t *testing.T, c GuardConfig) *RequestGuard {
	t.Helper()
	g, err := NewRequestGuard(c)
	if err != nil {
		t.Fatal(err)
	}
	return g
}
func guardKey(g *RequestGuard, remote string, headers ...string) [32]byte {
	r := httptest.NewRequest("GET", "/healthz", nil)
	r.RemoteAddr = remote
	for _, value := range headers {
		r.Header.Add("X-Forwarded-For", value)
	}
	return g.clientKey(r)
}

func TestRequestBucketsBoundPeersAndGlobals(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	g := testGuard(t, GuardConfig{Now: func() time.Time { return now }})
	a := guardKey(g, "192.0.2.1:1234")
	b := guardKey(g, "192.0.2.2:1234")
	c := guardKey(g, "192.0.2.3:1234")
	for i := 0; i < 5; i++ {
		if ok, _ := g.allow(a, "password"); !ok {
			t.Fatal("initial peer burst rejected")
		}
	}
	if ok, retry := g.allow(a, "password"); ok || retry != 6 {
		t.Fatalf("peer quota not enforced: %v %d", ok, retry)
	}
	for i := 0; i < 5; i++ {
		if ok, _ := g.allow(b, "password"); !ok {
			t.Fatal("peer denial consumed global quota")
		}
	}
	if ok, retry := g.allow(c, "password"); ok || retry != 1 {
		t.Fatal("global password quota not enforced")
	}
	now = now.Add(time.Second)
	if ok, _ := g.allow(c, "password"); !ok {
		t.Fatal("global quota did not refill")
	}
	now = now.Add(5 * time.Second)
	if ok, _ := g.allow(a, "password"); !ok {
		t.Fatal("peer quota did not refill")
	}
	now = now.Add(-5 * time.Second)
	if ok, _ := g.allow(a, "password"); ok {
		t.Fatal("clock rollback replenished tokens")
	}
}

func TestPeerTableIsBoundedAndHealthDoesNotAllocatePeers(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	g := testGuard(t, GuardConfig{MaxPeers: 2, Now: func() time.Time { return now }})
	for _, ip := range []string{"192.0.2.1:1", "192.0.2.2:2"} {
		if ok, _ := g.allow(guardKey(g, ip), "api"); !ok {
			t.Fatal("initial peers denied")
		}
	}
	third := guardKey(g, "192.0.2.3:3")
	if ok, _ := g.allow(third, "api"); ok || len(g.peers) != 2 {
		t.Fatal("peer capacity was bypassed")
	}
	if ok, _ := g.allow(third, "health"); !ok || len(g.peers) != 2 {
		t.Fatal("health probes allocated peer state or failed solely on peer capacity")
	}
	now = now.Add(16 * time.Minute)
	if ok, _ := g.allow(third, "api"); !ok || len(g.peers) != 1 {
		t.Fatal("idle peer state did not expire")
	}
}

func TestProxyTrustAndAddressCanonicalization(t *testing.T) {
	plain := testGuard(t, GuardConfig{})
	direct := guardKey(plain, "198.51.100.8:1")
	if guardKey(plain, "198.51.100.8:1", "203.0.113.66") != direct {
		t.Fatal("untrusted forwarded header changed identity")
	}
	if guardKey(plain, "[::ffff:198.51.100.8]:1") != direct {
		t.Fatal("IPv4-mapped address bypassed peer quota")
	}
	if guardKey(plain, "[2001:db8:1::1]:1") != guardKey(plain, "[2001:db8:1::abcd]:2") {
		t.Fatal("IPv6 host rotation bypassed /64 quota")
	}
	if guardKey(plain, "[2001:db8:1::1]:1") == guardKey(plain, "[2001:db8:2::1]:1") {
		t.Fatal("different IPv6 networks collapsed")
	}
	trusted := testGuard(t, GuardConfig{TrustedProxies: []netip.Prefix{netip.MustParsePrefix("10.0.0.1/32"), netip.MustParsePrefix("10.0.0.2/32")}})
	if guardKey(trusted, "10.0.0.1:1", "203.0.113.66,198.51.100.8,10.0.0.2") != direct {
		t.Fatal("proxy chain did not stop at rightmost untrusted client")
	}
	if guardKey(trusted, "10.0.0.1:1", "203.0.113.66", "198.51.100.8") != direct {
		t.Fatal("multiple forwarded header lines were mishandled")
	}
	peer := guardKey(trusted, "10.0.0.1:1")
	for _, header := range []string{"bad-address,198.51.100.8", strings.Repeat("a", 1025), strings.Repeat("198.51.100.8,", 17), "fe80::1%eth0"} {
		if guardKey(trusted, "10.0.0.1:1", header) != peer {
			t.Fatal("invalid forwarded chain did not fall back to immediate peer")
		}
	}
	if guardKey(plain, "not-an-address") != sha256.Sum256([]byte("unknown")) {
		t.Fatal("invalid peer identity was not bounded")
	}
}

func TestHTTPRateLimitSkipsPasswordWorkAndReturnsRetry(t *testing.T) {
	fake := completeFakeAuth()
	calls := 0
	fake.login = func(context.Context, string, string) (auth.Parent, auth.TokenPair, error) {
		calls++
		return auth.Parent{}, auth.TokenPair{}, auth.ErrInvalidCredentials
	}
	handler := NewHandler(Options{Auth: fake})
	for i := 0; i < 21; i++ {
		r := httptest.NewRequest("POST", "/v1/auth/login", strings.NewReader(`{"email":"fixture@example.com","password":"fixture-password"}`))
		r.RemoteAddr = "192.0.2.1:1"
		r.Header.Set("X-Forwarded-For", "203.0.113."+strconv.Itoa(i+1))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if i < 20 && w.Code != 401 {
			t.Fatal("initial login response changed")
		}
		if i == 20 && (w.Code != 429 || w.Header().Get("Retry-After") == "" || w.Header().Get("Cache-Control") != "no-store") {
			t.Fatal("missing safe rate-limit response")
		}
	}
	if calls != 20 {
		t.Fatal("throttled request reached password work")
	}
}

func TestPasswordConcurrencyIsBoundedWithoutBlockingHealth(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	done := make(chan struct{})
	fake := completeFakeAuth()
	fake.login = func(context.Context, string, string) (auth.Parent, auth.TokenPair, error) {
		entered <- struct{}{}
		<-release
		return auth.Parent{}, auth.TokenPair{}, nil
	}
	handler := NewHandler(Options{Auth: fake, Guard: testGuard(t, GuardConfig{MaxInFlight: 4, MaxPasswordInFlight: 1, RateScale: 100})})
	request := func() *http.Request {
		return httptest.NewRequest("POST", "/v1/auth/login", strings.NewReader(`{"email":"fixture@example.com","password":"fixture-password"}`))
	}
	first := httptest.NewRecorder()
	go func() { defer close(done); handler.ServeHTTP(first, request()) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("password work did not start")
	}
	second := httptest.NewRecorder()
	handler.ServeHTTP(second, request())
	if second.Code != 503 || second.Header().Get("Retry-After") != "1" {
		t.Fatal("password concurrency cap failed")
	}
	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest("GET", "/healthz", nil))
	if health.Code != 200 {
		t.Fatal("password slot blocked health work")
	}
	close(release)
	<-done
	if first.Code != 200 {
		t.Fatal("admitted request failed")
	}
	third := httptest.NewRecorder()
	handler.ServeHTTP(third, request())
	if third.Code != 200 {
		t.Fatal("password slot leaked")
	}
}

func TestAdmissionContextAndInputBounds(t *testing.T) {
	g := testGuard(t, GuardConfig{RequestTimeout: 20 * time.Millisecond})
	observed := false
	h := NewHandler(Options{Guard: g, Ready: func(ctx context.Context) error {
		<-ctx.Done()
		observed = errors.Is(ctx.Err(), context.DeadlineExceeded)
		return ctx.Err()
	}})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/readyz", nil))
	if !observed || w.Code != 503 {
		t.Fatal("request context budget was not propagated")
	}
	called := false
	direct := g.wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	for _, tc := range []struct {
		encoding string
		length   int64
		uri      string
		status   int
	}{{"gzip", 1, "/", 415}, {"", 1<<20 + 1, "/", 400}, {"", 0, "/" + strings.Repeat("x", 8192), 414}} {
		r := httptest.NewRequest("POST", "/", nil)
		r.Header.Set("Content-Encoding", tc.encoding)
		r.ContentLength = tc.length
		r.RequestURI = tc.uri
		w := httptest.NewRecorder()
		direct.ServeHTTP(w, r)
		if w.Code != tc.status || called {
			t.Fatal("invalid input reached handler")
		}
	}
	chunked := g.wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.Copy(io.Discard, r.Body)
		if err == nil {
			t.Error("chunked body was unbounded")
		}
		writeError(w, 400, "invalid_request")
	}))
	r := httptest.NewRequest("PUT", "/v1/device/apps", strings.NewReader(strings.Repeat("x", (1<<20)+1)))
	r.ContentLength = -1
	chunked.ServeHTTP(httptest.NewRecorder(), r)
}

func TestLogsUseTemplatesAndSuppressUnknownInputAndPanicValues(t *testing.T) {
	var output bytes.Buffer
	fake := completeFakeAuth()
	fake.authenticate = func(context.Context, string) (auth.Parent, string, error) { return auth.Parent{}, "", nil }
	fake.login = func(context.Context, string, string) (auth.Parent, auth.TokenPair, error) {
		panic("private-panic-secret")
	}
	h := NewHandler(Options{Auth: fake, Logger: slog.New(slog.NewJSONHandler(&output, nil))})
	for _, path := range []string{"/unknown/private-path-secret?value=private-query-secret", "/v1/device-auth/private-token-secret"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("PRIVATE-METHOD-SECRET", "/healthz", nil))
	r := httptest.NewRequest("POST", "/v1/auth/login", strings.NewReader(`{"email":"private-email-secret","password":"private-password-secret"}`))
	r.Header.Set("Authorization", "Bearer private-header-secret")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 500 || w.Body.String() != "{\"error\":\"server_error\"}\n" {
		t.Fatal("panic response was not generic")
	}
	for _, secret := range []string{"private-path-secret", "private-query-secret", "private-token-secret", "PRIVATE-METHOD-SECRET", "private-panic-secret", "private-email-secret", "private-password-secret", "private-header-secret"} {
		if strings.Contains(output.String(), secret) {
			t.Fatal("request or panic data leaked into logs")
		}
	}
	if !strings.Contains(output.String(), `"path":"unmatched"`) || !strings.Contains(output.String(), `"handler_panic":true`) {
		t.Fatal("safe log metadata missing")
	}
}

func TestProtectionRejectsUnsafeConfiguration(t *testing.T) {
	for _, c := range []GuardConfig{{MaxPeers: -1}, {MaxInFlight: 1, MaxPasswordInFlight: 2}, {MaxPasswordInFlight: 17}, {RateScale: 101}, {TrustedProxies: []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0")}}} {
		if _, err := NewRequestGuard(c); err == nil {
			t.Fatal("invalid protection settings accepted")
		}
	}
}

func TestMatchedRouteLogOmitsIdentifierAndAbortsPartialPanic(t *testing.T) {
	var output bytes.Buffer
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/devices/{deviceID}", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("partial"))
		panic("private-panic-value")
	})
	h := requestLogging(slog.New(slog.NewJSONHandler(&output, nil)), mux, mux)
	func() {
		defer func() {
			if recover() != http.ErrAbortHandler {
				t.Error("partial response did not abort the transport")
			}
		}()
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/v1/devices/private-device-id?token=private-query", nil))
		t.Error("partial panic returned normally")
	}()
	if !strings.Contains(output.String(), `"path":"/v1/devices/{deviceID}"`) || !strings.Contains(output.String(), `"aborted":true`) {
		t.Fatal("route template or abort metadata missing")
	}
	for _, secret := range []string{"private-device-id", "private-query", "private-panic-value"} {
		if strings.Contains(output.String(), secret) {
			t.Fatal("dynamic route or panic secret leaked")
		}
	}
}

func TestGlobalAdmissionAndPanicReleaseSlots(t *testing.T) {
	g := testGuard(t, GuardConfig{MaxInFlight: 1, MaxPasswordInFlight: 1, RateScale: 100})
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	h := g.wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { close(entered); <-release }))
	go func() {
		defer close(done)
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	}()
	<-entered
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	close(release)
	<-done
	if w.Code != 503 || w.Header().Get("Retry-After") != "1" {
		t.Fatal("global admission did not reject excess work")
	}
	panics := g.wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("fixture") }))
	func() {
		defer func() { _ = recover() }()
		panics.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/auth/login", nil))
	}()
	w = httptest.NewRecorder()
	g.wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })).ServeHTTP(w, httptest.NewRequest("POST", "/v1/auth/login", nil))
	if w.Code != 204 {
		t.Fatal("panic leaked global or password admission slot")
	}
}
