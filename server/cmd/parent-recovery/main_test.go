package main

import (
	"appcontrol/server/internal/auth"
	"appcontrol/server/internal/storage"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrivateStdinRecoveryAndNoSecretOutput(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "recovery.db")
	db, err := storage.Open(ctx, path)
	if err != nil {
		t.Fatal("database setup failed")
	}
	s, err := auth.NewService(db, auth.Options{})
	if err != nil {
		t.Fatal("authentication setup failed")
	}
	if _, err := s.Register(ctx, "fixture@example.com", "previous-fixture-password"); err != nil {
		t.Fatal("account setup failed")
	}
	if err := db.Close(); err != nil {
		t.Fatal("database close failed")
	}
	password := "new-fixture-密碼-password"
	input, _ := json.Marshal(map[string]string{"email": "fixture@example.com", "password": password})
	var out bytes.Buffer
	if err := run(ctx, []string{"-db", path}, bytes.NewReader(input), &out); err != nil {
		t.Fatal("private stdin recovery failed")
	}
	if !strings.Contains(out.String(), "Parent password recovered.") || strings.Contains(out.String(), password) || strings.Contains(out.String(), "fixture@example.com") {
		t.Fatal("unexpected or private administration output")
	}
	db, err = storage.OpenExisting(ctx, path)
	if err != nil {
		t.Fatal("database reopen failed")
	}
	defer db.Close()
	s, err = auth.NewService(db, auth.Options{})
	if err != nil {
		t.Fatal("authentication reopen failed")
	}
	if _, _, err := s.Login(ctx, "fixture@example.com", password); err != nil {
		t.Fatal("UTF-8 recovery password not retained")
	}
}

func TestRecoveryRejectsBadInputAndDoesNotCreateDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "must-not-exist.db")
	for _, input := range []string{"", `null`, `{}`, `{"email":"fixture@example.com","password":"fixture-password","extra":true}`, `{"email":"fixture@example.com","password":"fixture-password"}{}`, strings.Repeat("x", 8193), `{"email":"fixture@example.com","password":"fixture-password"}`} {
		var out bytes.Buffer
		if err := run(context.Background(), []string{"-db", path}, strings.NewReader(input), &out); err == nil {
			t.Fatal("invalid or missing-database input accepted")
		}
		if out.Len() != 0 {
			t.Fatal("failure printed private input")
		}
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("recovery created missing database")
	}
}
