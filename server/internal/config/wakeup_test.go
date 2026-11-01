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
<<<<<<< HEAD

func TestParentFCMKeyIsIndependentOfChildSenderConfiguration(t *testing.T) {
	values := map[string]string{"APP_CONTROL_PARENT_FCM_TOKEN_KEY_FILE": "private-parent-routing-key"}
	settings, err := Load(func(name string) (string, bool) { value, ok := values[name]; return value, ok })
	if err != nil || settings.ParentWakeupTokenKeyPath != "private-parent-routing-key" ||
		settings.WakeupServiceAccountPath != "" || settings.WakeupTokenKeyPath != "" {
		t.Fatal("parent routing key must be independently configurable")
	}
}

func TestParentFCMSenderIsOptInAndSeparatelyConfigured(t *testing.T) {
	settings, err := Load(func(string) (string, bool) { return "", false })
	if err != nil || settings.ParentWakeupSenderEnabled {
		t.Fatal("parent sender must default to disabled")
	}
	for _, values := range []map[string]string{
		{"APP_CONTROL_PARENT_FCM_SENDER_ENABLED": "true"},
		{"APP_CONTROL_PARENT_FCM_SENDER_ENABLED": "true", "APP_CONTROL_PARENT_FCM_TOKEN_KEY_FILE": "parent-key"},
		{"APP_CONTROL_PARENT_FCM_SENDER_ENABLED": "yes"},
	} {
		if _, err := Load(func(name string) (string, bool) { value, ok := values[name]; return value, ok }); err == nil || strings.Contains(err.Error(), "parent-key") {
			t.Fatal("incomplete or invalid parent sender configuration accepted")
		}
	}
	values := map[string]string{
		"APP_CONTROL_PARENT_FCM_SENDER_ENABLED":       "true",
		"APP_CONTROL_PARENT_FCM_TOKEN_KEY_FILE":       "parent-key",
		"APP_CONTROL_PARENT_FCM_SERVICE_ACCOUNT_FILE": "parent-account",
	}
	settings, err = Load(func(name string) (string, bool) { value, ok := values[name]; return value, ok })
	if err != nil || !settings.ParentWakeupSenderEnabled ||
		settings.ParentWakeupTokenKeyPath != "parent-key" ||
		settings.ParentWakeupServiceAccountPath != "parent-account" ||
		settings.WakeupServiceAccountPath != "" {
		t.Fatal("parent sender did not remain separately configured")
	}
}
=======
>>>>>>> origin/main
