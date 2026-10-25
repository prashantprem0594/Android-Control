package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"appcontrol/server/internal/storage"
)

func TestAdministrationRecoveryRoundTrip(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath, keyPath := filepath.Join(dir, "source.db"), filepath.Join(dir, "key.bin")
	backupPath, restoredPath := filepath.Join(dir, "backup.acb"), filepath.Join(dir, "restored.db")
	db, err := storage.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "CREATE TABLE recovery_fixture (value TEXT); INSERT INTO recovery_fixture VALUES ('private-fixture-data')"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	invoke := func(args ...string) {
		t.Helper()
		if err := run(ctx, args, &output); err != nil {
			t.Fatal(err)
		}
	}
	invoke("keygen", "-key-file", keyPath)
	key, err := os.ReadFile(keyPath)
	if err != nil || len(key) != 32 {
		t.Fatal("invalid generated key")
	}
	if run(ctx, []string{"keygen", "-key-file", keyPath}, &output) == nil {
		t.Fatal("keygen replaced key")
	}
	after, _ := os.ReadFile(keyPath)
	if !bytes.Equal(key, after) {
		t.Fatal("existing key changed")
	}
	invoke("integrity", "-db", dbPath)
	invoke("backup", "-db", dbPath, "-backup", backupPath, "-key-file", keyPath)
	scratch := filepath.Join(dir, "private-scratch")
	invoke("verify", "-backup", backupPath, "-key-file", keyPath, "-scratch-dir", scratch)
	entries, err := os.ReadDir(scratch)
	if err != nil || len(entries) != 0 {
		t.Fatal("verification retained plaintext files")
	}
	wrongKey := filepath.Join(dir, "wrong-key")
	invoke("keygen", "-key-file", wrongKey)
	if run(ctx, []string{"verify", "-backup", backupPath, "-key-file", wrongKey, "-scratch-dir", scratch}, &output) == nil {
		t.Fatal("verification accepted wrong key")
	}
	entries, err = os.ReadDir(scratch)
	if err != nil || len(entries) != 0 {
		t.Fatal("failed verification retained plaintext files")
	}
	if run(ctx, []string{"restore", "-db", restoredPath, "-backup", backupPath, "-key-file", wrongKey}, &output) == nil {
		t.Fatal("wrong key accepted")
	}
	if _, err := os.Stat(restoredPath); !os.IsNotExist(err) {
		t.Fatal("failed restore published database")
	}
	invoke("restore", "-db", restoredPath, "-backup", backupPath, "-key-file", keyPath)
	invoke("integrity", "-db", restoredPath)
	if run(ctx, []string{"restore", "-db", restoredPath, "-backup", backupPath, "-key-file", keyPath}, &output) == nil {
		t.Fatal("restore replaced existing destination")
	}
	db, err = storage.OpenExisting(ctx, restoredPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var value string
	if err := db.QueryRowContext(ctx, "SELECT value FROM recovery_fixture").Scan(&value); err != nil || value != "private-fixture-data" {
		t.Fatal("data not restored")
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM audit_events WHERE action='database_restored'").Scan(&count); err != nil || count != 1 {
		t.Fatal("recovery audit missing or duplicated")
	}
	if bytes.Contains(output.Bytes(), key) || strings.Contains(output.String(), value) || strings.Contains(output.String(), dir) {
		t.Fatal("administration output leaked private data")
	}
}

func TestAdministrationRejectsInvalidInputWithoutCreatingDatabase(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.db")
	for _, args := range [][]string{
		nil, {"unknown"}, {"integrity"}, {"integrity", "-db", missing},
		{"integrity", "-db", missing, "extra"}, {"integrity", "-db", missing, "-timeout", "0s"},
		{"keygen", "-key-file", filepath.Join(dir, "key"), "-db", missing},
		{"backup", "-db", missing}, {"restore", "-db", missing},
		{"verify"}, {"verify", "-backup", "missing", "-key-file", "missing"},
		{"integrity", "-db", missing, "-scratch-dir", dir},
	} {
		if run(context.Background(), args, &bytes.Buffer{}) == nil {
			t.Fatalf("invalid arguments accepted: %v", args)
		}
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("inspection created missing database")
	}
	for _, size := range []int{0, 31, 33, 4096} {
		path := filepath.Join(dir, "bad-key")
		if err := os.WriteFile(path, make([]byte, size), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := readKey(path); err == nil {
			t.Fatal("invalid key length accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if run(ctx, []string{"keygen", "-key-file", filepath.Join(dir, "canceled-key")}, &bytes.Buffer{}) == nil {
		t.Fatal("canceled operation accepted")
	}
}
