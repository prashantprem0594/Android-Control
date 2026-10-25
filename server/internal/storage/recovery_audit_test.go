package storage

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestRecoveryAuditMigrationPreservesExistingHistory(t *testing.T) {
	ctx := context.Background()
	db := openTestDatabase(t, ctx, filepath.Join(t.TempDir(), "upgrade.db"))
	defer db.Close()
	prior := fstest.MapFS{}
	entries, err := embeddedMigrations.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() >= "0006" {
			continue
		}
		path := "migrations/" + entry.Name()
		data, err := embeddedMigrations.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		prior[path] = &fstest.MapFile{Data: data}
	}
	if err = migrateFS(ctx, db, prior); err != nil {
		t.Fatal(err)
	}
	insert := `INSERT INTO parent_accounts(id,email,password_hash,created_at,updated_at)
		VALUES(?,?,'unused','2026-10-23T00:00:00Z','2026-10-23T00:00:00Z')`
	if _, err = db.Exec(insert, "first", "first@example.com"); err != nil {
		t.Fatal(err)
	}
	if err = Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(insert, "second", "second@example.com"); err != nil {
		t.Fatal(err)
	}
	var count, maxID int
	if err = db.QueryRow("SELECT COUNT(*),MAX(id) FROM audit_events WHERE action='parent_registered'").Scan(&count, &maxID); err != nil || count != 2 || maxID != 2 {
		t.Fatalf("upgrade lost history, sequence, or producer trigger: %d %d %v", count, maxID, err)
	}
	if _, err = db.Exec("UPDATE audit_events SET actor_id='changed'"); err == nil {
		t.Fatal("upgrade lost immutability")
	}
}

func TestRecoveryAuditPublication(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := Open(ctx, filepath.Join(dir, "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(dir, "source.acb")
	if err = CreateEncryptedBackup(ctx, db, backup, key); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(dir, "restored.db")
	if err = RestoreEncryptedBackup(ctx, backup, destination, key); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(ctx, destination)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var count int
	if err = restored.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE action='database_restored'
		AND actor_type='system' AND parent_account_id='' AND actor_id='' AND target_id='' AND reason=''`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("missing private recovery record: count=%d err=%v", count, err)
	}
	if _, err = restored.Exec("DELETE FROM audit_events WHERE action='database_restored'"); err == nil {
		t.Fatal("recovery event deletion accepted")
	}
	if _, err = db.Exec(`CREATE TRIGGER test_recovery_failure BEFORE INSERT ON audit_events
		WHEN NEW.action='database_restored' BEGIN SELECT RAISE(ABORT,'injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	failedBackup := filepath.Join(dir, "failure.acb")
	if err = CreateEncryptedBackup(ctx, db, failedBackup, key); err != nil {
		t.Fatal(err)
	}
	failedDestination := filepath.Join(dir, "failed.db")
	if err = RestoreEncryptedBackup(ctx, failedBackup, failedDestination, key); err == nil {
		t.Fatal("unaudited recovery succeeded")
	}
	if _, err = os.Stat(failedDestination); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed recovery published destination")
	}
}
