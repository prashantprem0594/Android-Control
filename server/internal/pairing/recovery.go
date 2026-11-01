package pairing

import (
	"context"
	"crypto/ecdsa"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"time"

	"appcontrol/server/internal/storage"
)

const recoveryDomain = "app-control-device-recovery-v1\n"

var (
	ErrRecoveryUnavailable = errors.New("device recovery is unavailable")
	ErrInvalidRecovery     = errors.New("invalid device recovery request")
	ErrRecoveryNotFound    = errors.New("device recovery request not found")
)

type RecoveryStart struct {
	RequestID    string    `json:"requestId"`
	RequestToken string    `json:"requestToken"`
	Challenge    string    `json:"challenge"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

type RecoveryStatus struct {
	State    string `json:"state"`
	DeviceID string `json:"deviceId,omitempty"`
}

type PendingRecovery struct {
	RequestID   string    `json:"requestId"`
	DeviceID    string    `json:"deviceId"`
	DisplayName string    `json:"displayName"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

func (s *Service) recoveryBindingHash(handle string) ([]byte, error) {
	if len(s.recoveryHMACKey) != 32 {
		return nil, ErrRecoveryUnavailable
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(handle)
	if err != nil || len(handle) != 43 || len(raw) != 32 {
		return nil, ErrInvalidRecovery
	}
	mac := hmac.New(sha256.New, s.recoveryHMACKey)
	_, _ = mac.Write(raw)
	return mac.Sum(nil), nil
}

func (s *Service) BindRecovery(ctx context.Context, deviceID, handle string) error {
	hash, err := s.recoveryBindingHash(handle)
	if err != nil {
		return err
	}
	now := formatTime(s.now().UTC())
	result, err := s.database.ExecContext(ctx, `INSERT INTO device_recovery_bindings(device_id,binding_hash,created_at,updated_at)
		SELECT id,?,?,? FROM devices WHERE id=? AND state='active'
		ON CONFLICT(device_id) DO UPDATE SET binding_hash=excluded.binding_hash,updated_at=excluded.updated_at`, hash, now, now, deviceID)
	if err != nil {
		return fmt.Errorf("bind recovery handle: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return ErrInvalidCredential
	}
	return nil
}

func (s *Service) StartRecovery(ctx context.Context, handle, publicKeyValue, algorithm string) (RecoveryStart, error) {
	bindingHash, err := s.recoveryBindingHash(handle)
	if err != nil {
		return RecoveryStart{}, err
	}
	if algorithm != KeyAlgorithmECDSAP256SHA256 {
		return RecoveryStart{}, ErrInvalidRecovery
	}
	der, _, err := parsePublicKey(publicKeyValue)
	if err != nil {
		return RecoveryStart{}, ErrInvalidRecovery
	}
	id, err := s.randomValue("drr_", 16)
	if err != nil {
		return RecoveryStart{}, err
	}
	tokenBytes := make([]byte, 32)
	if _, err := io.ReadFull(s.random, tokenBytes); err != nil {
		return RecoveryStart{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	challenge := make([]byte, 32)
	if _, err = io.ReadFull(s.random, challenge); err != nil {
		return RecoveryStart{}, err
	}
	tokenHash := sha256.Sum256([]byte(token))
	challengeHash := sha256.Sum256(challenge)
	now := s.now().UTC()
	expires := now.Add(5 * time.Minute)
	_, err = s.database.ExecContext(ctx, `INSERT INTO device_recovery_requests
		(id,device_id,request_token_hash,proposed_public_key_der,key_algorithm,challenge_hash,state,created_at,expires_at)
		VALUES(?,(SELECT b.device_id FROM device_recovery_bindings b JOIN devices d ON d.id=b.device_id
		 WHERE b.binding_hash=? AND d.state='active'),?,?,?,?, 'awaiting_proof',?,?)`,
		id, bindingHash, tokenHash[:], der, algorithm, challengeHash[:], formatTime(now), formatTime(expires))
	if err != nil {
		return RecoveryStart{}, fmt.Errorf("start recovery: %w", err)
	}
	return RecoveryStart{RequestID: id, RequestToken: token, Challenge: base64.RawURLEncoding.EncodeToString(challenge), ExpiresAt: expires}, nil
}

func recoveryMessage(id string, challenge []byte) []byte {
	return append([]byte(recoveryDomain+id+"\n"), challenge...)
}

func (s *Service) ProveRecovery(ctx context.Context, id, token, challengeValue, signatureValue string) error {
	if !validOpaqueValue(id, "drr_", 16) || !validRecoveryToken(token) {
		return ErrInvalidRecovery
	}
	challenge, err := base64.RawURLEncoding.Strict().DecodeString(challengeValue)
	if err != nil || len(challenge) != 32 {
		return ErrInvalidRecovery
	}
	signature, err := base64.RawURLEncoding.Strict().DecodeString(signatureValue)
	if err != nil || len(signature) == 0 || len(signature) > 256 {
		return ErrInvalidRecovery
	}
	tokenHash := sha256.Sum256([]byte(token))
	challengeHash := sha256.Sum256(challenge)
	now := s.now().UTC()
	valid := false
	err = storage.WithinTransaction(ctx, s.database, func(tx *sql.Tx) error {
		var storedToken, storedChallenge, der []byte
		var state, expiry string
		err := tx.QueryRowContext(ctx, `SELECT request_token_hash,challenge_hash,proposed_public_key_der,state,expires_at
			FROM device_recovery_requests WHERE id=?`, id).Scan(&storedToken, &storedChallenge, &der, &state, &expiry)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidRecovery
		}
		if err != nil {
			return err
		}
		deadline, err := parseTime(expiry)
		if err != nil {
			return err
		}
		key, keyErr := parsePublicKeyDER(der)
		digest := sha256.Sum256(recoveryMessage(id, challenge))
		valid = state == "awaiting_proof" && now.Before(deadline) &&
			subtle.ConstantTimeCompare(tokenHash[:], storedToken) == 1 && subtle.ConstantTimeCompare(challengeHash[:], storedChallenge) == 1 &&
			keyErr == nil && ecdsa.VerifyASN1(key, digest[:], signature)
		if !valid {
			return ErrInvalidRecovery
		}
		_, err = tx.ExecContext(ctx, `UPDATE device_recovery_requests SET state='awaiting_parent',proof_verified_at=?
			WHERE id=? AND state='awaiting_proof'`, formatTime(now), id)
		return err
	})
	if err != nil {
		return err
	}
	if !valid {
		return ErrInvalidRecovery
	}
	return nil
}

func (s *Service) RecoveryStatus(ctx context.Context, id, token string) (RecoveryStatus, error) {
	if !validOpaqueValue(id, "drr_", 16) || !validRecoveryToken(token) {
		return RecoveryStatus{}, ErrInvalidRecovery
	}
	hash := sha256.Sum256([]byte(token))
	var stored []byte
	var state, expiry string
	var device sql.NullString
	err := s.database.QueryRowContext(ctx, `SELECT request_token_hash,state,expires_at,device_id FROM device_recovery_requests WHERE id=?`, id).Scan(&stored, &state, &expiry, &device)
	if err != nil {
		return RecoveryStatus{}, ErrInvalidRecovery
	}
	deadline, err := parseTime(expiry)
	if err != nil || subtle.ConstantTimeCompare(hash[:], stored) != 1 {
		return RecoveryStatus{}, ErrInvalidRecovery
	}
	if !s.now().UTC().Before(deadline) && state != "approved" && state != "rejected" {
		return RecoveryStatus{State: "expired"}, nil
	}
	if state == "approved" {
		return RecoveryStatus{State: "approved", DeviceID: device.String}, nil
	}
	if state == "rejected" {
		return RecoveryStatus{State: "rejected"}, nil
	}
	return RecoveryStatus{State: "awaiting_parent"}, nil
}

func validRecoveryToken(value string) bool {
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(value)
	return len(value) == 43 && err == nil && len(decoded) == 32
}

func (s *Service) PendingRecoveries(ctx context.Context, parentID string) ([]PendingRecovery, error) {
	rows, err := s.database.QueryContext(ctx, `SELECT r.id,d.id,d.display_name,r.expires_at FROM device_recovery_requests r
		JOIN devices d ON d.id=r.device_id WHERE d.parent_account_id=? AND d.state='active' AND r.state='awaiting_parent' AND r.expires_at>? ORDER BY r.created_at`, parentID, formatTime(s.now().UTC()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []PendingRecovery{}
	for rows.Next() {
		var item PendingRecovery
		var expiry string
		if err = rows.Scan(&item.RequestID, &item.DeviceID, &item.DisplayName, &expiry); err != nil {
			return nil, err
		}
		item.ExpiresAt, err = parseTime(expiry)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Service) ResolveRecovery(ctx context.Context, parentID, id string, approve bool) error {
	if !validOpaqueValue(id, "drr_", 16) {
		return ErrRecoveryNotFound
	}
	now := s.now().UTC()
	return storage.WithinTransaction(ctx, s.database, func(tx *sql.Tx) error {
		var deviceID, state, expiry string
		var der []byte
		err := tx.QueryRowContext(ctx, `SELECT r.device_id,r.state,r.expires_at,r.proposed_public_key_der FROM device_recovery_requests r
			JOIN devices d ON d.id=r.device_id WHERE r.id=? AND d.parent_account_id=? AND d.state='active'`, id, parentID).Scan(&deviceID, &state, &expiry, &der)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrRecoveryNotFound
		}
		if err != nil {
			return err
		}
		deadline, err := parseTime(expiry)
		if err != nil {
			return err
		}
		if state != "awaiting_parent" || !now.Before(deadline) {
			return ErrRecoveryNotFound
		}
		if !approve {
			_, err = tx.ExecContext(ctx, `UPDATE device_recovery_requests SET state='rejected',resolved_at=? WHERE id=? AND state='awaiting_parent'`, formatTime(now), id)
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE devices SET public_key_der=?,key_algorithm=?,credential_version=credential_version+1,
			credential_updated_at=?,last_reattached_at=? WHERE id=? AND parent_account_id=? AND state='active'`, der, KeyAlgorithmECDSAP256SHA256, formatTime(now), formatTime(now), deviceID, parentID)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil || count != 1 {
			return ErrRecoveryNotFound
		}
		for _, q := range []string{"DELETE FROM device_sessions WHERE device_id=?", "DELETE FROM device_auth_challenges WHERE device_id=?", "DELETE FROM wakeup_registration_tokens WHERE device_id=?"} {
			if _, err = tx.ExecContext(ctx, q, deviceID); err != nil {
				return err
			}
		}
		// A fresh installation must not execute commands that were queued or
		// delivered to the credential it replaced.
		if _, err = tx.ExecContext(ctx, `UPDATE commands SET status='expired',status_at=?
			WHERE device_id=? AND status IN ('queued','delivered')`, formatTime(now), deviceID); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE remote_assistance_sessions SET status='closed',closed_at=? WHERE device_id=? AND status='active'`, formatTime(now), deviceID); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE device_recovery_requests SET state='rejected',resolved_at=? WHERE device_id=? AND id<>? AND state IN ('awaiting_proof','awaiting_parent')`, formatTime(now), deviceID, id); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE device_recovery_requests SET state='approved',resolved_at=? WHERE id=? AND state='awaiting_parent'`, formatTime(now), id); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO audit_events(occurred_at,parent_account_id,actor_type,actor_id,action,target_id)
			VALUES(?,?,'parent',?,'device_credential_rotated',?)`, formatTime(now), parentID, parentID, deviceID)
		return err
	})
}
