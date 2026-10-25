package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"appcontrol/server/internal/auth"
	"appcontrol/server/internal/control"
	"appcontrol/server/internal/storage"
)

func TestQueuedCommandSurvivesAbruptServerTermination(t *testing.T) {
	const marker = "APP_CONTROL_DURABILITY_TEST_DB"
	fake := completeFakeAuth()
	fake.authenticate = func(context.Context, string) (auth.Parent, string, error) { return auth.Parent{ID: "owner"}, "", nil }
	if path := os.Getenv(marker); path != "" {
		db, err := storage.Open(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		for _, query := range []string{
			`INSERT INTO parent_accounts(id,email,password_hash,created_at,updated_at) VALUES('owner','owner@example.com','unused','2026-10-23T00:00:00Z','2026-10-23T00:00:00Z')`,
			`INSERT INTO devices(id,parent_account_id,display_name,public_key_der,key_algorithm,state,created_at) VALUES('device','owner','test',X'00','ECDSA_P256_SHA256','active','2026-10-23T00:00:00Z')`,
		} {
			if _, err = db.Exec(query); err != nil {
				t.Fatal(err)
			}
		}
		server := httptest.NewServer(NewHandler(Options{Auth: fake, Control: control.NewService(db)}))
		fmt.Fprintln(os.Stdout, server.URL)
		select {} // Parent kills the process; no graceful shutdown or DB Close.
	}
	path := filepath.Join(t.TempDir(), "crash.db")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	process := exec.CommandContext(ctx, executable, "-test.run=^TestQueuedCommandSurvivesAbruptServerTermination$")
	process.Env = append(os.Environ(), marker+"="+path)
	stdout, err := process.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	process.Stderr = &stderr
	if err = process.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if process.ProcessState == nil {
			_ = process.Process.Kill()
			_ = process.Wait()
		}
	}()
	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			ready <- scanner.Text()
		} else {
			ready <- ""
		}
	}()
	var address string
	select {
	case address = <-ready:
	case <-ctx.Done():
		t.Fatal("subprocess server startup timeout")
	}
	if !strings.HasPrefix(address, "http://127.0.0.1:") {
		t.Fatalf("unexpected subprocess startup: %q", address)
	}
	request, err := http.NewRequest("POST", address+"/v1/devices/device/commands", strings.NewReader(`{"type":"LOCK_DEVICE","ttlSeconds":60}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer test-owner")
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var command control.Command
	err = json.NewDecoder(response.Body).Decode(&command)
	response.Body.Close()
	if err != nil || response.StatusCode != 201 || command.Status != "queued" {
		t.Fatal("command creation failed before crash")
	}
	if err = process.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = process.Wait()
	db, err := storage.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	restarted := httptest.NewServer(NewHandler(Options{Auth: fake, Control: control.NewService(db)}))
	defer restarted.Close()
	request, err = http.NewRequest("GET", restarted.URL+"/v1/devices/device/commands/"+command.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer test-owner")
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var recovered control.Command
	err = json.NewDecoder(response.Body).Decode(&recovered)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || recovered.ID != command.ID || recovered.Status != "queued" {
		t.Fatal("command lost across abrupt termination and HTTP restart")
	}
	var audits int
	if err = db.QueryRow("SELECT COUNT(*) FROM audit_events WHERE target_id=? AND action='command_created'", command.ID).Scan(&audits); err != nil || audits != 1 {
		t.Fatal("audit lost across crash")
	}
}
