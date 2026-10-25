package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io"
	"net/url"
	"os"
	"unicode/utf8"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
	encoder "github.com/skip2/go-qrcode"
)

const maxPayloadBytes = 4096

type provisioningPayload struct {
	Component       string            `json:"android.app.extra.PROVISIONING_DEVICE_ADMIN_COMPONENT_NAME"`
	DownloadURL     string            `json:"android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_DOWNLOAD_LOCATION"`
	PackageChecksum string            `json:"android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_CHECKSUM"`
	KeepSystemApps  bool              `json:"android.app.extra.PROVISIONING_LEAVE_ALL_SYSTEM_APPS_ENABLED"`
	AdminExtras     map[string]string `json:"android.app.extra.PROVISIONING_ADMIN_EXTRAS_BUNDLE"`
}

func readPayload(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open provisioning payload: %w", err)
	}
	defer file.Close()

	payload, err := io.ReadAll(io.LimitReader(file, maxPayloadBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read provisioning payload: %w", err)
	}
	if len(payload) == 0 || len(payload) > maxPayloadBytes {
		return nil, errors.New("provisioning payload must be between 1 and 4096 bytes")
	}
	if !utf8.Valid(payload) {
		return nil, errors.New("provisioning payload must be UTF-8")
	}
	if err := validatePayload(payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func validatePayload(payload []byte) error {
	var value provisioningPayload
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("invalid provisioning payload: %w", err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("provisioning payload must contain exactly one JSON object")
	}
	if value.Component != "com.appcontrol.child/com.appcontrol.child.admin.AppControlDeviceAdminReceiver" {
		return errors.New("unexpected provisioning admin component")
	}
	if !isHTTPSURL(value.DownloadURL) {
		return errors.New("provisioning download URL must use HTTPS")
	}
	checksum, err := base64.RawURLEncoding.DecodeString(value.PackageChecksum)
	if err != nil || len(checksum) != 32 || base64.RawURLEncoding.EncodeToString(checksum) != value.PackageChecksum {
		return errors.New("invalid provisioning package checksum")
	}
	if !value.KeepSystemApps {
		return errors.New("provisioning payload must retain system applications")
	}
	origin, err := url.Parse(value.AdminExtras["appcontrol.serverOrigin"])
	if len(value.AdminExtras) != 1 || err != nil || origin.Scheme != "https" ||
		origin.Host == "" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" ||
		(origin.Path != "" && origin.Path != "/") {
		return errors.New("invalid provisioning server origin")
	}
	return nil
}

func isHTTPSURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil
}

func renderAndVerify(payload []byte) ([]byte, error) {
	pngBytes, err := encoder.Encode(string(payload), encoder.Medium, 1024)
	if err != nil {
		return nil, fmt.Errorf("encode provisioning QR: %w", err)
	}
	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, fmt.Errorf("decode generated PNG: %w", err)
	}
	bitmap, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return nil, fmt.Errorf("prepare QR for independent decode: %w", err)
	}
	decoded, err := qrcode.NewQRCodeReader().DecodeWithoutHints(bitmap)
	if err != nil {
		return nil, fmt.Errorf("independent QR decoder rejected generated image: %w", err)
	}
	if decoded.GetText() != string(payload) {
		return nil, errors.New("independent QR decoder returned payload different from input")
	}
	return pngBytes, nil
}
