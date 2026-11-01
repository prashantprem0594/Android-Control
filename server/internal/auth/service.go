package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"strings"
	"time"

	"appcontrol/server/internal/storage"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrLoginRateLimited   = errors.New("login temporarily rate limited")
	ErrInvalidToken       = errors.New("invalid or expired token")
	ErrRefreshReuse       = errors.New("refresh token reuse detected")
	ErrEmailExists        = errors.New("email already registered")
)

type Options struct {
	Now                func() time.Time
	Random             io.Reader
	PasswordParameters PasswordParameters
	AccessLifetime     time.Duration
	RefreshLifetime    time.Duration
	LoginWindow        time.Duration
	LoginBlockDuration time.Duration
	MaxLoginFailures   int
}

type Service struct {
	database           *sql.DB
	now                func() time.Time
	random             io.Reader
	passwordParameters PasswordParameters
	accessLifetime     time.Duration
	refreshLifetime    time.Duration
	loginWindow        time.Duration
	loginBlockDuration time.Duration
	maxLoginFailures   int
	dummyPasswordHash  string
}

type Parent struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type TokenPair struct {
	AccessToken      string    `json:"accessToken"`
	AccessExpiresAt  time.Time `json:"accessExpiresAt"`
	RefreshToken     string    `json:"refreshToken"`
	RefreshExpiresAt time.Time `json:"refreshExpiresAt"`
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
	if options.PasswordParameters == (PasswordParameters{}) {
		options.PasswordParameters = DefaultPasswordParameters
	}
	if options.AccessLifetime == 0 {
		options.AccessLifetime = 15 * time.Minute
	}
	if options.RefreshLifetime == 0 {
		options.RefreshLifetime = 10 * 365 * 24 * time.Hour
	}
	if options.LoginWindow == 0 {
		options.LoginWindow = 15 * time.Minute
	}
	if options.LoginBlockDuration == 0 {
		options.LoginBlockDuration = 5 * time.Second
	}
	if options.MaxLoginFailures == 0 {
		options.MaxLoginFailures = 20
	}
	if err := validatePasswordParameters(options.PasswordParameters); err != nil {
		return nil, err
	}
	if options.AccessLifetime <= 0 || options.RefreshLifetime <= 0 || options.LoginWindow <= 0 || options.LoginBlockDuration <= 0 || options.MaxLoginFailures < 1 {
		return nil, fmt.Errorf("authentication durations and failure limit must be positive")
	}
	dummy, err := hashPassword("dummy-password-never-used", options.PasswordParameters, options.Random)
	if err != nil {
		return nil, fmt.Errorf("create dummy password hash: %w", err)
	}
	if err := shortenExistingLoginBlocks(database, options.LoginBlockDuration); err != nil {
		return nil, fmt.Errorf("update existing login block policy: %w", err)
	}
	return &Service{
		database: database, now: options.Now, random: options.Random,
		passwordParameters: options.PasswordParameters,
		accessLifetime:     options.AccessLifetime, refreshLifetime: options.RefreshLifetime,
		loginWindow: options.LoginWindow, loginBlockDuration: options.LoginBlockDuration,
		maxLoginFailures: options.MaxLoginFailures, dummyPasswordHash: dummy,
	}, nil
}

// Apply a shorter configured cooldown to blocks persisted by an earlier
// server version while preserving their failure counts.
func shortenExistingLoginBlocks(database *sql.DB, duration time.Duration) error {
	rows, err := database.Query("SELECT email, updated_at, blocked_until FROM auth_login_failures WHERE blocked_until IS NOT NULL")
	if err != nil {
		return err
	}
	type loginBlock struct {
		email string
		until time.Time
	}
	var updates []loginBlock
	for rows.Next() {
		var email, updatedAt, blockedUntil string
		if err := rows.Scan(&email, &updatedAt, &blockedUntil); err != nil {
			rows.Close()
			return err
		}
		updated, err := parseTime(updatedAt)
		if err != nil {
			rows.Close()
			return err
		}
		blocked, err := parseTime(blockedUntil)
		if err != nil {
			rows.Close()
			return err
		}
		shortened := updated.Add(duration)
		if blocked.After(shortened) {
			updates = append(updates, loginBlock{email: email, until: shortened})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(updates) == 0 {
		return nil
	}
	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, update := range updates {
		if _, err := tx.Exec("UPDATE auth_login_failures SET blocked_until=? WHERE email=?", formatTime(update.until), update.email); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (service *Service) Register(ctx context.Context, email, password string) (Parent, error) {
	canonical, err := canonicalEmail(email)
	if err != nil {
		return Parent{}, err
	}
	hash, err := hashPassword(password, service.passwordParameters, service.random)
	if err != nil {
		return Parent{}, err
	}
	id, err := service.randomID("par_", 16)
	if err != nil {
		return Parent{}, err
	}
	now := formatTime(service.now())
	_, err = service.database.ExecContext(ctx,
		"INSERT INTO parent_accounts(id, email, password_hash, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		id, canonical, hash, now, now,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return Parent{}, ErrEmailExists
		}
		return Parent{}, fmt.Errorf("create parent account: %w", err)
	}
	return Parent{ID: id, Email: canonical}, nil
}

func (service *Service) Login(ctx context.Context, email, password string) (Parent, TokenPair, error) {
	canonical, err := canonicalEmail(email)
	if err != nil {
		return Parent{}, TokenPair{}, ErrInvalidCredentials
	}
	now := service.now().UTC()
	blocked, err := service.isLoginBlocked(ctx, canonical, now)
	if err != nil {
		return Parent{}, TokenPair{}, err
	}
	if blocked {
		return Parent{}, TokenPair{}, ErrLoginRateLimited
	}

	var parent Parent
	var encodedHash string
	err = service.database.QueryRowContext(ctx,
		"SELECT id, email, password_hash FROM parent_accounts WHERE email = ?", canonical,
	).Scan(&parent.ID, &parent.Email, &encodedHash)
	accountExists := err == nil
	if errors.Is(err, sql.ErrNoRows) {
		encodedHash = service.dummyPasswordHash
	} else if err != nil {
		return Parent{}, TokenPair{}, fmt.Errorf("load parent account: %w", err)
	}
	valid, verifyErr := verifyPassword(encodedHash, password)
	if verifyErr != nil {
		return Parent{}, TokenPair{}, fmt.Errorf("verify password: %w", verifyErr)
	}
	if !accountExists || !valid {
		if err := service.recordLoginFailure(ctx, canonical, now); err != nil {
			return Parent{}, TokenPair{}, err
		}
		return Parent{}, TokenPair{}, ErrInvalidCredentials
	}
	if _, err := service.database.ExecContext(ctx, "DELETE FROM auth_login_failures WHERE email = ?", canonical); err != nil {
		return Parent{}, TokenPair{}, fmt.Errorf("clear login failures: %w", err)
	}
	pair, err := service.createSession(ctx, parent.ID, now, encodedHash)
	if err != nil {
		return Parent{}, TokenPair{}, err
	}
	return parent, pair, nil
}

func (service *Service) Refresh(ctx context.Context, refreshToken string) (TokenPair, error) {
	if !validTokenShape(refreshToken, "rt_") {
		return TokenPair{}, ErrInvalidToken
	}
	now := service.now().UTC()
	oldHash := tokenHash(refreshToken)
	newRefresh, newRefreshHash, err := service.newToken("rt_")
	if err != nil {
		return TokenPair{}, err
	}
	newAccess, newAccessHash, err := service.newToken("at_")
	if err != nil {
		return TokenPair{}, err
	}
	pair := TokenPair{
		AccessToken: newAccess, AccessExpiresAt: now.Add(service.accessLifetime),
		RefreshToken: newRefresh, RefreshExpiresAt: now.Add(service.refreshLifetime),
	}
	reuseDetected := false
	resultErr := storage.WithinTransaction(ctx, service.database, func(transaction *sql.Tx) error {
		var familyID, accountID, tokenExpiryText, familyExpiryText string
		var consumedAt, revokedAt sql.NullString
		err := transaction.QueryRowContext(ctx, `
			SELECT r.family_id, f.parent_account_id, r.expires_at, r.consumed_at, f.expires_at, f.revoked_at
			FROM auth_refresh_tokens r JOIN auth_session_families f ON f.id = r.family_id
			WHERE r.token_hash = ?`, oldHash[:],
		).Scan(&familyID, &accountID, &tokenExpiryText, &consumedAt, &familyExpiryText, &revokedAt)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidToken
		}
		if err != nil {
			return fmt.Errorf("load refresh token: %w", err)
		}
		if consumedAt.Valid {
			if err := revokeFamily(ctx, transaction, familyID, now, "refresh_reuse"); err != nil {
				return err
			}
			reuseDetected = true
			return nil
		}
		tokenExpiry, err := parseTime(tokenExpiryText)
		if err != nil {
			return err
		}
		familyExpiry, err := parseTime(familyExpiryText)
		if err != nil {
			return err
		}
		if revokedAt.Valid || !now.Before(tokenExpiry) || !now.Before(familyExpiry) {
			return ErrInvalidToken
		}
		if pair.RefreshExpiresAt.After(familyExpiry) {
			if _, err := transaction.ExecContext(ctx,
				"UPDATE auth_session_families SET expires_at = ? WHERE id = ? AND revoked_at IS NULL",
				formatTime(pair.RefreshExpiresAt), familyID,
			); err != nil {
				return fmt.Errorf("extend session family: %w", err)
			}
		}
		if pair.AccessExpiresAt.After(pair.RefreshExpiresAt) {
			pair.AccessExpiresAt = pair.RefreshExpiresAt
		}
		result, err := transaction.ExecContext(ctx,
			"UPDATE auth_refresh_tokens SET consumed_at = ?, replaced_by_hash = ? WHERE token_hash = ? AND consumed_at IS NULL",
			formatTime(now), newRefreshHash[:], oldHash[:],
		)
		if err != nil {
			return fmt.Errorf("consume refresh token: %w", err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("read refresh update result: %w", err)
		}
		if rows != 1 {
			if err := revokeFamily(ctx, transaction, familyID, now, "refresh_reuse"); err != nil {
				return err
			}
			reuseDetected = true
			return nil
		}
		if _, err := transaction.ExecContext(ctx,
			"INSERT INTO auth_refresh_tokens(token_hash, family_id, issued_at, expires_at) VALUES (?, ?, ?, ?)",
			newRefreshHash[:], familyID, formatTime(now), formatTime(pair.RefreshExpiresAt),
		); err != nil {
			return fmt.Errorf("insert rotated refresh token: %w", err)
		}
		if _, err := transaction.ExecContext(ctx,
			"INSERT INTO auth_access_tokens(token_hash, family_id, parent_account_id, issued_at, expires_at) VALUES (?, ?, ?, ?, ?)",
			newAccessHash[:], familyID, accountID, formatTime(now), formatTime(pair.AccessExpiresAt),
		); err != nil {
			return fmt.Errorf("insert rotated access token: %w", err)
		}
		return nil
	})
	if reuseDetected {
		return TokenPair{}, ErrRefreshReuse
	}
	if resultErr != nil {
		return TokenPair{}, resultErr
	}
	return pair, nil
}

func (service *Service) Authenticate(ctx context.Context, accessToken string) (Parent, string, error) {
	if !validTokenShape(accessToken, "at_") {
		return Parent{}, "", ErrInvalidToken
	}
	hash := tokenHash(accessToken)
	var parent Parent
	var familyID, tokenExpiry, familyExpiry string
	var tokenRevoked, familyRevoked sql.NullString
	err := service.database.QueryRowContext(ctx, `
		SELECT p.id, p.email, a.family_id, a.expires_at, a.revoked_at, f.expires_at, f.revoked_at
		FROM auth_access_tokens a
		JOIN auth_session_families f ON f.id = a.family_id
		JOIN parent_accounts p ON p.id = a.parent_account_id
		WHERE a.token_hash = ?`, hash[:],
	).Scan(&parent.ID, &parent.Email, &familyID, &tokenExpiry, &tokenRevoked, &familyExpiry, &familyRevoked)
	if errors.Is(err, sql.ErrNoRows) {
		return Parent{}, "", ErrInvalidToken
	}
	if err != nil {
		return Parent{}, "", fmt.Errorf("load access token: %w", err)
	}
	tokenExpiresAt, err := parseTime(tokenExpiry)
	if err != nil {
		return Parent{}, "", err
	}
	familyExpiresAt, err := parseTime(familyExpiry)
	if err != nil {
		return Parent{}, "", err
	}
	now := service.now().UTC()
	if tokenRevoked.Valid || familyRevoked.Valid || !now.Before(tokenExpiresAt) || !now.Before(familyExpiresAt) {
		return Parent{}, "", ErrInvalidToken
	}
	return parent, familyID, nil
}

func (service *Service) Logout(ctx context.Context, accessToken string) error {
	_, familyID, err := service.Authenticate(ctx, accessToken)
	if err != nil {
		return err
	}
	return storage.WithinTransaction(ctx, service.database, func(transaction *sql.Tx) error {
		return revokeFamily(ctx, transaction, familyID, service.now().UTC(), "logout")
	})
}

func (service *Service) createSession(ctx context.Context, accountID string, now time.Time, verifiedHash string) (TokenPair, error) {
	familyID, err := service.randomID("fam_", 16)
	if err != nil {
		return TokenPair{}, err
	}
	access, accessHash, err := service.newToken("at_")
	if err != nil {
		return TokenPair{}, err
	}
	refresh, refreshHash, err := service.newToken("rt_")
	if err != nil {
		return TokenPair{}, err
	}
	pair := TokenPair{
		AccessToken: access, AccessExpiresAt: now.Add(service.accessLifetime),
		RefreshToken: refresh, RefreshExpiresAt: now.Add(service.refreshLifetime),
	}
	err = storage.WithinTransaction(ctx, service.database, func(transaction *sql.Tx) error {
		var currentHash string
		if err := transaction.QueryRowContext(ctx, "SELECT password_hash FROM parent_accounts WHERE id=?", accountID).Scan(&currentHash); err != nil {
			return err
		}
		if currentHash != verifiedHash {
			return ErrInvalidCredentials
		}
		if _, err := transaction.ExecContext(ctx,
			"INSERT INTO auth_session_families(id, parent_account_id, created_at, expires_at) VALUES (?, ?, ?, ?)",
			familyID, accountID, formatTime(now), formatTime(pair.RefreshExpiresAt),
		); err != nil {
			return fmt.Errorf("create session family: %w", err)
		}
		if _, err := transaction.ExecContext(ctx,
			"INSERT INTO auth_refresh_tokens(token_hash, family_id, issued_at, expires_at) VALUES (?, ?, ?, ?)",
			refreshHash[:], familyID, formatTime(now), formatTime(pair.RefreshExpiresAt),
		); err != nil {
			return fmt.Errorf("create refresh token: %w", err)
		}
		if _, err := transaction.ExecContext(ctx,
			"INSERT INTO auth_access_tokens(token_hash, family_id, parent_account_id, issued_at, expires_at) VALUES (?, ?, ?, ?, ?)",
			accessHash[:], familyID, accountID, formatTime(now), formatTime(pair.AccessExpiresAt),
		); err != nil {
			return fmt.Errorf("create access token: %w", err)
		}
		return nil
	})
	return pair, err
}

func revokeFamily(ctx context.Context, transaction *sql.Tx, familyID string, now time.Time, reason string) error {
	stamp := formatTime(now)
	if _, err := transaction.ExecContext(ctx,
		"UPDATE auth_session_families SET revoked_at = COALESCE(revoked_at, ?), revocation_reason = COALESCE(revocation_reason, ?) WHERE id = ?",
		stamp, reason, familyID,
	); err != nil {
		return fmt.Errorf("revoke session family: %w", err)
	}
	if _, err := transaction.ExecContext(ctx,
		"UPDATE auth_access_tokens SET revoked_at = COALESCE(revoked_at, ?) WHERE family_id = ?", stamp, familyID,
	); err != nil {
		return fmt.Errorf("revoke family access tokens: %w", err)
	}
	return nil
}

func (service *Service) isLoginBlocked(ctx context.Context, email string, now time.Time) (bool, error) {
	var blockedUntil sql.NullString
	err := service.database.QueryRowContext(ctx, "SELECT blocked_until FROM auth_login_failures WHERE email = ?", email).Scan(&blockedUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load login failures: %w", err)
	}
	if !blockedUntil.Valid {
		return false, nil
	}
	blocked, err := parseTime(blockedUntil.String)
	if err != nil {
		return false, err
	}
	return now.Before(blocked), nil
}

func (service *Service) recordLoginFailure(ctx context.Context, email string, now time.Time) error {
	return storage.WithinTransaction(ctx, service.database, func(transaction *sql.Tx) error {
		var count int
		var startedText string
		err := transaction.QueryRowContext(ctx,
			"SELECT failure_count, window_started_at FROM auth_login_failures WHERE email = ?", email,
		).Scan(&count, &startedText)
		if errors.Is(err, sql.ErrNoRows) {
			count = 0
			startedText = formatTime(now)
		} else if err != nil {
			return fmt.Errorf("load login failure window: %w", err)
		}
		started, err := parseTime(startedText)
		if err != nil {
			return err
		}
		if now.Sub(started) >= service.loginWindow {
			count = 0
			started = now
		}
		count++
		var blockedUntil any
		if count >= service.maxLoginFailures {
			blockedUntil = formatTime(now.Add(service.loginBlockDuration))
		}
		_, err = transaction.ExecContext(ctx, `
			INSERT INTO auth_login_failures(email, failure_count, window_started_at, blocked_until, updated_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(email) DO UPDATE SET failure_count=excluded.failure_count,
			window_started_at=excluded.window_started_at, blocked_until=excluded.blocked_until, updated_at=excluded.updated_at`,
			email, count, formatTime(started), blockedUntil, formatTime(now),
		)
		if err != nil {
			return fmt.Errorf("record login failure: %w", err)
		}
		return nil
	})
}

func canonicalEmail(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) < 3 || len(value) > 254 {
		return "", fmt.Errorf("valid email is required")
	}
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address != value || strings.ContainsAny(value, "\r\n") {
		return "", fmt.Errorf("valid email is required")
	}
	return value, nil
}

func (service *Service) randomID(prefix string, byteCount int) (string, error) {
	value := make([]byte, byteCount)
	if _, err := io.ReadFull(service.random, value); err != nil {
		return "", fmt.Errorf("generate identifier: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(value), nil
}

func (service *Service) newToken(prefix string) (string, [32]byte, error) {
	token, err := service.randomID(prefix, 32)
	if err != nil {
		return "", [32]byte{}, err
	}
	return token, tokenHash(token), nil
}

func tokenHash(token string) [32]byte { return sha256.Sum256([]byte(token)) }

func validTokenShape(token, prefix string) bool {
	if !strings.HasPrefix(token, prefix) || len(token) != len(prefix)+43 {
		return false
	}
	_, err := base64.RawURLEncoding.Strict().DecodeString(strings.TrimPrefix(token, prefix))
	return err == nil
}

func formatTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse stored timestamp: %w", err)
	}
	return parsed, nil
}
