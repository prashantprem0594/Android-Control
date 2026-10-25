package control

import (
	"bufio"
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"appcontrol/server/internal/storage"
)

func fixtureNames(value string) []string {
	if value == "-" {
		return []string{}
	}
	return strings.Split(value, ",")
}
func TestScheduleSharedVectors(t *testing.T) {
	file, err := os.Open("../../../contracts/schedule-vectors.tsv")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 14 {
			t.Fatal("invalid schedule vector")
		}
		t.Run(f[0], func(t *testing.T) {
			number := func(value string) int {
				n, err := strconv.Atoi(value)
				if err != nil {
					t.Fatal(err)
				}
				return n
			}
			days := []int{}
			for _, day := range strings.Split(f[3], ",") {
				days = append(days, number(day))
			}
			instant, err := time.Parse(time.RFC3339, f[1])
			if err != nil {
				t.Fatal(err)
			}
			p := Policy{Version: 7, InternetPaused: f[6] == "true", BlockedPackages: fixtureNames(f[8]), AlwaysAllowedPackages: fixtureNames(f[10]), Schedules: []Schedule{{ID: "window", TimeZone: f[2], DaysOfWeek: days, StartMinute: number(f[4]), EndMinute: number(f[5]), InternetPaused: f[7] == "true", BlockedPackages: fixtureNames(f[9])}}}
			effective, err := ResolvePolicy(p, instant)
			if err != nil {
				t.Fatal(err)
			}
			if effective.InternetPaused != (f[11] == "true") || !reflect.DeepEqual(effective.BlockedPackages, fixtureNames(f[12])) || effective.StateKey != f[13] {
				t.Fatalf("effective state differs from shared expectation: %+v", effective)
			}
		})
		count++
	}
	if err = scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if count != 12 {
		t.Fatal("missing shared vectors")
	}
}

func TestScheduledPolicyFormatAndBoundaryReports(t *testing.T) {
	db, _ := fixture(t)
	s := NewService(db)
	ctx := context.Background()
	now := time.Date(2026, 10, 23, 21, 59, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	token := session(t, db, "device", "A", now)
	rule := Schedule{ID: "bedtime", TimeZone: "UTC", DaysOfWeek: []int{5}, StartMinute: 1320, EndMinute: 420, InternetPaused: true, BlockedPackages: []string{"org.example.game", "org.example.safe"}}
	p, err := s.UpdatePolicyRules(ctx, "owner", "device", 0, false, nil, []string{"org.example.safe"}, []Schedule{rule})
	if err != nil || p.FormatVersion != 2 {
		t.Fatal("scheduled policy failed")
	}
	if _, err = s.DevicePolicy(ctx, token); !errors.Is(err, ErrPolicyFormat) {
		t.Fatal("legacy child fetched unsupported schedules")
	}
	current, err := s.DevicePolicyWithFormat(ctx, token, 2)
	if err != nil || len(current.Schedules) != 1 || len(current.AlwaysAllowedPackages) != 1 {
		t.Fatal("new child did not receive rules")
	}
	before, err := ResolvePolicy(current, now)
	if err != nil || before.InternetPaused {
		t.Fatal("schedule active before start")
	}
	if err = s.ReportPolicy(ctx, token, 1, "applied", ""); !errors.Is(err, ErrPolicyFormat) {
		t.Fatal("legacy report confirmed an extended policy")
	}
	if err = s.ReportPolicyState(ctx, token, 1, "applied", "", before.StateKey); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	parent, err := s.Policy(ctx, "owner", "device")
	if err != nil || parent.Reported == nil || parent.Reported.StateKey == parent.EffectiveStateKey {
		t.Fatal("old boundary state falsely confirmed current schedule")
	}
	if err = s.ReportPolicyState(ctx, token, 1, "applied", "", before.StateKey); !errors.Is(err, ErrConflict) {
		t.Fatal("stale boundary result accepted")
	}
	active, err := ResolvePolicy(parent, now)
	if err != nil || !active.InternetPaused || !reflect.DeepEqual(active.BlockedPackages, []string{"org.example.game"}) {
		t.Fatal("schedule/allow override failed")
	}
	if err = s.ReportPolicyState(ctx, token, 1, "applied", "", active.StateKey); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow("SELECT COUNT(*) FROM audit_events WHERE action='policy_applied'").Scan(&count); err != nil || count != 2 {
		t.Fatal("same-version boundary result not audited")
	}
	if err = s.ReportPolicyState(ctx, token, 1, "applied", "", active.StateKey); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow("SELECT COUNT(*) FROM audit_events WHERE action='policy_applied'").Scan(&count); err != nil || count != 2 {
		t.Fatal("duplicate boundary report created audit")
	}
	if _, err = s.UpdatePolicyRules(ctx, "other", "device", 1, false, nil, nil, nil); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-parent schedule write accepted")
	}
}

func TestScheduleValidationAndRollback(t *testing.T) {
	valid := func() Schedule {
		return Schedule{ID: "school", TimeZone: "Asia/Kolkata", DaysOfWeek: []int{1, 2, 3, 4, 5}, StartMinute: 480, EndMinute: 900, BlockedPackages: []string{"org.example.game"}}
	}
	for _, mutate := range []func(*Schedule){func(r *Schedule) { r.TimeZone = "Not/AZone" }, func(r *Schedule) { r.DaysOfWeek = []int{0} }, func(r *Schedule) { r.DaysOfWeek = []int{1, 1} }, func(r *Schedule) { r.StartMinute = r.EndMinute }, func(r *Schedule) { r.EndMinute = 1441 }, func(r *Schedule) { r.ID = "bad name" }, func(r *Schedule) { r.BlockedPackages = []string{"com.android.phone"} }} {
		rule := valid()
		mutate(&rule)
		if _, err := normalizeSchedules([]Schedule{rule}); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid rule accepted")
		}
	}
	if _, err := normalizeSchedules([]Schedule{valid(), valid()}); !errors.Is(err, ErrInvalid) {
		t.Fatal("duplicate schedule id accepted")
	}
	db, _ := fixture(t)
	s := NewService(db)
	ctx := context.Background()
	if _, err := db.Exec(`CREATE TRIGGER test_schedule_failure BEFORE INSERT ON audit_events WHEN NEW.action='policy_updated' BEGIN SELECT RAISE(ABORT,'injected'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdatePolicyRules(ctx, "owner", "device", 0, false, nil, []string{"org.example.safe"}, []Schedule{valid()}); err == nil {
		t.Fatal("unaudited schedules committed")
	}
	p, err := s.Policy(ctx, "owner", "device")
	if err != nil || p.Version != 0 || len(p.Schedules) != 0 || len(p.AlwaysAllowedPackages) != 0 {
		t.Fatal("schedule audit failure did not roll back")
	}
}

func TestScheduledPolicyAndReportsSurviveEncryptedRestore(t *testing.T) {
	db, _ := fixture(t)
	s := NewService(db)
	ctx := context.Background()
	now := time.Date(2026, 10, 23, 22, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	token := session(t, db, "device", "A", now)
	rules := []Schedule{{ID: "bedtime", TimeZone: "UTC", DaysOfWeek: []int{5}, StartMinute: 1320, EndMinute: 420, InternetPaused: true, BlockedPackages: []string{"org.example.game"}}}
	policy, err := s.UpdatePolicyRules(ctx, "owner", "device", 0, false, nil, []string{"org.example.safe"}, rules)
	if err != nil {
		t.Fatal(err)
	}
	effective, err := ResolvePolicy(policy, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ReportPolicyState(ctx, token, 1, "applied", "", effective.StateKey); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	backup := filepath.Join(dir, "schedules.acb")
	restoredPath := filepath.Join(dir, "restored.db")
	if err = storage.CreateEncryptedBackup(ctx, db, backup, key); err != nil {
		t.Fatal(err)
	}
	if err = storage.RestoreEncryptedBackup(ctx, backup, restoredPath, key); err != nil {
		t.Fatal(err)
	}
	restored, err := storage.Open(ctx, restoredPath)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	recovered := NewService(restored)
	recovered.now = s.now
	snapshot, err := recovered.Policy(ctx, "owner", "device")
	if err != nil || snapshot.FormatVersion != 2 || !reflect.DeepEqual(snapshot.Schedules, policy.Schedules) || !reflect.DeepEqual(snapshot.AlwaysAllowedPackages, policy.AlwaysAllowedPackages) || snapshot.Reported == nil || snapshot.Reported.StateKey != effective.StateKey {
		t.Fatal("schedule configuration or result lost during restore")
	}
	if _, err = recovered.UpdatePolicyRules(ctx, "owner", "device", 1, false, nil, nil, rules); err != nil {
		t.Fatal(err)
	}
	snapshot, err = recovered.Policy(ctx, "owner", "device")
	if err != nil {
		t.Fatal(err)
	}
	if err = recovered.ReportPolicyState(ctx, token, 2, "applied", "", snapshot.EffectiveStateKey); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = restored.QueryRow("SELECT COUNT(*) FROM audit_events WHERE action='policy_applied'").Scan(&count); err != nil || count != 2 {
		t.Fatal("state-report audit trigger lost after restore")
	}
}
