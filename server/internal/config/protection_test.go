package config

import (
	"strings"
	"testing"
	"time"
)

func TestProtectionSettings(t *testing.T) {
	values := map[string]string{
		"APP_CONTROL_REQUEST_TIMEOUT":        "5s",
		"APP_CONTROL_MAX_IN_FLIGHT":          "8",
		"APP_CONTROL_MAX_PASSWORD_IN_FLIGHT": "3",
		"APP_CONTROL_MAX_RATE_PEERS":         "64",
		"APP_CONTROL_RATE_SCALE":             "2",
		"APP_CONTROL_TRUSTED_PROXY_CIDRS":    "192.0.2.99/24, 2001:db8::1/64",
	}
	c, err := Load(func(k string) (string, bool) { v, ok := values[k]; return v, ok })
	if err != nil {
		t.Fatal(err)
	}
	if c.RequestTimeout != 5*time.Second || c.MaxInFlight != 8 || c.MaxPasswordInFlight != 3 || c.MaxRatePeers != 64 || c.RateScale != 2 || len(c.TrustedProxies) != 2 || c.TrustedProxies[0].String() != "192.0.2.0/24" {
		t.Fatal("protection overrides were not applied")
	}
	for _, tc := range []struct{ key, value string }{
		{"APP_CONTROL_REQUEST_TIMEOUT", "121s"},
		{"APP_CONTROL_MAX_IN_FLIGHT", "1025"},
		{"APP_CONTROL_MAX_IN_FLIGHT", "1"},
		{"APP_CONTROL_MAX_PASSWORD_IN_FLIGHT", "17"},
		{"APP_CONTROL_MAX_RATE_PEERS", "65537"},
		{"APP_CONTROL_RATE_SCALE", "101"},
		{"APP_CONTROL_TRUSTED_PROXY_CIDRS", "0.0.0.0/0"},
		{"APP_CONTROL_TRUSTED_PROXY_CIDRS", "::/0"},
		{"APP_CONTROL_TRUSTED_PROXY_CIDRS", "private-invalid-proxy"},
		{"APP_CONTROL_TRUSTED_PROXY_CIDRS", strings.Repeat("192.0.2.1/32,", 32) + "192.0.2.1/32"},
	} {
		_, err := Load(func(k string) (string, bool) { return tc.value, k == tc.key })
		if err == nil {
			t.Fatalf("accepted invalid %s", tc.key)
		}
		if strings.Contains(err.Error(), "private-invalid-proxy") {
			t.Fatal("invalid proxy input echoed in error")
		}
	}
}
