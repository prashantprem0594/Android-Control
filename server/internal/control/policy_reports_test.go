package control

import (
	"context"
	"errors"
	"testing"
	"time"

	"appcontrol/server/internal/audit"
)

func TestPolicyReportsDistinguishDesiredAndConfirmedVersions(t *testing.T) {
	db, _ := fixture(t)
	s := NewService(db)
	ctx := context.Background()
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	token := session(t, db, "device", "A", now)
	if _, err := s.UpdatePolicy(ctx, "owner", "device", 0, false, []string{"org.example.first"}); err != nil {
		t.Fatal(err)
	}
	desired, err := s.Policy(ctx, "owner", "device")
	if err != nil || desired.Reported != nil {
		t.Fatal("desired write reported applied before child result")
	}
	if err = s.ReportPolicy(ctx, token, 1, "partial", "package_unavailable"); err != nil {
		t.Fatal(err)
	}
	partial, err := s.Policy(ctx, "owner", "device")
	if err != nil || partial.Reported == nil || partial.Reported.Result != "partial" || partial.Reported.Version != 1 {
		t.Fatal("partial result missing")
	}
	if err = s.ReportPolicy(ctx, token, 1, "applied", ""); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second)
	if err = s.ReportPolicy(ctx, token, 1, "applied", ""); err != nil {
		t.Fatal(err)
	}
	applied, err := s.Policy(ctx, "owner", "device")
	if err != nil || applied.Reported == nil || !applied.Reported.ReportedAt.Equal(now.Add(-time.Second)) {
		t.Fatal("duplicate changed report timestamp")
	}
	if _, err = s.UpdatePolicy(ctx, "owner", "device", 1, false, []string{"org.example.latest"}); err != nil {
		t.Fatal(err)
	}
	pending, err := s.Policy(ctx, "owner", "device")
	if err != nil || pending.Version != 2 || pending.Reported == nil || pending.Reported.Version != 1 {
		t.Fatal("new desired version falsely confirmed")
	}
	if err = s.ReportPolicy(ctx, token, 1, "applied", ""); !errors.Is(err, ErrConflict) {
		t.Fatal("stale result accepted")
	}
	if err = s.ReportPolicy(ctx, token, 2, "applied", ""); err != nil {
		t.Fatal(err)
	}
	events, err := audit.List(ctx, db, "owner", 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, e := range events {
		counts[e.Action]++
		if e.Action == "policy_applied" && (e.ActorType != "device" || e.ActorID != "device") {
			t.Fatal("wrong report audit attribution")
		}
	}
	if counts["policy_partial"] != 1 || counts["policy_applied"] != 2 {
		t.Fatalf("report audit missing or duplicated: %v", counts)
	}
	if _, err = s.Policy(ctx, "other", "device"); !errors.Is(err, ErrNotFound) {
		t.Fatal("report exposed to another parent")
	}
}

func TestPolicyReportAuthorizationValidationAndAuditRollback(t *testing.T) {
	db, _ := fixture(t)
	s := NewService(db)
	ctx := context.Background()
	now := time.Now().UTC()
	token := session(t, db, "device", "A", now)
	if _, err := s.UpdatePolicy(ctx, "owner", "device", 0, false, nil); err != nil {
		t.Fatal(err)
	}
	for _, tc := range [][2]string{{"applied", "package_unavailable"}, {"failed", ""}, {"partial", "private free text"}, {"success", ""}} {
		if err := s.ReportPolicy(ctx, token, 1, tc[0], tc[1]); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid report accepted")
		}
	}
	if err := s.ReportPolicy(ctx, "invalid", 1, "applied", ""); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("unauthenticated report accepted")
	}
	if _, err := db.Exec(`CREATE TRIGGER test_policy_report_failure BEFORE INSERT ON audit_events WHEN NEW.action='policy_applied' BEGIN SELECT RAISE(ABORT,'injected'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.ReportPolicy(ctx, token, 1, "applied", ""); err == nil {
		t.Fatal("unaudited policy report committed")
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM device_policy_reports").Scan(&count); err != nil || count != 0 {
		t.Fatal("audit failure did not roll back report")
	}
	if _, err := db.Exec("DROP TRIGGER test_policy_report_failure"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE devices SET state='revoked',revoked_at='2026-10-23T00:00:00Z',revocation_reason='parent_revoked' WHERE id='device'"); err != nil {
		t.Fatal(err)
	}
	if err := s.ReportPolicy(ctx, token, 1, "applied", ""); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("revoked report accepted")
	}
}
