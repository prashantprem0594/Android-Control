package config

import (
	"strings"
	"testing"
)

func TestFCMPrivateFilesAreOptionalButMustBePaired(t *testing.T) {
	for _, values := range []map[string]string{{}, {"APP_CONTROL_FCM_SERVICE_ACCOUNT_FILE": "private-account-fixture"}, {"APP_CONTROL_FCM_TOKEN_KEY_FILE": "private-key-fixture"}, {"APP_CONTROL_FCM_SERVICE_ACCOUNT_FILE": "private-account-fixture", "APP_CONTROL_FCM_TOKEN_KEY_FILE": "private-key-fixture"}} {
		settings, err := Load(func(name string) (string, bool) { value, ok := values[name]; return value, ok })
		if len(values) == 1 {
			if err == nil || strings.Contains(err.Error(), "private-") {
				t.Fatal("partial FCM configuration accepted or printed private path")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(values) == 2 && (settings.WakeupServiceAccountPath != "private-account-fixture" || settings.WakeupTokenKeyPath != "private-key-fixture") {
			t.Fatal("FCM file settings lost")
		}
	}
}
