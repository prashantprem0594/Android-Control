package control

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCallContactsAuthorizationVersionAndAudit(t *testing.T) {
	db, _ := fixture(t)
	service := NewService(db)
	ctx := context.Background()
	token := session(t, db, "device", "K", time.Now().UTC())
	initial, err := service.DeviceCallContacts(ctx, token)
	if err != nil || initial.Version != 0 || initial.Contacts == nil || len(initial.Contacts) != 0 {
		t.Fatal("invalid initial contacts", err)
	}
	contacts := []CallContact{{Label: "Parent", Number: "+919876543210", IsParent: true}}
	saved, err := service.UpdateCallContacts(ctx, "owner", "device", 0, contacts)
	if err != nil || saved.Version != 1 {
		t.Fatal("save failed", err)
	}
	got, err := service.DeviceCallContacts(ctx, token)
	if err != nil || len(got.Contacts) != 1 || got.Contacts[0] != contacts[0] {
		t.Fatal("device snapshot mismatch", err)
	}
	if _, err := service.CallContacts(ctx, "other", "device"); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-parent read accepted", err)
	}
	if _, err := service.UpdateCallContacts(ctx, "other", "device", 1, contacts); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-parent edit accepted", err)
	}
	if _, err := service.UpdateCallContacts(ctx, "owner", "device", 0, []CallContact{}); !errors.Is(err, ErrConflict) {
		t.Fatal("stale write accepted", err)
	}
	if _, err := service.DeviceCallContacts(ctx, "owner"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("parent token used as device token", err)
	}
	var actor, target string
	if err := db.QueryRow("SELECT actor_id,target_id FROM audit_events WHERE action='policy_updated'").Scan(&actor, &target); err != nil || actor != "owner" || target != "device" {
		t.Fatal("incorrect audit attribution", err)
	}
	// A failure to append an immutable audit event must roll back the contact edit.
	if _, err := db.Exec(`CREATE TRIGGER reject_contact_audit BEFORE INSERT ON audit_events WHEN NEW.action='policy_updated' BEGIN SELECT RAISE(ABORT,'audit unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := service.UpdateCallContacts(ctx, "owner", "device", 1, []CallContact{}); err == nil {
		t.Fatal("audit failure ignored")
	}
	got, err = service.CallContacts(ctx, "owner", "device")
	if err != nil || got.Version != 1 || len(got.Contacts) != 1 {
		t.Fatal("failed write did not roll back", err)
	}
	if _, err := db.Exec("DROP TRIGGER reject_contact_audit"); err != nil {
		t.Fatal(err)
	}
	cleared, err := service.UpdateCallContacts(ctx, "owner", "device", 1, []CallContact{})
	if err != nil || cleared.Version != 2 || cleared.Contacts == nil || len(cleared.Contacts) != 0 {
		t.Fatal("cannot clear contacts", err)
	}
	if _, err := db.Exec("UPDATE devices SET state='revoked',revoked_at='2026-10-24T00:00:00Z',revocation_reason='parent_revoked'"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.DeviceCallContacts(ctx, token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("revoked device read accepted", err)
	}
	if _, err := service.CallContacts(ctx, "owner", "device"); !errors.Is(err, ErrNotFound) {
		t.Fatal("revoked parent read accepted", err)
	}
}

func TestCallContactsRejectDialStringsAndAmbiguousContacts(t *testing.T) {
	cases := [][]CallContact{
		nil,
		{{Label: "Parent", Number: "112"}},
		{{Label: "Parent", Number: "+919876543210,1"}},
		{{Label: "Parent", Number: "+919876543210;1"}},
		{{Label: "Parent", Number: "+919876543210#"}},
		{{Label: "Parent", Number: "tel:+919876543210"}},
		{{Label: "Parent", Number: "+019876543210"}},
		{{Label: "Parent\nprivate", Number: "+919876543210"}},
		{{Label: "\u202eParent", Number: "+919876543210"}},
		{{Label: strings.Repeat("x", 41), Number: "+919876543210"}},
		{{Label: " Parent", Number: "+919876543210"}},
		{{Label: "A", Number: "+919876543210"}, {Label: "B", Number: "+919876543210"}},
		{{Label: "A", Number: "+919876543210", IsParent: true}, {Label: "B", Number: "+919876543211", IsParent: true}},
	}
	for index, contacts := range cases {
		if !errors.Is(validateCallContacts(contacts), ErrInvalid) {
			t.Fatalf("invalid case %d accepted", index)
		}
	}
}
