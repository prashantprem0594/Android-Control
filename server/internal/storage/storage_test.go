package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	_ "modernc.org/sqlite"
)

func TestMigrationRollbackAndIdempotence(t *testing.T) {
	ctx := context.Background()
	database := openTestDatabase(t, ctx, filepath.Join(t.TempDir(), "migration.db"))
	defer database.Close()
	failing := fstest.MapFS{
		"migrations/0001_stable.sql": {Data: []byte("CREATE TABLE stable(value TEXT NOT NULL) STRICT;")},
		"migrations/0002_fails.sql":  {Data: []byte("CREATE TABLE should_rollback(value TEXT) STRICT; INSERT INTO missing_table(value) VALUES ('x');")},
	}
	if err := migrateFS(ctx, database, failing); err == nil {
		t.Fatal("expected the second migration to fail")
	}
	assertTableExists(t, database, "stable", true)
	assertTableExists(t, database, "should_rollback", false)
	var applied int
	if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&applied); err != nil || applied != 1 {
		t.Fatalf("expected one applied migration, count=%d error=%v", applied, err)
	}
	successful := fstest.MapFS{
		"migrations/0001_stable.sql": {Data: []byte("CREATE TABLE stable(value TEXT NOT NULL) STRICT;")},
	}
	if err := migrateFS(ctx, database, successful); err != nil {
		t.Fatalf("repeat applied migration: %v", err)
	}
}

func TestWithinTransactionCommitsOrRollsBack(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, filepath.Join(t.TempDir(), "transaction.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()
	expected := errors.New("reject transaction")
	err = WithinTransaction(ctx, database, func(transaction *sql.Tx) error {
		_, insertErr := transaction.ExecContext(ctx, "INSERT INTO app_metadata(key, value, updated_at) VALUES (?, ?, ?)", "rolled-back", []byte("value"), time.Now().UTC().Format(time.RFC3339Nano))
		if insertErr != nil {
			return insertErr
		}
		return expected
	})
	if !errors.Is(err, expected) {
		t.Fatalf("expected transaction error, got %v", err)
	}
	assertMetadataCount(t, database, 0)
	if err := WithinTransaction(ctx, database, func(transaction *sql.Tx) error {
		_, insertErr := transaction.ExecContext(ctx, "INSERT INTO app_metadata(key, value, updated_at) VALUES (?, ?, ?)", "committed", []byte("value"), time.Now().UTC().Format(time.RFC3339Nano))
		return insertErr
	}); err != nil {
		t.Fatalf("commit transaction: %v", err)
	}
	assertMetadataCount(t, database, 1)
}

func TestEncryptedBackupRestoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	database, err := Open(ctx, filepath.Join(directory, "source.db"))
	if err != nil {
		t.Fatalf("open source database: %v", err)
	}
	secretValue := []byte("family-backup-secret")
	if _, err := database.ExecContext(ctx, "INSERT INTO app_metadata(key, value, updated_at) VALUES (?, ?, ?)", "test", secretValue, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		database.Close()
		t.Fatalf("insert backup value: %v", err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		database.Close()
		t.Fatalf("generate test key: %v", err)
	}
	backupPath := filepath.Join(directory, "backup.acb")
	if err := CreateEncryptedBackup(ctx, database, backupPath, key); err != nil {
		database.Close()
		t.Fatalf("create backup: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close source database: %v", err)
	}
	encrypted, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read encrypted backup: %v", err)
	}
	if bytes.Contains(encrypted, secretValue) || bytes.HasPrefix(encrypted, []byte("SQLite format 3")) {
		t.Fatal("backup contains plaintext database content")
	}

	wrongKey := make([]byte, 32)
	if _, err := rand.Read(wrongKey); err != nil {
		t.Fatalf("generate wrong key: %v", err)
	}
	wrongDestination := filepath.Join(directory, "wrong.db")
	if err := RestoreEncryptedBackup(ctx, backupPath, wrongDestination, wrongKey); err == nil {
		t.Fatal("expected authentication failure with wrong backup key")
	}
	if _, err := os.Stat(wrongDestination); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("failed restore published a database: %v", err)
	}

	restoredPath := filepath.Join(directory, "restored.db")
	if err := RestoreEncryptedBackup(ctx, backupPath, restoredPath, key); err != nil {
		t.Fatalf("restore backup: %v", err)
	}
	restored, err := Open(ctx, restoredPath)
	if err != nil {
		t.Fatalf("open restored database: %v", err)
	}
	defer restored.Close()
	var restoredValue []byte
	if err := restored.QueryRowContext(ctx, "SELECT value FROM app_metadata WHERE key = ?", "test").Scan(&restoredValue); err != nil {
		t.Fatalf("read restored value: %v", err)
	}
	if !bytes.Equal(restoredValue, secretValue) {
		t.Fatalf("restored value mismatch: %q", restoredValue)
	}
	if err := IntegrityCheck(ctx, restored); err != nil {
		t.Fatalf("restored integrity check: %v", err)
	}
}

func openTestDatabase(t *testing.T, ctx context.Context, path string) *sql.DB {
	t.Helper()
	database, err := openRaw(ctx, path)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	return database
}

func assertTableExists(t *testing.T, database *sql.DB, name string, expected bool) {
	t.Helper()
	var count int
	if err := database.QueryRow("SELECT COUNT(*) FROM sqlite_schema WHERE type = 'table' AND name = ?", name).Scan(&count); err != nil {
		t.Fatalf("check table %s: %v", name, err)
	}
	if (count == 1) != expected {
		t.Fatalf("table %s existence=%t, expected %t", name, count == 1, expected)
	}
}

func assertMetadataCount(t *testing.T, database *sql.DB, expected int) {
	t.Helper()
	var count int
	if err := database.QueryRow("SELECT COUNT(*) FROM app_metadata").Scan(&count); err != nil {
		t.Fatalf("count metadata: %v", err)
	}
	if count != expected {
		t.Fatalf("metadata count=%d, expected %d", count, expected)
	}
}
