package auth

import (
	"appcontrol/server/internal/audit"
	"appcontrol/server/internal/pairing"
	"context"
	"errors"
	"testing"
)

func TestRecoveryRevokesSessionsAndUnusedEnrollmentButPreservesActiveDevices(t *testing.T) {
	s, db, clock := newTestService(t, nil)
	ctx := context.Background()
	parent := registerTestParent(t, s)
	other, err := s.Register(ctx, "other@example.com", "another-fixture-password")
	if err != nil {
		t.Fatal(err)
	}
	_, first, err := s.Login(ctx, parent.Email, testPassword)
	if err != nil {
		t.Fatal(err)
	}
	_, second, err := s.Login(ctx, parent.Email, testPassword)
	if err != nil {
		t.Fatal(err)
	}
	_, otherTokens, err := s.Login(ctx, other.Email, "another-fixture-password")
	if err != nil {
		t.Fatal(err)
	}
	pairingService, err := pairing.NewService(db, pairing.Options{Now: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	code, err := pairingService.CreateCode(ctx, parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"active", "pending"} {
		if _, err := db.Exec("INSERT INTO devices(id,parent_account_id,display_name,public_key_der,key_algorithm,state,created_at) VALUES(?,?,?,X'00','ECDSA_P256_SHA256',?,?)", state, parent.ID, "Fixture", state, formatTime(clock.Now())); err != nil {
			t.Fatal(err)
		}
	}
	var oldHash string
	if err := db.QueryRow("SELECT password_hash FROM parent_accounts WHERE id=?", parent.ID).Scan(&oldHash); err != nil {
		t.Fatal(err)
	}
	if err := s.RecoverPassword(ctx, " Parent@Example.com ", "new-fixture-password"); err != nil {
		t.Fatal(err)
	}
	for _, tokens := range []TokenPair{first, second} {
		if _, _, err := s.Authenticate(ctx, tokens.AccessToken); !errors.Is(err, ErrInvalidToken) {
			t.Fatal("old access survived recovery")
		}
		if _, err := s.Refresh(ctx, tokens.RefreshToken); !errors.Is(err, ErrInvalidToken) {
			t.Fatal("old refresh survived recovery")
		}
	}
	if _, _, err := s.Authenticate(ctx, otherTokens.AccessToken); err != nil {
		t.Fatal("other parent's session revoked")
	}
	if _, _, err := s.Login(ctx, parent.Email, testPassword); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatal("old password accepted")
	}
	if _, _, err := s.Login(ctx, parent.Email, "new-fixture-password"); err != nil {
		t.Fatal("new password rejected")
	}
	if _, err := s.createSession(ctx, parent.ID, clock.Now(), oldHash); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatal("stale verified password minted a session")
	}
	var active, pending string
	if err := db.QueryRow("SELECT state FROM devices WHERE id='active'").Scan(&active); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT state FROM devices WHERE id='pending'").Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if active != "active" || pending != "revoked" {
		t.Fatal("incorrect enrollment recovery boundary")
	}
	var expiry string
	if err := db.QueryRow("SELECT expires_at FROM pairing_codes WHERE parent_account_id=?", parent.ID).Scan(&expiry); err != nil || expiry != formatTime(clock.Now()) {
		t.Fatal("unused pairing code survived recovery")
	}
	_ = code
	events, err := audit.List(ctx, db, parent.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range events {
		if event.Action == "parent_password_reset" {
			found = true
		}
	}
	if !found {
		t.Fatal("password reset missing from immutable audit")
	}
}

func TestRecoveryAuditFailureRollsBackPasswordAndSessionChanges(t *testing.T) {
	s, db, _ := newTestService(t, nil)
	ctx := context.Background()
	parent := registerTestParent(t, s)
	_, tokens, err := s.Login(ctx, parent.Email, testPassword)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER reject_fixture_password_reset BEFORE INSERT ON audit_events WHEN NEW.action='parent_password_reset' BEGIN SELECT RAISE(ABORT,'fixture'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.RecoverPassword(ctx, parent.Email, "new-fixture-password"); err == nil {
		t.Fatal("recovery ignored audit failure")
	}
	if _, _, err := s.Authenticate(ctx, tokens.AccessToken); err != nil {
		t.Fatal("failed recovery revoked session")
	}
	if _, _, err := s.Login(ctx, parent.Email, testPassword); err != nil {
		t.Fatal("failed recovery changed password")
	}
}
