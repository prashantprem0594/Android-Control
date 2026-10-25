package storage

import (
	"context"
	"io/fs"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestPasswordRecoveryMigrationPreservesExistingInventoryAudit(t *testing.T) {
	ctx := context.Background()
	db, err := openRaw(ctx, filepath.Join(t.TempDir(), "upgrade.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	old := fstest.MapFS{}
	paths, err := fs.Glob(embeddedMigrations, "migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if filepath.Base(path) >= "0014_parent_password_recovery.sql" {
			continue
		}
		data, err := embeddedMigrations.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		old[path] = &fstest.MapFile{Data: data}
	}
	if err := migrateFS(ctx, db, old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO audit_events(occurred_at,parent_account_id,actor_type,actor_id,action,target_id) VALUES('2026-10-24T00:00:00Z','fixture','device','fixture','app_inventory_updated','fixture')`); err != nil {
		t.Fatal(err)
	}
	var beforeID int
	var beforeTime string
	if err := db.QueryRow("SELECT id,occurred_at FROM audit_events WHERE action='app_inventory_updated'").Scan(&beforeID, &beforeTime); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	var afterTime string
	if err := db.QueryRow("SELECT occurred_at FROM audit_events WHERE id=? AND action='app_inventory_updated'", beforeID).Scan(&afterTime); err != nil || afterTime != beforeTime {
		t.Fatal("upgrade changed existing audit event")
	}
	if _, err := db.Exec(`INSERT INTO audit_events(occurred_at,parent_account_id,actor_type,actor_id,action,target_id) VALUES('2026-10-24T01:00:00Z','fixture','device','fixture','app_inventory_updated','fixture')`); err != nil {
		t.Fatal("upgrade removed existing action")
	}
	for _, query := range []string{"DELETE FROM audit_events", "UPDATE audit_events SET target_id='changed'"} {
		if _, err := db.Exec(query); err == nil {
			t.Fatal("upgrade lost append-only audit protection")
		}
	}
	if err := IntegrityCheck(ctx, db); err != nil {
		t.Fatal(err)
	}
}
