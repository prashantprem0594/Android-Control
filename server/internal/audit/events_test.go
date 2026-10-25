package audit_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"appcontrol/server/internal/audit"
	"appcontrol/server/internal/auth"
	"appcontrol/server/internal/control"
	"appcontrol/server/internal/pairing"
	"appcontrol/server/internal/storage"
)

func TestSecurityFlowsProducePrivateAppendOnlyHistory(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "audit.db")
	db, err := storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	now := time.Now().UTC()
	authentication, err := auth.NewService(db, auth.Options{Now: func() time.Time { return now }, PasswordParameters: auth.PasswordParameters{MemoryKiB: 8192, Iterations: 1, Parallelism: 1, SaltBytes: 16, KeyBytes: 32}})
	if err != nil {
		t.Fatal(err)
	}
	const password = "private-test-password"
	parent, err := authentication.Register(ctx, "private@example.com", password)
	if err != nil {
		t.Fatal(err)
	}
	other, err := authentication.Register(ctx, "other@example.com", password)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = authentication.Login(ctx, parent.Email, "wrong-password"); err == nil {
		t.Fatal("bad password accepted")
	}
	_, tokens, err := authentication.Login(ctx, parent.Email, password)
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := authentication.Refresh(ctx, tokens.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = authentication.Refresh(ctx, tokens.RefreshToken); err == nil {
		t.Fatal("refresh reuse accepted")
	}
	_, logoutTokens, err := authentication.Login(ctx, parent.Email, password)
	if err != nil {
		t.Fatal(err)
	}
	if err = authentication.Logout(ctx, logoutTokens.AccessToken); err != nil {
		t.Fatal(err)
	}
	service, err := pairing.NewService(db, pairing.Options{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	encode := base64.RawURLEncoding.EncodeToString
	code, err := service.CreateCode(ctx, parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := service.StartEnrollment(ctx, code.Code, "Sensitive device name", encode(der), pairing.KeyAlgorithmECDSAP256SHA256)
	if err != nil {
		t.Fatal(err)
	}
	challenge, _ := base64.RawURLEncoding.DecodeString(enrollment.Challenge)
	digest := sha256.Sum256(append([]byte("app-control-device-enrollment-v1\n"+enrollment.ID+"\n"), challenge...))
	signature, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	wrongKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	wrongSignature, err := ecdsa.SignASN1(rand.Reader, wrongKey, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.CompleteEnrollment(ctx, enrollment.ID, enrollment.Challenge, encode(wrongSignature)); err == nil {
		t.Fatal("wrong enrollment proof accepted")
	}
	device, err := service.CompleteEnrollment(ctx, enrollment.ID, enrollment.Challenge, encode(signature))
	if err != nil {
		t.Fatal(err)
	}
	ac, err := service.CreateAuthChallenge(ctx, device.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.CompleteAuthentication(ctx, device.ID, ac.ID, ac.Challenge, encode(signature)); err == nil {
		t.Fatal("wrong authentication proof accepted")
	}
	ac, err = service.CreateAuthChallenge(ctx, device.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := base64.RawURLEncoding.DecodeString(ac.Challenge)
	digest = sha256.Sum256(append([]byte("app-control-device-auth-v1\n"+device.ID+"\n"+ac.ID+"\n"), raw...))
	signature, err = ecdsa.SignASN1(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	session, err := service.CompleteAuthentication(ctx, device.ID, ac.ID, ac.Challenge, encode(signature))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `CREATE TRIGGER test_revoke_audit_failure BEFORE INSERT ON audit_events
        WHEN NEW.action='device_revoked' BEGIN SELECT RAISE(ABORT,'injected audit failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	if err = service.RevokeDevice(ctx, parent.ID, device.ID); err == nil {
		t.Fatal("unaudited revocation succeeded")
	}
	if _, err = service.AuthenticateDevice(ctx, session.AccessToken); err != nil {
		t.Fatal("failed revocation did not roll back")
	}
	if _, err = db.Exec("DROP TRIGGER test_revoke_audit_failure"); err != nil {
		t.Fatal(err)
	}
	controls := control.NewService(db)
	if err = controls.UpdateInventory(ctx, session.AccessToken, []control.InstalledApp{{PackageName: "org.example.auditprivate", Label: "Private inventory label", Blockable: true}}); err != nil {
		t.Fatal(err)
	}
	if _, err = controls.QueueLock(ctx, parent.ID, device.ID, 60); err != nil {
		t.Fatal(err)
	}
	if _, err = controls.UpdatePolicy(ctx, parent.ID, device.ID, 0, true, []string{"org.example.auditprivate"}); err != nil {
		t.Fatal(err)
	}
	if err = controls.ReportPolicy(ctx, session.AccessToken, 1, "partial", "internet_pause_unavailable"); err != nil {
		t.Fatal(err)
	}
	if err = service.RevokeDevice(ctx, parent.ID, device.ID); err != nil {
		t.Fatal(err)
	}
	if err = service.RevokeDevice(ctx, parent.ID, device.ID); err != nil {
		t.Fatal(err)
	}
	events, err := audit.List(ctx, db, parent.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	reasons := map[string]bool{}
	for _, event := range events {
		counts[event.Action]++
		reasons[event.Reason] = true
		if _, err := time.Parse(time.RFC3339Nano, event.OccurredAt); err != nil {
			t.Fatal(err)
		}
	}
	if counts["command_created"] != 1 || counts["policy_updated"] != 1 {
		t.Fatal("command or policy audit missing from security lifecycle")
	}
	if counts["app_inventory_updated"] != 1 || counts["policy_partial"] != 1 {
		t.Fatal("inventory or policy result audit missing")
	}
	for _, action := range []string{"parent_registered", "parent_login", "parent_login_failed", "parent_refresh", "parent_session_revoked", "pairing_code_created", "enrollment_started", "enrollment_completed", "enrollment_proof_failed", "device_authenticated", "device_auth_failed", "device_revoked"} {
		if counts[action] == 0 {
			t.Errorf("missing %s", action)
		}
	}
	if counts["device_revoked"] != 1 || !reasons["logout"] || !reasons["refresh_reuse"] || !reasons["parent_revoked"] {
		t.Fatalf("wrong revocation history: %v %v", counts, reasons)
	}
	encoded, _ := json.Marshal(events)
	if strings.Contains(string(encoded), "Private inventory label") || strings.Contains(string(encoded), "org.example.auditprivate") {
		t.Fatal("inventory or policy content leaked into audit")
	}
	for _, secret := range []string{password, parent.Email, "Sensitive device name", tokens.AccessToken, tokens.RefreshToken, rotated.AccessToken, session.AccessToken, code.Code, enrollment.Challenge, encode(der), encode(signature)} {
		if strings.Contains(string(encoded), secret) {
			t.Fatal("audit leaked sensitive data")
		}
	}
	others, err := audit.List(ctx, db, other.ID, 0, 100)
	if err != nil || len(others) != 1 || others[0].Action != "parent_registered" {
		t.Fatalf("account isolation: %v %v", others, err)
	}
	page, err := audit.List(ctx, db, parent.ID, events[0].ID, 1)
	if err != nil || len(page) != 1 || page[0].ID != events[1].ID {
		t.Fatal("invalid pagination")
	}
	for _, statement := range []string{
		"UPDATE audit_events SET action='parent_login'",
		"DELETE FROM audit_events",
		"INSERT OR REPLACE INTO audit_events SELECT * FROM audit_events WHERE id=1",
		"REPLACE INTO audit_events SELECT * FROM audit_events WHERE id=1",
	} {
		if _, err = db.ExecContext(ctx, statement); err == nil {
			t.Fatalf("mutation accepted: %s", statement)
		}
	}
	// An injected audit storage failure must roll back the triggering action.
	_, err = db.ExecContext(ctx, `CREATE TRIGGER test_audit_failure BEFORE INSERT ON audit_events BEGIN SELECT RAISE(ABORT,'injected audit failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.CreateCode(ctx, parent.ID); err == nil {
		t.Fatal("unaudited pairing code created")
	}
	var codes int
	if err = db.QueryRow("SELECT COUNT(*) FROM pairing_codes").Scan(&codes); err != nil || codes != 1 {
		t.Fatal("failed pairing code issuance did not roll back")
	}
	if _, err = authentication.Register(ctx, "rollback@example.com", password); err == nil {
		t.Fatal("unaudited account created")
	}
	var remaining int
	if err = db.QueryRow("SELECT COUNT(*) FROM parent_accounts WHERE email='rollback@example.com'").Scan(&remaining); err != nil || remaining != 0 {
		t.Fatal("audit failure did not roll back account")
	}
	if _, err = db.Exec("DROP TRIGGER test_audit_failure"); err != nil {
		t.Fatal(err)
	}
	// An ordinary caller transaction rollback removes its audit events too.
	err = storage.WithinTransaction(ctx, db, func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO parent_accounts(id,email,password_hash,created_at,updated_at) VALUES('rolled-back','rollback@example.com','unused',?,?)", now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		return context.Canceled
	})
	if err != context.Canceled {
		t.Fatal(err)
	}
	var orphan int
	if err = db.QueryRow("SELECT COUNT(*) FROM audit_events WHERE parent_account_id='rolled-back'").Scan(&orphan); err != nil || orphan != 0 {
		t.Fatal("orphan rollback audit")
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := audit.List(ctx, db, parent.ID, 0, 100)
	if err != nil || len(persisted) != len(events) {
		t.Fatal("audit did not survive reopen and repeated migration")
	}
	if _, err = db.Exec("DELETE FROM audit_events"); err == nil {
		t.Fatal("immutability lost on reopen")
	}
	backupPath := filepath.Join(t.TempDir(), "audit.acb")
	restoredPath := filepath.Join(t.TempDir(), "restored.db")
	backupKey := make([]byte, 32)
	if _, err = rand.Read(backupKey); err != nil {
		t.Fatal(err)
	}
	if err = storage.CreateEncryptedBackup(ctx, db, backupPath, backupKey); err != nil {
		t.Fatal(err)
	}
	if err = storage.RestoreEncryptedBackup(ctx, backupPath, restoredPath, backupKey); err != nil {
		t.Fatal(err)
	}
	restored, err := storage.Open(ctx, restoredPath)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	restoredEvents, err := audit.List(ctx, restored, parent.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	restoredJSON, _ := json.Marshal(restoredEvents)
	if string(restoredJSON) != string(encoded) {
		t.Fatal("audit changed after encrypted restore")
	}
	var reportVersion int
	if err = restored.QueryRow("SELECT version FROM device_policy_reports WHERE device_id=?", device.ID).Scan(&reportVersion); err != nil || reportVersion != 1 {
		t.Fatal("policy report lost during encrypted restore")
	}
	var inventoryJSON string
	if err = restored.QueryRow("SELECT apps_json FROM device_app_inventory WHERE device_id=?", device.ID).Scan(&inventoryJSON); err != nil || !strings.Contains(inventoryJSON, "org.example.auditprivate") {
		t.Fatal("inventory lost during encrypted restore")
	}
	if _, err = restored.Exec("DELETE FROM audit_events"); err == nil {
		t.Fatal("immutability lost after restore")
	}
}
