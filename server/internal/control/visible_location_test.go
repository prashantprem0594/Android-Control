package control

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestVisibleLocationIsOwnerOnlyAndRetentionBounded(t *testing.T) {
	ctx := context.Background()
	db, _ := fixture(t)
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	service := NewService(db)
	service.now = func() time.Time { return now }
	if _, err := db.Exec(`INSERT INTO visible_location_samples(device_id,latitude_e7,longitude_e7,accuracy_meters,captured_at,received_at)
		VALUES('device',123,456,10,?,?)`, now.Add(-time.Minute).Format(time.RFC3339Nano), now.Add(-time.Minute).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	location, err := service.VisibleLocation(ctx, "owner", "device")
	if err != nil || location == nil || location.LatitudeE7 != 123 || location.LongitudeE7 != 456 {
		t.Fatalf("unexpected owner result: %#v, %v", location, err)
	}
	if _, err := service.VisibleLocation(ctx, "other", "device"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-parent location read: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO visible_location_samples(device_id,latitude_e7,longitude_e7,accuracy_meters,captured_at,received_at)
		VALUES('device',1,2,1,?,?)`, now.Add(-13*time.Hour).Format(time.RFC3339Nano), now.Add(-13*time.Hour).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if err := service.PruneVisibleLocations(ctx); err != nil {
		t.Fatal(err)
	}
	var old int
	if err := db.QueryRow(`SELECT count(*) FROM visible_location_samples WHERE latitude_e7=1`).Scan(&old); err != nil || old != 0 {
		t.Fatalf("expired point retained: count=%d err=%v", old, err)
	}
}

func TestVisibleLocationRejectsInvalidAndStalePoints(t *testing.T) {
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	for _, value := range []VisibleLocation{
		{LatitudeE7: 900000001, LongitudeE7: 0, AccuracyMeters: 1, CapturedAt: now},
		{LatitudeE7: 0, LongitudeE7: 0, AccuracyMeters: -1, CapturedAt: now},
		{LatitudeE7: 0, LongitudeE7: 0, AccuracyMeters: 1, CapturedAt: now.Add(-16 * time.Minute)},
		{LatitudeE7: 0, LongitudeE7: 0, AccuracyMeters: 1, CapturedAt: now.Add(6 * time.Minute)},
	} {
		if validVisibleLocation(value, now) {
			t.Fatalf("accepted invalid point: %#v", value)
		}
	}
}
