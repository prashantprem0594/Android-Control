package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"appcontrol/server/internal/storage"
)

const adminProcessMarker = "APP_CONTROL_TEST_ADMIN_PROCESS"

func TestAdministrationProcess(t *testing.T) {
	if os.Getenv(adminProcessMarker) != "1" {
		return
	}
	for index, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{os.Args[0]}, os.Args[index+1:]...)
			main() // Exercise production parsing, signals, output and error exit code.
			os.Exit(0)
		}
	}
	os.Exit(2)
}

func TestAdministrationRecoveryAcrossProcesses(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "source.db")
	db, err := storage.Open(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.ExecContext(ctx, "CREATE TABLE process_fixture(value TEXT); INSERT INTO process_fixture VALUES('private-process-fixture')"); err != nil {
		t.Fatal(err)
	}
	var captured bytes.Buffer
	invoke := func(wantSuccess bool, args ...string) {
		t.Helper()
		command := exec.CommandContext(ctx, executable, append([]string{"-test.run=^TestAdministrationProcess$", "--"}, args...)...)
		command.Env = append(os.Environ(), adminProcessMarker+"=1")
		output, err := command.CombinedOutput()
		captured.Write(output)
		if (err == nil) != wantSuccess {
			t.Fatalf("administration process %s returned unexpected success/failure", args[0])
		}
		if ctx.Err() != nil {
			t.Fatal("administration process deadline exceeded")
		}
	}
	keyPath := filepath.Join(dir, "backup-key.bin")
	backup := filepath.Join(dir, "snapshot.acb")
	scratch := filepath.Join(dir, "scratch")
	restoredPath := filepath.Join(t.TempDir(), "restored.db")
	invoke(true, "keygen", "-key-file", keyPath)
	keyBefore, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	// The separate CLI must snapshot committed data while the source is open
	// in WAL mode, not depend on this test checkpointing or closing it first.
	invoke(true, "backup", "-db", source, "-backup", backup, "-key-file", keyPath)
	backupBefore, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	invoke(true, "verify", "-backup", backup, "-key-file", keyPath, "-scratch-dir", scratch)
	entries, err := os.ReadDir(scratch)
	if err != nil || len(entries) != 0 {
		t.Fatal("verification retained scratch data")
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(source); err != nil {
		t.Fatal(err)
	}
	// Source database is unavailable: restore can use only backup and key.
	wrongKey := filepath.Join(dir, "wrong-key.bin")
	invoke(true, "keygen", "-key-file", wrongKey)
	invoke(false, "restore", "-db", restoredPath, "-backup", backup, "-key-file", wrongKey)
	if _, err = os.Stat(restoredPath); !os.IsNotExist(err) {
		t.Fatal("failed CLI restore published a database")
	}
	invoke(true, "restore", "-db", restoredPath, "-backup", backup, "-key-file", keyPath)
	invoke(true, "integrity", "-db", restoredPath)
	invoke(false, "restore", "-db", restoredPath, "-backup", backup, "-key-file", keyPath)
	restored, err := storage.OpenExisting(ctx, restoredPath)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var value string
	if err = restored.QueryRowContext(ctx, "SELECT value FROM process_fixture").Scan(&value); err != nil || value != "private-process-fixture" {
		t.Fatal("cross-process recovery lost committed data")
	}
	var audits int
	if err = restored.QueryRowContext(ctx, "SELECT count(*) FROM audit_events WHERE action='database_restored'").Scan(&audits); err != nil || audits != 1 {
		t.Fatal("CLI recovery audit missing or duplicated")
	}
	keyAfter, err := os.ReadFile(keyPath)
	if err != nil || !bytes.Equal(keyBefore, keyAfter) {
		t.Fatal("CLI changed the backup key")
	}
	backupAfter, err := os.ReadFile(backup)
	if err != nil || !bytes.Equal(backupBefore, backupAfter) {
		t.Fatal("CLI changed the source backup")
	}
	if bytes.Contains(captured.Bytes(), keyBefore) || bytes.Contains(captured.Bytes(), []byte(value)) || bytes.Contains(captured.Bytes(), []byte(dir)) {
		t.Fatal("CLI process output exposed private fixture data")
	}
}
