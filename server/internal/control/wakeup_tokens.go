package control

import (
	"appcontrol/server/internal/storage"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"errors"
	"time"
)

// WakeupTokens is routing metadata only, never command authorization. Tokens
// are authenticated ciphertext with device identity bound as additional data.
type WakeupTokens struct {
	db     *sql.DB
	cipher cipher.AEAD
	now    func() time.Time
}

func NewWakeupTokens(db *sql.DB, key []byte) (*WakeupTokens, error) {
	if db == nil || len(key) != 32 {
		return nil, ErrInvalid
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrInvalid
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrInvalid
	}
	return &WakeupTokens{db: db, cipher: aead, now: time.Now}, nil
}
func (s *WakeupTokens) Register(ctx context.Context, credential, registration string) error {
	if len(registration) < 1 || len(registration) > 4096 {
		return ErrInvalid
	}
	for _, character := range registration {
		if character < 33 || character > 126 {
			return ErrInvalid
		}
	}
	return storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		now := s.now().UTC()
		id, err := deviceAuthorization(ctx, tx, credential, now)
		if err != nil {
			return err
		}
		nonce := make([]byte, s.cipher.NonceSize())
		if _, err := rand.Read(nonce); err != nil {
			return ErrInvalid
		}
		sealed := s.cipher.Seal(nonce, nonce, []byte(registration), []byte(id))
		_, err = tx.ExecContext(ctx, `INSERT INTO wakeup_registration_tokens(device_id,registration_ciphertext,updated_at) VALUES(?,?,?)
		 ON CONFLICT(device_id) DO UPDATE SET registration_ciphertext=excluded.registration_ciphertext,updated_at=excluded.updated_at`, id, sealed, now.Format(time.RFC3339Nano))
		return err
	})
}
func (s *WakeupTokens) Remove(ctx context.Context, credential string) error {
	return storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		id, err := deviceAuthorization(ctx, tx, credential, s.now().UTC())
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "DELETE FROM wakeup_registration_tokens WHERE device_id=?", id)
		return err
	})
}
func (s *WakeupTokens) Registration(ctx context.Context, deviceID string) (string, error) {
	var sealed []byte
	err := s.db.QueryRowContext(ctx, `SELECT t.registration_ciphertext FROM wakeup_registration_tokens t JOIN devices d ON d.id=t.device_id
	 WHERE t.device_id=? AND d.state='active'`, deviceID).Scan(&sealed)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", ErrInvalid
	}
	n := s.cipher.NonceSize()
	if len(sealed) < n+s.cipher.Overhead()+1 || len(sealed) > n+s.cipher.Overhead()+4096 {
		return "", ErrInvalid
	}
	cleartext, err := s.cipher.Open(nil, sealed[:n], sealed[n:], []byte(deviceID))
	if err != nil {
		return "", ErrInvalid
	}
	defer clear(cleartext)
	return string(cleartext), nil
}
