package pairing

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"appcontrol/server/internal/storage"
)

const (
	KeyAlgorithmECDSAP256SHA256 = "ECDSA_P256_SHA256"
	challengeDomain             = "app-control-device-enrollment-v1\n"
)

var (
	ErrInvalidPairingCode = errors.New("invalid or expired pairing code")
	ErrInvalidEnrollment  = errors.New("invalid or expired enrollment")
	ErrInvalidProof       = errors.New("invalid device key proof")
	ErrDeviceNotFound     = errors.New("device not found")
)

type Options struct {
	Now                  func() time.Time
	Random               io.Reader
	PairingCodeLifetime  time.Duration
	EnrollmentLifetime   time.Duration
	MaxVerificationTries int
	RecoveryHMACKey      []byte
}

type Service struct {
	database             *sql.DB
	now                  func() time.Time
	random               io.Reader
	pairingCodeLifetime  time.Duration
	enrollmentLifetime   time.Duration
	maxVerificationTries int
	recoveryHMACKey      []byte
}

type PairingCode struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type Enrollment struct {
	ID           string    `json:"enrollmentId"`
	DeviceID     string    `json:"deviceId"`
	Challenge    string    `json:"challenge"`
	ExpiresAt    time.Time `json:"expiresAt"`
	KeyAlgorithm string    `json:"keyAlgorithm"`
}

type Device struct {
	ID           string     `json:"id"`
	ParentID     string     `json:"parentId"`
	DisplayName  string     `json:"displayName"`
	KeyAlgorithm string     `json:"keyAlgorithm"`
	State        string     `json:"state"`
	CreatedAt    time.Time  `json:"createdAt"`
	EnrolledAt   *time.Time `json:"enrolledAt,omitempty"`
}

func NewService(database *sql.DB, options Options) (*Service, error) {
	if database == nil {
		return nil, fmt.Errorf("database is required")
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Random == nil {
		options.Random = rand.Reader
	}
	if options.PairingCodeLifetime == 0 {
		options.PairingCodeLifetime = 10 * time.Minute
	}
	if options.EnrollmentLifetime == 0 {
		options.EnrollmentLifetime = 5 * time.Minute
	}
	if options.MaxVerificationTries == 0 {
		options.MaxVerificationTries = 5
	}
	if options.PairingCodeLifetime <= 0 || options.EnrollmentLifetime <= 0 || options.MaxVerificationTries < 1 {
		return nil, fmt.Errorf("pairing lifetimes and attempt limit must be positive")
	}
	if len(options.RecoveryHMACKey) != 0 && len(options.RecoveryHMACKey) != 32 {
		return nil, fmt.Errorf("recovery HMAC key must contain exactly 32 bytes")
	}
	return &Service{
		database: database, now: options.Now, random: options.Random,
		pairingCodeLifetime:  options.PairingCodeLifetime,
		enrollmentLifetime:   options.EnrollmentLifetime,
		maxVerificationTries: options.MaxVerificationTries,
		recoveryHMACKey:      append([]byte(nil), options.RecoveryHMACKey...),
	}, nil
}

func (service *Service) CreateCode(ctx context.Context, parentID string) (PairingCode, error) {
	if strings.TrimSpace(parentID) == "" {
		return PairingCode{}, fmt.Errorf("parent account is required")
	}
	code, err := service.randomValue("pc_", 16)
	if err != nil {
		return PairingCode{}, err
	}
	hash := sha256.Sum256([]byte(code))
	now := service.now().UTC()
	expiresAt := now.Add(service.pairingCodeLifetime)
	if _, err := service.database.ExecContext(ctx,
		"INSERT INTO pairing_codes(code_hash, parent_account_id, created_at, expires_at) VALUES (?, ?, ?, ?)",
		hash[:], parentID, formatTime(now), formatTime(expiresAt),
	); err != nil {
		return PairingCode{}, fmt.Errorf("create pairing code: %w", err)
	}
	return PairingCode{Code: code, ExpiresAt: expiresAt}, nil
}

func (service *Service) StartEnrollment(ctx context.Context, code, displayName, publicKeyValue, keyAlgorithm string) (Enrollment, error) {
	if !validOpaqueValue(code, "pc_", 16) {
		return Enrollment{}, ErrInvalidPairingCode
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" || len(displayName) > 100 {
		return Enrollment{}, fmt.Errorf("display name must contain 1 to 100 bytes")
	}
	if keyAlgorithm != KeyAlgorithmECDSAP256SHA256 {
		return Enrollment{}, fmt.Errorf("unsupported key algorithm")
	}
	publicKeyDER, publicKey, err := parsePublicKey(publicKeyValue)
	if err != nil {
		return Enrollment{}, err
	}
	if publicKey.Curve != elliptic.P256() {
		return Enrollment{}, fmt.Errorf("public key must use P-256")
	}
	deviceID, err := service.randomValue("dev_", 16)
	if err != nil {
		return Enrollment{}, err
	}
	enrollmentID, err := service.randomValue("enr_", 16)
	if err != nil {
		return Enrollment{}, err
	}
	challengeBytes := make([]byte, 32)
	if _, err := io.ReadFull(service.random, challengeBytes); err != nil {
		return Enrollment{}, fmt.Errorf("generate enrollment challenge: %w", err)
	}
	challenge := base64.RawURLEncoding.EncodeToString(challengeBytes)
	challengeHash := sha256.Sum256(challengeBytes)
	codeHash := sha256.Sum256([]byte(code))
	now := service.now().UTC()
	expiresAt := now.Add(service.enrollmentLifetime)
	err = storage.WithinTransaction(ctx, service.database, func(transaction *sql.Tx) error {
		var parentID, codeExpiresText string
		var consumedAt sql.NullString
		if err := transaction.QueryRowContext(ctx,
			"SELECT parent_account_id, expires_at, consumed_at FROM pairing_codes WHERE code_hash = ?", codeHash[:],
		).Scan(&parentID, &codeExpiresText, &consumedAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrInvalidPairingCode
			}
			return fmt.Errorf("load pairing code: %w", err)
		}
		codeExpires, err := parseTime(codeExpiresText)
		if err != nil {
			return err
		}
		if consumedAt.Valid || !now.Before(codeExpires) {
			return ErrInvalidPairingCode
		}
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO devices(id, parent_account_id, display_name, public_key_der, key_algorithm, state, created_at)
			VALUES (?, ?, ?, ?, ?, 'pending', ?)`,
			deviceID, parentID, displayName, publicKeyDER, keyAlgorithm, formatTime(now),
		); err != nil {
			return fmt.Errorf("create pending device: %w", err)
		}
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO device_enrollments(id, device_id, challenge_hash, created_at, expires_at)
			VALUES (?, ?, ?, ?, ?)`,
			enrollmentID, deviceID, challengeHash[:], formatTime(now), formatTime(expiresAt),
		); err != nil {
			return fmt.Errorf("create enrollment challenge: %w", err)
		}
		result, err := transaction.ExecContext(ctx, `
			UPDATE pairing_codes SET consumed_at = ?, enrollment_id = ?
			WHERE code_hash = ? AND consumed_at IS NULL`, formatTime(now), enrollmentID, codeHash[:])
		if err != nil {
			return fmt.Errorf("consume pairing code: %w", err)
		}
		rows, err := result.RowsAffected()
		if err != nil || rows != 1 {
			return ErrInvalidPairingCode
		}
		return nil
	})
	if err != nil {
		return Enrollment{}, err
	}
	return Enrollment{
		ID: enrollmentID, DeviceID: deviceID, Challenge: challenge,
		ExpiresAt: expiresAt, KeyAlgorithm: keyAlgorithm,
	}, nil
}

func (service *Service) CompleteEnrollment(ctx context.Context, enrollmentID, challengeValue, signatureValue string) (Device, error) {
	if !validOpaqueValue(enrollmentID, "enr_", 16) {
		return Device{}, ErrInvalidEnrollment
	}
	challenge, err := base64.RawURLEncoding.Strict().DecodeString(challengeValue)
	if err != nil || len(challenge) != 32 {
		return Device{}, ErrInvalidEnrollment
	}
	signature, err := base64.RawURLEncoding.Strict().DecodeString(signatureValue)
	if err != nil || len(signature) == 0 || len(signature) > 256 {
		return Device{}, ErrInvalidProof
	}
	now := service.now().UTC()
	invalidProof := false
	var completed Device
	err = storage.WithinTransaction(ctx, service.database, func(transaction *sql.Tx) error {
		var publicKeyDER, storedChallengeHash []byte
		var createdText, enrollmentExpiresText string
		var enrolledText, completedText sql.NullString
		var attempts int
		err := transaction.QueryRowContext(ctx, `
			SELECT d.id, d.parent_account_id, d.display_name, d.public_key_der, d.key_algorithm,
			       d.state, d.created_at, d.enrolled_at, e.challenge_hash, e.expires_at,
			       e.completed_at, e.verification_attempts
			FROM device_enrollments e JOIN devices d ON d.id = e.device_id
			WHERE e.id = ?`, enrollmentID,
		).Scan(&completed.ID, &completed.ParentID, &completed.DisplayName, &publicKeyDER,
			&completed.KeyAlgorithm, &completed.State, &createdText, &enrolledText,
			&storedChallengeHash, &enrollmentExpiresText, &completedText, &attempts)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidEnrollment
		}
		if err != nil {
			return fmt.Errorf("load enrollment: %w", err)
		}
		createdAt, err := parseTime(createdText)
		if err != nil {
			return err
		}
		completed.CreatedAt = createdAt
		expiresAt, err := parseTime(enrollmentExpiresText)
		if err != nil {
			return err
		}
		if completedText.Valid || completed.State != "pending" || attempts >= service.maxVerificationTries || !now.Before(expiresAt) {
			return ErrInvalidEnrollment
		}
		actualChallengeHash := sha256.Sum256(challenge)
		key, parseErr := parsePublicKeyDER(publicKeyDER)
		messageHash := sha256.Sum256(enrollmentMessage(enrollmentID, challenge))
		proofValid := parseErr == nil &&
			subtle.ConstantTimeCompare(actualChallengeHash[:], storedChallengeHash) == 1 &&
			ecdsa.VerifyASN1(key, messageHash[:], signature)
		if !proofValid {
			attempts++
			state := "pending"
			if attempts >= service.maxVerificationTries {
				state = "revoked"
			}
			if _, err := transaction.ExecContext(ctx,
				"UPDATE device_enrollments SET verification_attempts = ? WHERE id = ?", attempts, enrollmentID,
			); err != nil {
				return fmt.Errorf("record invalid enrollment proof: %w", err)
			}
			if state == "revoked" {
				if _, err := transaction.ExecContext(ctx,
					"UPDATE devices SET state = 'revoked', revoked_at = ?, revocation_reason = 'proof_failures' WHERE id = ?",
					formatTime(now), completed.ID,
				); err != nil {
					return fmt.Errorf("revoke failed enrollment: %w", err)
				}
			}
			invalidProof = true
			return nil
		}
		if _, err := transaction.ExecContext(ctx,
			"UPDATE device_enrollments SET completed_at = ? WHERE id = ?", formatTime(now), enrollmentID,
		); err != nil {
			return fmt.Errorf("complete enrollment: %w", err)
		}
		if _, err := transaction.ExecContext(ctx,
			"UPDATE devices SET state = 'active', enrolled_at = ? WHERE id = ?", formatTime(now), completed.ID,
		); err != nil {
			return fmt.Errorf("activate device: %w", err)
		}
		completed.State = "active"
		completed.EnrolledAt = &now
		return nil
	})
	if err != nil {
		return Device{}, err
	}
	if invalidProof {
		return Device{}, ErrInvalidProof
	}
	return completed, nil
}

func (service *Service) Device(ctx context.Context, parentID, deviceID string) (Device, error) {
	var result Device
	var createdText string
	var enrolledText sql.NullString
	err := service.database.QueryRowContext(ctx, `
		SELECT id, parent_account_id, display_name, key_algorithm, state, created_at, enrolled_at
		FROM devices WHERE id = ? AND parent_account_id = ?`, deviceID, parentID,
	).Scan(&result.ID, &result.ParentID, &result.DisplayName, &result.KeyAlgorithm, &result.State, &createdText, &enrolledText)
	if errors.Is(err, sql.ErrNoRows) {
		return Device{}, ErrDeviceNotFound
	}
	if err != nil {
		return Device{}, fmt.Errorf("load device: %w", err)
	}
	result.CreatedAt, err = parseTime(createdText)
	if err != nil {
		return Device{}, err
	}
	if enrolledText.Valid {
		enrolledAt, err := parseTime(enrolledText.String)
		if err != nil {
			return Device{}, err
		}
		result.EnrolledAt = &enrolledAt
	}
	return result, nil
}

func parsePublicKey(value string) ([]byte, *ecdsa.PublicKey, error) {
	der, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil || len(der) == 0 || len(der) > 1024 {
		return nil, nil, fmt.Errorf("invalid public key encoding")
	}
	key, err := parsePublicKeyDER(der)
	if err != nil {
		return nil, nil, err
	}
	return der, key, nil
}

func parsePublicKeyDER(der []byte) (*ecdsa.PublicKey, error) {
	parsed, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	key, ok := parsed.(*ecdsa.PublicKey)
	if !ok || key.Curve != elliptic.P256() {
		return nil, fmt.Errorf("public key must be ECDSA P-256")
	}
	return key, nil
}

func enrollmentMessage(enrollmentID string, challenge []byte) []byte {
	message := make([]byte, 0, len(challengeDomain)+len(enrollmentID)+1+len(challenge))
	message = append(message, challengeDomain...)
	message = append(message, enrollmentID...)
	message = append(message, '\n')
	message = append(message, challenge...)
	return message
}

func (service *Service) randomValue(prefix string, bytes int) (string, error) {
	randomBytes := make([]byte, bytes)
	if _, err := io.ReadFull(service.random, randomBytes); err != nil {
		return "", fmt.Errorf("generate pairing value: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(randomBytes), nil
}

func validOpaqueValue(value, prefix string, byteCount int) bool {
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(strings.TrimPrefix(value, prefix))
	return err == nil && len(decoded) == byteCount
}

func formatTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse stored pairing timestamp: %w", err)
	}
	return parsed, nil
}
