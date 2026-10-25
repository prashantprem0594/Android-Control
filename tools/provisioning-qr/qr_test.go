package main

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

var testChecksum = base64.RawURLEncoding.EncodeToString(make([]byte, 32))

var testPayload = `{
  "android.app.extra.PROVISIONING_DEVICE_ADMIN_COMPONENT_NAME": "com.appcontrol.child/com.appcontrol.child.admin.AppControlDeviceAdminReceiver",
  "android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_DOWNLOAD_LOCATION": "https://downloads.example.test/child.apk",
  "android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_CHECKSUM": "` + testChecksum + `",
  "android.app.extra.PROVISIONING_LEAVE_ALL_SYSTEM_APPS_ENABLED": true,
  "android.app.extra.PROVISIONING_ADMIN_EXTRAS_BUNDLE": {
    "appcontrol.serverOrigin": "https://control.example.test"
  }
}`

func TestRenderAndVerifyRoundTrip(t *testing.T) {
	payload := []byte(testPayload)
	pngBytes, err := renderAndVerify(payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(pngBytes) == 0 || bytes.Equal(pngBytes, payload) {
		t.Fatal("expected a generated PNG image")
	}
}

func TestPNGOutputNeverOverwritesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "setup.png")
	if err := writeNewPNG(path, []byte("original")); err != nil {
		t.Fatal(err)
	}
	if err := writeNewPNG(path, []byte("replacement")); !os.IsExist(err) {
		t.Fatal("existing output was not refused", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "original" {
		t.Fatal("existing output changed", err)
	}
}

func TestValidatePayloadRejectsUnknownFields(t *testing.T) {
	payload := []byte(testPayload[:len(testPayload)-1] + `, "wifiPassword": "not allowed"}`)
	if err := validatePayload(payload); err == nil {
		t.Fatal("expected unknown payload field to be rejected")
	}
}
