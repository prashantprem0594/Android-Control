package control

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWakeupTokensEncryptBindRotateAndRevoke(t *testing.T) {
	db, _ := fixture(t)
	ctx := context.Background()
	now := time.Now().UTC()
	store, err := NewWakeupTokens(db, bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return now }
	token := session(t, db, "device", "W", now)
	if err := store.Register(ctx, token, "routing-private-fixture"); err != nil {
		t.Fatal("registration failed")
	}
	var sealed []byte
	if err := db.QueryRow("SELECT registration_ciphertext FROM wakeup_registration_tokens WHERE device_id='device'").Scan(&sealed); err != nil || bytes.Contains(sealed, []byte("routing-private-fixture")) {
		t.Fatal("routing token not encrypted")
	}
	if value, err := store.Registration(ctx, "device"); err != nil || value != "routing-private-fixture" {
		t.Fatal("encrypted routing token unavailable")
	}
	if _, err := db.Exec(`INSERT INTO devices(id,parent_account_id,display_name,public_key_der,key_algorithm,state,created_at) VALUES('device-two','owner','test',X'00','ECDSA_P256_SHA256','active',?)`, now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	two := session(t, db, "device-two", "X", now)
	if err := store.Register(ctx, two, "other-routing-fixture"); err != nil {
		t.Fatal("second registration failed")
	}
	if _, err := db.Exec("UPDATE wakeup_registration_tokens SET registration_ciphertext=? WHERE device_id='device-two'", sealed); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Registration(ctx, "device-two"); !errors.Is(err, ErrInvalid) {
		t.Fatal("ciphertext replayed across device identities")
	}
	otherKey, err := NewWakeupTokens(db, bytes.Repeat([]byte{2}, 32))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := otherKey.Registration(ctx, "device"); !errors.Is(err, ErrInvalid) {
		t.Fatal("wrong routing key accepted")
	}
	if err := store.Register(ctx, token, "rotated-routing-fixture"); err != nil {
		t.Fatal("token rotation failed")
	}
	if value, err := store.Registration(ctx, "device"); err != nil || value != "rotated-routing-fixture" {
		t.Fatal("old routing token survived rotation")
	}
	if err := store.Register(ctx, "invalid", "foreign-routing"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("registration without device session accepted")
	}
	for _, value := range []string{"", "bad\nvalue", strings.Repeat("x", 4097)} {
		if err := store.Register(ctx, token, value); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid routing token accepted")
		}
	}
	if _, err := db.Exec("UPDATE devices SET state='revoked',revoked_at=?,revocation_reason='parent_revoked' WHERE id='device'", now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM wakeup_registration_tokens WHERE device_id='device'").Scan(&count); err != nil || count != 0 {
		t.Fatal("revoked registration not deleted")
	}
	if _, err := store.Registration(ctx, "device"); !errors.Is(err, ErrNotFound) {
		t.Fatal("revoked routing token resolved")
	}
	if err := store.Register(ctx, token, "after-revocation"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("revoked device replaced registration")
	}
	if err := store.Remove(ctx, two); err != nil {
		t.Fatal("authenticated registration removal failed")
	}
}
