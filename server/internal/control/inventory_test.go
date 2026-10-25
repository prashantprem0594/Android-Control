package control

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"appcontrol/server/internal/audit"
)

func TestInventoryProtectsTargetsAndIsPrivateAndIdempotent(t *testing.T) {
	db, _ := fixture(t)
	s := NewService(db)
	ctx := context.Background()
	token := session(t, db, "device", "A", time.Now().UTC())
	apps := []InstalledApp{
		{PackageName: "org.example.normal", Label: "Private app label", Blockable: true},
		{PackageName: "com.xiaomi.essential", Label: "OEM system", System: true, Blockable: true},
		{PackageName: "org.example.phone", Label: "Default phone", Blockable: false},
		{PackageName: "com.google.android.gms", Label: "FCM", Blockable: true},
	}
	if err := s.UpdateInventory(ctx, token, apps); err != nil {
		t.Fatal(err)
	}
	inventory, err := s.Inventory(ctx, "owner", "device")
	if err != nil || len(inventory.Apps) != 4 || inventory.UpdatedAt == nil {
		t.Fatal("inventory missing")
	}
	for _, a := range inventory.Apps {
		if a.PackageName != "org.example.normal" && a.Blockable {
			t.Fatal("critical inventory target remained blockable")
		}
	}
	apps[0], apps[3] = apps[3], apps[0]
	if err = s.UpdateInventory(ctx, token, apps); err != nil {
		t.Fatal(err)
	}
	events, err := audit.List(ctx, db, "owner", 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range events {
		if event.Action == "app_inventory_updated" {
			count++
			if strings.Contains(event.TargetID, "Private") || event.Reason != "" {
				t.Fatal("inventory contents in audit")
			}
		}
	}
	if count != 1 {
		t.Fatal("unchanged sorted snapshot duplicated audit")
	}
	if _, err = s.Inventory(ctx, "other", "device"); !errors.Is(err, ErrNotFound) {
		t.Fatal("inventory crossed parent accounts")
	}
	for _, target := range []string{"com.xiaomi.essential", "org.example.phone", "org.example.unknown", "com.google.android.gms"} {
		if _, err = s.UpdatePolicy(ctx, "owner", "device", 0, false, []string{target}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("protected/unknown target accepted: %s", target)
		}
	}
	if _, err = s.UpdatePolicy(ctx, "owner", "device", 0, false, []string{"org.example.normal"}); err != nil {
		t.Fatal("ordinary installed app rejected")
	}
	if _, err = s.UpdatePolicy(ctx, "owner", "device", 1, false, nil); err != nil {
		t.Fatal("empty policy could not restore apps")
	}
}

func TestInventoryValidationAndAuditRollback(t *testing.T) {
	db, _ := fixture(t)
	s := NewService(db)
	ctx := context.Background()
	token := session(t, db, "device", "A", time.Now().UTC())
	app := InstalledApp{PackageName: "org.example.app", Label: "App", Blockable: true}
	for _, apps := range [][]InstalledApp{{{PackageName: "bad package"}}, {app, app}, {{PackageName: "org.example.app", Label: strings.Repeat("a", 513)}}, make([]InstalledApp, 2001)} {
		if err := s.UpdateInventory(ctx, token, apps); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid inventory accepted")
		}
	}
	if err := s.UpdateInventory(ctx, "invalid", []InstalledApp{app}); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("unauthenticated inventory accepted")
	}
	if _, err := db.Exec(`CREATE TRIGGER test_inventory_failure BEFORE INSERT ON audit_events WHEN NEW.action='app_inventory_updated' BEGIN SELECT RAISE(ABORT,'injected'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateInventory(ctx, token, []InstalledApp{app}); err == nil {
		t.Fatal("unaudited inventory committed")
	}
	inventory, err := s.Inventory(ctx, "owner", "device")
	if err != nil || len(inventory.Apps) != 0 || inventory.UpdatedAt != nil {
		t.Fatal("audit failure did not roll back inventory")
	}
	if _, err = db.Exec("DROP TRIGGER test_inventory_failure"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("UPDATE devices SET state='revoked',revoked_at='2026-10-23T00:00:00Z',revocation_reason='parent_revoked' WHERE id='device'"); err != nil {
		t.Fatal(err)
	}
	if err = s.UpdateInventory(ctx, token, []InstalledApp{app}); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("revoked inventory upload accepted")
	}
}
