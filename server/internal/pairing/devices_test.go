package pairing

import (
	"context"
	"errors"
	"testing"
)

func TestDevicePagesRemainBoundedAndParentScoped(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, public := generateDeviceKey(t)
	wanted := map[string]bool{}
	for i := 0; i < 5; i++ {
		parent := f.parentA.ID
		if i == 4 {
			parent = f.parentB.ID
		}
		code, err := f.service.CreateCode(ctx, parent)
		if err != nil {
			t.Fatal(err)
		}
		enrollment, err := f.service.StartEnrollment(ctx, code.Code, "Fixture", public, KeyAlgorithmECDSAP256SHA256)
		if err != nil {
			t.Fatal(err)
		}
		if parent == f.parentA.ID {
			wanted[enrollment.DeviceID] = true
		}
	}
	seen := map[string]bool{}
	cursor := ""
	for pageNumber := 0; pageNumber < 3; pageNumber++ {
		page, err := f.service.Devices(ctx, f.parentA.ID, cursor, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Devices) != 2 {
			t.Fatalf("page %d has %d devices", pageNumber, len(page.Devices))
		}
		for _, device := range page.Devices {
			if !wanted[device.ID] || seen[device.ID] || device.ParentID != f.parentA.ID || device.ID <= cursor {
				t.Fatal("duplicate, foreign or unordered device")
			}
			seen[device.ID] = true
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if len(seen) != len(wanted) {
		t.Fatal("pagination lost devices")
	}
	for _, limit := range []int{-1, 0, 101} {
		if _, err := f.service.Devices(ctx, f.parentA.ID, "", limit); !errors.Is(err, ErrInvalidPage) {
			t.Fatal("invalid limit accepted")
		}
	}
	if _, err := f.service.Devices(ctx, f.parentA.ID, "invalid", 50); !errors.Is(err, ErrInvalidPage) {
		t.Fatal("invalid cursor accepted")
	}
}
