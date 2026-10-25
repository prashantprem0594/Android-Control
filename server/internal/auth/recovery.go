package auth

import (
	"appcontrol/server/internal/storage"
	"context"
	"database/sql"
	"errors"
)

// RecoverPassword is an operator-only local administration operation. There is
// deliberately no unauthenticated HTTP password-recovery route.
func (service *Service) RecoverPassword(ctx context.Context, email, password string) error {
	canonical, err := canonicalEmail(email)
	if err != nil {
		return err
	}
	hash, err := hashPassword(password, service.passwordParameters, service.random)
	if err != nil {
		return err
	}
	return storage.WithinTransaction(ctx, service.database, func(tx *sql.Tx) error {
		var id string
		if err := tx.QueryRowContext(ctx, "SELECT id FROM parent_accounts WHERE email=?", canonical).Scan(&id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrInvalidCredentials
			}
			return err
		}
		now := formatTime(service.now().UTC())
		if _, err := tx.ExecContext(ctx, "UPDATE parent_accounts SET password_hash=?,updated_at=? WHERE id=?", hash, now, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE auth_session_families SET revoked_at=?,revocation_reason='password_reset' WHERE parent_account_id=? AND revoked_at IS NULL", now, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE pairing_codes SET expires_at=? WHERE parent_account_id=? AND consumed_at IS NULL", now, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE devices SET state='revoked',revoked_at=?,revocation_reason='password_reset' WHERE parent_account_id=? AND state='pending'", now, id); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "DELETE FROM auth_login_failures WHERE email=?", canonical)
		return err
	})
}
