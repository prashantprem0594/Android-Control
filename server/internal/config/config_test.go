package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	loaded, err := Load(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatalf("load defaults: %v", err)
	}

	if loaded.HTTPAddress != ":8080" {
		t.Fatalf("unexpected address: %q", loaded.HTTPAddress)
	}
	if loaded.DatabasePath != "data/app-control.db" {
		t.Fatalf("unexpected database path: %q", loaded.DatabasePath)
	}
	if loaded.ShutdownTimeout != 10*time.Second {
		t.Fatalf("unexpected shutdown timeout: %v", loaded.ShutdownTimeout)
	}
	if loaded.MaxHeaderBytes != 1<<20 {
		t.Fatalf("unexpected maximum header size: %d", loaded.MaxHeaderBytes)
	}
	if loaded.RemoteCallsEnabled {
		t.Fatal("remote call commands must default to disabled")
	}
}

func TestLoadOverrides(t *testing.T) {
	values := map[string]string{
<<<<<<< HEAD
		"APP_CONTROL_HTTP_ADDRESS":           "127.0.0.1:9090",
		"APP_CONTROL_SHUTDOWN_TIMEOUT":       "3s",
		"APP_CONTROL_MAX_HEADER_BYTES":       "4096",
		"APP_CONTROL_READ_HEADER_TIMEOUT":    "2s",
		"APP_CONTROL_DATABASE_PATH":          "data/test.db",
		"APP_CONTROL_REMOTE_CALLS_ENABLED":   "true",
		"APP_CONTROL_RECOVERY_HMAC_KEY_FILE": "private/recovery-key.bin",
=======
		"APP_CONTROL_HTTP_ADDRESS":         "127.0.0.1:9090",
		"APP_CONTROL_SHUTDOWN_TIMEOUT":     "3s",
		"APP_CONTROL_MAX_HEADER_BYTES":     "4096",
		"APP_CONTROL_READ_HEADER_TIMEOUT":  "2s",
		"APP_CONTROL_DATABASE_PATH":        "data/test.db",
		"APP_CONTROL_REMOTE_CALLS_ENABLED": "true",
>>>>>>> origin/main
	}

	loaded, err := Load(func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	})
	if err != nil {
		t.Fatalf("load overrides: %v", err)
	}

<<<<<<< HEAD
	if loaded.HTTPAddress != "127.0.0.1:9090" || loaded.DatabasePath != "data/test.db" || loaded.ShutdownTimeout != 3*time.Second || loaded.MaxHeaderBytes != 4096 || !loaded.RemoteCallsEnabled || loaded.RecoveryHMACKeyPath != "private/recovery-key.bin" {
=======
	if loaded.HTTPAddress != "127.0.0.1:9090" || loaded.DatabasePath != "data/test.db" || loaded.ShutdownTimeout != 3*time.Second || loaded.MaxHeaderBytes != 4096 || !loaded.RemoteCallsEnabled {
>>>>>>> origin/main
		t.Fatalf("overrides were not applied: %+v", loaded)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	tests := map[string]map[string]string{
		"address":       {"APP_CONTROL_HTTP_ADDRESS": "missing-port"},
		"duration":      {"APP_CONTROL_SHUTDOWN_TIMEOUT": "later"},
		"zero duration": {"APP_CONTROL_READ_TIMEOUT": "0s"},
		"header size":   {"APP_CONTROL_MAX_HEADER_BYTES": "0"},
		"database path": {"APP_CONTROL_DATABASE_PATH": "   "},
		"remote calls":  {"APP_CONTROL_REMOTE_CALLS_ENABLED": "yes"},
	}

	for name, values := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Load(func(key string) (string, bool) {
				value, ok := values[key]
				return value, ok
			})
			if err == nil || strings.TrimSpace(err.Error()) == "" {
				t.Fatal("expected a descriptive configuration error")
			}
		})
	}
}
