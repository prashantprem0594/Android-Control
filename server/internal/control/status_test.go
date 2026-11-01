package control

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStatusIsServerTimedScopedAndRevocable(t *testing.T) {
	db, _ := fixture(t)
	s := NewService(db)
	ctx := context.Background()
	now := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	initial, err := s.Status(ctx, "owner", "device")
	if err != nil || initial.ReceivedAt != nil || initial.Capabilities != nil {
		t.Fatal("unreported status fabricated capabilities")
	}
	token := session(t, db, "device", "S", now)
	capabilities := Capabilities{AndroidAPI: 37, DeviceOwner: true, ExactAlarms: true, NetworkConnected: true, WiFiConnected: true}
	if err := s.ReportStatus(ctx, token, capabilities); err != nil {
		t.Fatal(err)
	}
	status, err := s.Status(ctx, "owner", "device")
	if err != nil || status.ReceivedAt == nil || !status.ReceivedAt.Equal(now) || status.Capabilities == nil || *status.Capabilities != capabilities {
		t.Fatal("status not recorded with server time")
	}
	if _, err := s.Status(ctx, "other", "device"); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-parent status read")
	}
	now = now.Add(time.Minute)
	capabilities.ExactAlarms = false
	if err := s.ReportStatus(ctx, token, capabilities); err != nil {
		t.Fatal(err)
	}
	updated, err := s.Status(ctx, "owner", "device")
	if err != nil || !updated.ReceivedAt.Equal(now) || updated.Capabilities.ExactAlarms {
		t.Fatal("permission loss or last contact not updated")
	}
	if err := s.ReportStatus(ctx, token, Capabilities{AndroidAPI: 20}); !errors.Is(err, ErrInvalid) {
		t.Fatal("invalid Android API accepted")
	}
	if err := s.ReportStatus(ctx, "invalid", capabilities); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("invalid credential accepted")
	}
	if _, err := db.Exec("UPDATE devices SET state='revoked',revoked_at=?,revocation_reason='parent_revoked' WHERE id='device'", now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if err := s.ReportStatus(ctx, token, capabilities); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("revoked device reported status")
	}
	var received string
	if err := db.QueryRow("SELECT received_at FROM device_status WHERE device_id='device'").Scan(&received); err != nil || received != now.Format(time.RFC3339Nano) {
		t.Fatal("rejected report changed last contact")
	}
}
