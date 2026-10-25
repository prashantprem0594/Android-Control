package main

import (
	"appcontrol/server/internal/config"
	"appcontrol/server/internal/storage"
	"appcontrol/server/internal/wakeup"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestOptionalWakeupWiringValidatesPrivateFilesWithoutNetworking(t *testing.T) {
	directory := t.TempDir()
	db, err := storage.Open(context.Background(), filepath.Join(directory, "server.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	provider, store, err := configureWakeup(db, config.Config{})
	if err != nil || provider != nil || store != nil {
		t.Fatal("disabled provider initialized")
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal("fixture key failed")
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal("fixture encoding failed")
	}
	raw, _ := json.Marshal(map[string]string{"type": "service_account", "project_id": "fixture-project", "client_email": "fixture@fixture-project.iam.gserviceaccount.com", "private_key_id": "fixture-key", "private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), "token_uri": "https://oauth2.googleapis.com/token"})
	account := filepath.Join(directory, "account.json")
	routing := filepath.Join(directory, "routing.key")
	if os.WriteFile(account, raw, 0600) != nil || os.WriteFile(routing, make([]byte, 32), 0600) != nil {
		t.Fatal("fixture private files failed")
	}
	settings := config.Config{WakeupServiceAccountPath: account, WakeupTokenKeyPath: routing}
	provider, store, err = configureWakeup(db, settings)
	if err != nil || provider == nil || store == nil {
		t.Fatal("complete provider configuration failed")
	}
	if os.WriteFile(routing, make([]byte, 31), 0600) != nil {
		t.Fatal("fixture invalid key write failed")
	}
	if _, _, err := configureWakeup(db, settings); err != wakeup.ErrUnavailable {
		t.Fatal("invalid routing key accepted or private error exposed")
	}
}
