package httpapi

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"

	"appcontrol/server/internal/auth"
	"appcontrol/server/internal/control"
	"appcontrol/server/internal/pairing"
	"appcontrol/server/internal/storage"
)

const recoveryProcessDatabase = "APP_CONTROL_TEST_RECOVERY_PROCESS_DATABASE"

type recoveryProcessAddress struct {
	URL         string
	Certificate []byte
}

func TestRecoveryServerProcess(t *testing.T) {
	path := os.Getenv(recoveryProcessDatabase)
	if path == "" {
		return
	}
	db, err := storage.OpenExisting(context.Background(), path)
	if err != nil {
		t.Fatal("recovery process cannot open database")
	}
	defer db.Close()
	authentication, err := auth.NewService(db, auth.Options{})
	if err != nil {
		t.Fatal("recovery process authentication setup failed")
	}
	devices, err := pairing.NewService(db, pairing.Options{})
	if err != nil {
		t.Fatal("recovery process device setup failed")
	}
	server := httptest.NewTLSServer(NewHandler(Options{
		Auth: authentication, Pairing: devices, Control: control.NewService(db), Ready: db.PingContext,
	}))
	defer server.Close()
	// Only public connection metadata crosses stdout. No fixture credentials,
	// private keys, request bodies or database rows are printed.
	address, err := json.Marshal(recoveryProcessAddress{server.URL, server.Certificate().Raw})
	if err != nil {
		t.Fatal("recovery process metadata failed")
	}
	fmt.Fprintln(os.Stdout, string(address))
	select {} // Parent terminates and reaps this isolated test server.
}

func startRecoveryProcess(t *testing.T, database string) (string, *http.Client) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	process := exec.CommandContext(ctx, executable, "-test.run=^TestRecoveryServerProcess$")
	process.Env = append(os.Environ(), recoveryProcessDatabase+"="+database)
	output, err := process.StdoutPipe()
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if err = process.Start(); err != nil {
		cancel()
		t.Fatal("recovery process start failed")
	}
	t.Cleanup(func() {
		_ = process.Process.Kill()
		_ = process.Wait()
		cancel()
	})
	ready := make(chan recoveryProcessAddress, 1)
	go func() {
		scanner := bufio.NewScanner(output)
		var address recoveryProcessAddress
		if scanner.Scan() {
			_ = json.Unmarshal(scanner.Bytes(), &address)
		}
		ready <- address
	}()
	var address recoveryProcessAddress
	select {
	case address = <-ready:
	case <-ctx.Done():
		t.Fatal("recovery process startup timed out")
	}
	certificate, err := x509.ParseCertificate(address.Certificate)
	if err != nil {
		t.Fatal("recovery process certificate missing")
	}
	roots := x509.NewCertPool()
	roots.AddCert(certificate)
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
	t.Cleanup(transport.CloseIdleConnections)
	return address.URL, &http.Client{Transport: transport, Timeout: 10 * time.Second}
}
