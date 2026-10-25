package control

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

func TestCommandHistoryPaginationExpiryAndTerminalResults(t *testing.T) {
	db, _ := fixture(t)
	s := NewService(db)
	ctx := context.Background()
	now := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	wanted := map[string]bool{}
	token := session(t, db, "device", "H", now)
	for index := 0; index < 5; index++ {
		command, err := s.QueueLock(ctx, "owner", "device", 60)
		if err != nil {
			t.Fatal(err)
		}
		wanted[command.ID] = true
		if index < 2 {
			if _, err := s.Fetch(ctx, token); err != nil {
				t.Fatal(err)
			}
			result := "succeeded"
			if index == 1 {
				result = "failed"
			}
			if err := s.Acknowledge(ctx, token, command.ID, result); err != nil {
				t.Fatal(err)
			}
		}
		// Include both whole-second and fractional timestamps, and dates closer
		// than SQLite's comparison precision, to exercise cursor ties.
		now = now.Add(100 * time.Nanosecond)
	}
	now = now.Add(2 * time.Minute)
	seen := map[string]bool{}
	states := map[string]int{}
	cursor := ""
	for index := 0; index < 4; index++ {
		page, err := s.Commands(ctx, "owner", "device", cursor, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Commands) > 2 {
			t.Fatal("unbounded history")
		}
		for _, command := range page.Commands {
			if !wanted[command.ID] || seen[command.ID] {
				t.Fatal("history duplicate or foreign command")
			}
			seen[command.ID] = true
			states[command.Status]++
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if len(seen) != 5 || states["succeeded"] != 1 || states["failed"] != 1 || states["expired"] != 3 {
		t.Fatalf("history states %#v", states)
	}
	if _, err := s.Commands(ctx, "other", "device", "", 50); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign history exposed")
	}
	for _, invalid := range []string{"bad", base64.RawURLEncoding.EncodeToString([]byte("bad\nwrong"))} {
		if _, err := s.Commands(ctx, "owner", "device", invalid, 50); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid cursor accepted")
		}
	}
	for _, limit := range []int{0, 101} {
		if _, err := s.Commands(ctx, "owner", "device", "", limit); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid page limit accepted")
		}
	}
}
