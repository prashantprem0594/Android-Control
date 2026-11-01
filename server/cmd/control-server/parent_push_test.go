package main

import (
	"appcontrol/server/internal/config"
	"appcontrol/server/internal/parentpush"
	"appcontrol/server/internal/storage"
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

func TestParentPushRegistryUsesSeparateProtectedKey(t *testing.T) {
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "parent-push.db"))
	if err != nil {
		t.Fatal(err)
	}

	defer db.Close()
	if store, err := configureParentPush(db, config.Config{}); err != nil || store != nil {
		t.Fatal("parent registration should remain disabled without its key")
	}
	path := filepath.Join(t.TempDir(), "parent-routing.key")
	if err := os.WriteFile(path, make([]byte, 31), 0600); err != nil {
		t.Fatal(err)
	}
	settings := config.Config{ParentWakeupTokenKeyPath: path}
	if _, err := configureParentPush(db, settings); err != parentpush.ErrUnavailable {
		t.Fatal("invalid parent routing key accepted")
	}
	if err := os.WriteFile(path, make([]byte, 32), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := configureParentPush(db, settings)
	if err != nil || store == nil {
		t.Fatal("valid parent routing key did not initialize registry")
	}
}

func TestParentFCMSenderConfigurationIsOptInAndUsesSeparateAccount(t *testing.T) {
	if sender, err := configureParentPushSender(config.Config{}); err != nil || sender != nil {
		t.Fatal("disabled parent sender initialized")
	}
	accountPath := filepath.Join(t.TempDir(), "parent-account.json")
	if err := os.WriteFile(accountPath, []byte(`{"private_key":"private-fixture"}`), 0600); err != nil {
		t.Fatal(err)
	}
	settings := config.Config{
		ParentWakeupSenderEnabled:      true,
		ParentWakeupTokenKeyPath:       "parent-key",
		ParentWakeupServiceAccountPath: accountPath,
	}
	if _, err := configureParentPushSender(settings); err != parentpush.ErrSenderUnavailable {
		t.Fatal("malformed parent service account accepted or detailed error exposed")
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]string{
		"type": "service_account", "project_id": "parent-fixture",
		"client_email":   "fixture@parent-fixture.iam.gserviceaccount.com",
		"private_key_id": "fixture-key",
		"private_key":    string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		"token_uri":      "https://oauth2.googleapis.com/token",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(accountPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	sender, err := configureParentPushSender(settings)
	if err != nil || sender == nil {
		t.Fatal("valid parent-specific service account did not configure")
	}
}
