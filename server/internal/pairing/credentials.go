package pairing

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"io"
	"time"

	"appcontrol/server/internal/storage"
)

var ErrInvalidCredential = errors.New("invalid or revoked device credential")

type AuthChallenge struct {
	ID        string    `json:"challengeId"`
	Challenge string    `json:"challenge"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type DeviceSession struct {
	AccessToken string    `json:"accessToken"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

// One outstanding challenge per active device; requesting a new one invalidates the old one.
func (s *Service) CreateAuthChallenge(ctx context.Context, deviceID string) (AuthChallenge, error) {
	if !validOpaqueValue(deviceID, "dev_", 16) {
		return AuthChallenge{}, ErrInvalidCredential
	}
	id, err := s.randomValue("dac_", 16)
	if err != nil {
		return AuthChallenge{}, err
	}
	raw := make([]byte, 32)
	if _, err := io.ReadFull(s.random, raw); err != nil {
		return AuthChallenge{}, err
	}
	hash := sha256.Sum256(raw)
	expires := s.now().UTC().Add(time.Minute)
	result, err := s.database.ExecContext(ctx, `INSERT INTO device_auth_challenges(id,device_id,challenge_hash,expires_at,consumed)
 SELECT ?,id,?,?,0 FROM devices WHERE id=? AND state='active'
 ON CONFLICT(device_id) DO UPDATE SET id=excluded.id,challenge_hash=excluded.challenge_hash,expires_at=excluded.expires_at,consumed=0`, id, hash[:], formatTime(expires), deviceID)
	if err != nil {
		return AuthChallenge{}, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return AuthChallenge{}, err
	}
	if count != 1 {
		return AuthChallenge{}, ErrInvalidCredential
	}
	return AuthChallenge{ID: id, Challenge: base64.RawURLEncoding.EncodeToString(raw), ExpiresAt: expires}, nil
}

func authMessage(deviceID, challengeID string, challenge []byte) []byte {
	return append([]byte("app-control-device-auth-v1\n"+deviceID+"\n"+challengeID+"\n"), challenge...)
}

func (s *Service) CompleteAuthentication(ctx context.Context, deviceID, challengeID, challengeValue, signatureValue string) (DeviceSession, error) {
	if !validOpaqueValue(deviceID, "dev_", 16) || !validOpaqueValue(challengeID, "dac_", 16) {
		return DeviceSession{}, ErrInvalidCredential
	}
	challenge, err := base64.RawURLEncoding.Strict().DecodeString(challengeValue)
	if err != nil || len(challenge) != 32 {
		return DeviceSession{}, ErrInvalidCredential
	}
	signature, err := base64.RawURLEncoding.Strict().DecodeString(signatureValue)
	if err != nil || len(signature) == 0 || len(signature) > 256 {
		return DeviceSession{}, ErrInvalidCredential
	}
	token, err := s.randomValue("dt_", 32)
	if err != nil {
		return DeviceSession{}, err
	}
	now := s.now().UTC()
	expires := now.Add(15 * time.Minute)
	valid := false
	err = storage.WithinTransaction(ctx, s.database, func(tx *sql.Tx) error {
		var der, hash []byte
		var expiry string
		var consumed int
		err := tx.QueryRowContext(ctx, `SELECT d.public_key_der,c.challenge_hash,c.expires_at,c.consumed FROM device_auth_challenges c
   JOIN devices d ON d.id=c.device_id WHERE c.id=? AND d.id=? AND d.state='active'`, challengeID, deviceID).Scan(&der, &hash, &expiry, &consumed)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidCredential
		}
		if err != nil {
			return err
		}
		deadline, err := parseTime(expiry)
		if err != nil {
			return err
		}
		if consumed != 0 || !now.Before(deadline) {
			return ErrInvalidCredential
		}
		// Consume even a bad proof to bound verification attempts per challenge.
		if _, err = tx.ExecContext(ctx, "UPDATE device_auth_challenges SET consumed=1 WHERE id=?", challengeID); err != nil {
			return err
		}
		key, err := parsePublicKeyDER(der)
		actual := sha256.Sum256(challenge)
		digest := sha256.Sum256(authMessage(deviceID, challengeID, challenge))
		if err != nil || subtle.ConstantTimeCompare(actual[:], hash) != 1 || !ecdsa.VerifyASN1(key, digest[:], signature) {
			_, auditErr := tx.ExecContext(ctx, `INSERT INTO audit_events(occurred_at,parent_account_id,actor_type,actor_id,action,target_id)
                SELECT ?,parent_account_id,'anonymous','','device_auth_failed',id FROM devices WHERE id=?`, formatTime(now), deviceID)
			return auditErr
		}
		tokenHash := sha256.Sum256([]byte(token))
		// Keep one session per device; obtaining a new session invalidates the old token.
		if _, err = tx.ExecContext(ctx, "DELETE FROM device_sessions WHERE device_id=?", deviceID); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO device_sessions(token_hash,device_id,expires_at,issued_at) VALUES(?,?,?,?)", tokenHash[:], deviceID, formatTime(expires), formatTime(now)); err != nil {
			return err
		}
		valid = true
		return nil
	})
	if err != nil {
		return DeviceSession{}, err
	}
	if !valid {
		return DeviceSession{}, ErrInvalidCredential
	}
	return DeviceSession{AccessToken: token, ExpiresAt: expires}, nil
}

// Always consult persistent state; no cached authorization survives revocation.
func (s *Service) AuthenticateDevice(ctx context.Context, token string) (string, error) {
	if !validOpaqueValue(token, "dt_", 32) {
		return "", ErrInvalidCredential
	}
	hash := sha256.Sum256([]byte(token))
	var id, expiry string
	err := s.database.QueryRowContext(ctx, `SELECT d.id,s.expires_at FROM device_sessions s JOIN devices d ON d.id=s.device_id WHERE s.token_hash=? AND d.state='active'`, hash[:]).Scan(&id, &expiry)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrInvalidCredential
	}
	if err != nil {
		return "", err
	}
	deadline, err := parseTime(expiry)
	if err != nil {
		return "", err
	}
	if !s.now().UTC().Before(deadline) {
		return "", ErrInvalidCredential
	}
	return id, nil
}

func (s *Service) RevokeDevice(ctx context.Context, parentID, deviceID string) error {
	return storage.WithinTransaction(ctx, s.database, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE devices SET state='revoked',revoked_at=COALESCE(revoked_at,?),revocation_reason=COALESCE(revocation_reason,'parent_revoked') WHERE id=? AND parent_account_id=?`, formatTime(s.now()), deviceID, parentID)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrDeviceNotFound
		}
		if _, err = tx.ExecContext(ctx, "DELETE FROM device_sessions WHERE device_id=?", deviceID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "DELETE FROM device_auth_challenges WHERE device_id=?", deviceID)
		return err
	})
}
