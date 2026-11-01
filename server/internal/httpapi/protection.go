package httpapi

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"
)

type GuardConfig struct {
	Now                 func() time.Time
	TrustedProxies      []netip.Prefix
	RequestTimeout      time.Duration
	MaxInFlight         int
	MaxPasswordInFlight int
	MaxPeers            int
	RateScale           int
}
type requestRate struct{ perMinute, burst float64 }
type requestBucket struct {
	tokens  float64
	updated time.Time
}
type peerBuckets struct {
	lastSeen time.Time
	groups   map[string]requestBucket
}

// RequestGuard bounds per-process ingress state and work. It never trusts an
// address header unless the immediate peer is an explicitly configured proxy.
type RequestGuard struct {
	config    GuardConfig
	mu        sync.Mutex
	peers     map[[32]byte]*peerBuckets
	globals   map[string]requestBucket
	sweepAt   time.Time
	inFlight  chan struct{}
	passwords chan struct{}
}

func NewRequestGuard(c GuardConfig) (*RequestGuard, error) {
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.RequestTimeout == 0 {
		c.RequestTimeout = 12 * time.Second
	}
	if c.MaxInFlight == 0 {
		c.MaxInFlight = 128
	}
	if c.MaxPasswordInFlight == 0 {
		c.MaxPasswordInFlight = 2
	}
	if c.MaxPeers == 0 {
		c.MaxPeers = 4096
	}
	if c.RateScale == 0 {
		c.RateScale = 1
	}
	if c.RequestTimeout <= 0 || c.RequestTimeout > 2*time.Minute || c.MaxInFlight < 1 || c.MaxInFlight > 1024 || c.MaxPasswordInFlight < 1 || c.MaxPasswordInFlight > 16 || c.MaxPasswordInFlight > c.MaxInFlight || c.MaxPeers < 1 || c.MaxPeers > 65536 || c.RateScale < 1 || c.RateScale > 100 || len(c.TrustedProxies) > 32 {
		return nil, fmt.Errorf("invalid request protection settings")
	}
	c.TrustedProxies = append([]netip.Prefix{}, c.TrustedProxies...)
	for _, prefix := range c.TrustedProxies {
		if !prefix.IsValid() || prefix.Bits() == 0 {
			return nil, fmt.Errorf("trusted proxies must be specific IP prefixes")
		}
	}
	return &RequestGuard{config: c, peers: make(map[[32]byte]*peerBuckets), globals: make(map[string]requestBucket), inFlight: make(chan struct{}, c.MaxInFlight), passwords: make(chan struct{}, c.MaxPasswordInFlight)}, nil
}

func requestGroup(r *http.Request) string {
	if r.Method == "GET" || r.Method == "HEAD" {
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			return "health"
		}
	}
	if r.Method == "POST" {
<<<<<<< HEAD
		if r.URL.Path == "/v1/device-recovery/requests" ||
			strings.HasPrefix(r.URL.Path, "/v1/device-recovery/requests/") &&
				(strings.HasSuffix(r.URL.Path, "/proof") || strings.HasSuffix(r.URL.Path, "/status")) {
			return "device-recovery"
		}
=======
>>>>>>> origin/main
		switch r.URL.Path {
		case "/v1/auth/register", "/v1/auth/login":
			return "password"
		case "/v1/enrollments", "/v1/enrollments/complete":
			return "enrollment"
		case "/v1/device-auth/challenges", "/v1/device-auth/sessions":
			return "device-auth"
		case "/v1/auth/refresh", "/v1/auth/logout":
			return "session"
		}
	}
	return "api"
}
func rates(group string, scale int) (peer, global requestRate) {
	switch group {
	case "password":
<<<<<<< HEAD
		peer = requestRate{20, 20}
		global = requestRate{60, 20}
=======
		peer = requestRate{10, 5}
		global = requestRate{60, 10}
>>>>>>> origin/main
	case "enrollment":
		peer = requestRate{60, 20}
		global = requestRate{240, 60}
	case "device-auth":
		peer = requestRate{120, 30}
		global = requestRate{600, 120}
<<<<<<< HEAD
	case "device-recovery":
		peer = requestRate{10, 5}
		global = requestRate{120, 30}
=======
>>>>>>> origin/main
	case "session":
		peer = requestRate{120, 30}
		global = requestRate{600, 120}
	case "health":
		peer = requestRate{600, 30}
		global = requestRate{600, 30}
	default:
		peer = requestRate{300, 120}
		global = requestRate{3000, 600}
	}
	factor := float64(scale)
	peer.perMinute *= factor
	peer.burst *= factor
	global.perMinute *= factor
	global.burst *= factor
	return
}
func refill(bucket requestBucket, rate requestRate, now time.Time) requestBucket {
	if bucket.updated.IsZero() {
		return requestBucket{rate.burst, now}
	}
	if now.After(bucket.updated) {
		bucket.tokens = math.Min(rate.burst, bucket.tokens+now.Sub(bucket.updated).Seconds()*rate.perMinute/60)
		bucket.updated = now
	}
	return bucket
}
func retrySeconds(tokens float64, rate requestRate) int {
	if tokens >= 1 {
		return 0
	}
	return max(1, int(math.Ceil((1-tokens)*60/rate.perMinute)))
}
func (g *RequestGuard) allow(key [32]byte, group string) (bool, int) {
	now := g.config.Now()
	peerRate, globalRate := rates(group, g.config.RateScale)
	g.mu.Lock()
	defer g.mu.Unlock()
	global := refill(g.globals[group], globalRate, now)
	g.globals[group] = global
	if group == "health" {
		if global.tokens < 1 {
			return false, retrySeconds(global.tokens, globalRate)
		}
		global.tokens--
		g.globals[group] = global
		return true, 0
	}
	if !now.Before(g.sweepAt) {
		for address, peer := range g.peers {
			if now.Sub(peer.lastSeen) >= 15*time.Minute {
				delete(g.peers, address)
			}
		}
		g.sweepAt = now.Add(time.Minute)
	}
	if global.tokens < 1 {
		return false, retrySeconds(global.tokens, globalRate)
	}
	peer := g.peers[key]
	if peer == nil {
		if len(g.peers) >= g.config.MaxPeers {
			return false, 60
		}
		peer = &peerBuckets{lastSeen: now, groups: make(map[string]requestBucket)}
		g.peers[key] = peer
	}
	if now.After(peer.lastSeen) {
		peer.lastSeen = now
	}
	bucket := refill(peer.groups[group], peerRate, now)
	peer.groups[group] = bucket
	if bucket.tokens < 1 {
		return false, retrySeconds(bucket.tokens, peerRate)
	}
	bucket.tokens--
	global.tokens--
	peer.groups[group] = bucket
	g.globals[group] = global
	return true, 0
}

func (g *RequestGuard) trusted(address netip.Addr) bool {
	for _, prefix := range g.config.TrustedProxies {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}
func peerAddress(remote string) netip.Addr {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	address, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return address.Unmap().WithZone("")
}
func (g *RequestGuard) clientKey(r *http.Request) [32]byte {
	address := peerAddress(r.RemoteAddr)
	if address.IsValid() && g.trusted(address) {
		values := r.Header.Values("X-Forwarded-For")
		size := 0
		for _, value := range values {
			size += len(value) + 1
		}
		if size > 0 && size <= 1024 {
			parts := strings.Split(strings.Join(values, ","), ",")
			if len(parts) <= 16 {
				chain := make([]netip.Addr, 0, len(parts))
				valid := true
				for _, part := range parts {
					value, err := netip.ParseAddr(strings.TrimSpace(part))
					if err != nil || value.Zone() != "" {
						valid = false
						break
					}
					chain = append(chain, value.Unmap())
				}
				if valid {
					for i := len(chain) - 1; i >= 0; i-- {
						address = chain[i]
						if !g.trusted(address) {
							break
						}
					}
				}
			}
		}
	}
	key := "unknown"
	if address.IsValid() {
		bits := 64
		if address.Is4() {
			bits = 32
		}
		key = netip.PrefixFrom(address, bits).Masked().String()
	}
	return sha256.Sum256([]byte(key))
}

func (g *RequestGuard) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.RequestURI) > 8192 {
			writeError(w, 414, "request_target_too_long")
			return
		}
		if encoding := strings.TrimSpace(r.Header.Get("Content-Encoding")); encoding != "" && !strings.EqualFold(encoding, "identity") {
			writeError(w, 415, "unsupported_content_encoding")
			return
		}
		if r.ContentLength > 1<<20 {
			writeError(w, 400, "invalid_request")
			return
		}
		select {
		case g.inFlight <- struct{}{}:
			defer func() { <-g.inFlight }()
		default:
			w.Header().Set("Retry-After", "1")
			writeError(w, 503, "server_busy")
			return
		}
		group := requestGroup(r)
		if allowed, retry := g.allow(g.clientKey(r), group); !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(retry))
			writeError(w, 429, "rate_limited")
			return
		}
		if group == "password" {
			select {
			case g.passwords <- struct{}{}:
				defer func() { <-g.passwords }()
			default:
				w.Header().Set("Retry-After", "1")
				writeError(w, 503, "server_busy")
				return
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), g.config.RequestTimeout)
		defer cancel()
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
