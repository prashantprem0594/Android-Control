package main

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServerFailureLogOmitsErrorDetails(t *testing.T) {
	const marker = "SYNTHETIC_DATABASE_PATH_MARKER"
	var output bytes.Buffer
	logServerFailure(slog.New(slog.NewJSONHandler(&output, nil)), errors.New(marker))
	if strings.Contains(output.String(), marker) {
		t.Fatal("server failure log exposed the underlying error")
	}
	if !strings.Contains(output.String(), `"msg":"control server stopped with error"`) ||
		!strings.Contains(output.String(), `"failure":"internal"`) {
		t.Fatalf("fixed failure context missing: %s", output.String())
	}
}

func TestRecoveryHMACKeyLoading(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recovery-key.bin")
	expected := []byte("0123456789abcdef0123456789abcdef")
	if err := os.WriteFile(path, expected, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadRecoveryHMACKey(path)
	if err != nil || !bytes.Equal(loaded, expected) {
		t.Fatalf("valid key was not loaded: %v", err)
	}
	if err := os.WriteFile(path, []byte("short"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadRecoveryHMACKey(path); err == nil {
		t.Fatal("short recovery key accepted")
	}
	if _, err := loadRecoveryHMACKey(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing recovery key accepted")
	}
}
