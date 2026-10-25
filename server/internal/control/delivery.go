package control

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"appcontrol/server/internal/storage"
)

// Authorization is checked inside each state-changing transaction, so logout,
// session replacement and revocation cannot race a cached authorization result.
func deviceAuthorization(ctx context.Context, tx *sql.Tx, token string, now time.Time) (string, error) {
	if len(token) != 46 || token[:3] != "dt_" {
		return "", ErrUnauthorized
	}
	hash := sha256.Sum256([]byte(token))
	var deviceID, expiry string
	err := tx.QueryRowContext(ctx, `SELECT d.id,s.expires_at FROM device_sessions s JOIN devices d ON d.id=s.device_id
  WHERE s.token_hash=? AND d.state='active'`, hash[:]).Scan(&deviceID, &expiry)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrUnauthorized
	}
	if err != nil {
		return "", err
	}
	deadline, err := time.Parse(time.RFC3339Nano, expiry)
	if err != nil {
		return "", err
	}
	if !now.Before(deadline) {
		return "", ErrUnauthorized
	}
	return deviceID, nil
}

// Compare parsed timestamps rather than RFC3339 strings with differing precision.
func expireCommands(ctx context.Context, tx *sql.Tx, deviceID string, now time.Time) error {
	rows, err := tx.QueryContext(ctx, "SELECT id,expires_at FROM commands WHERE device_id=? AND status IN ('queued','delivered')", deviceID)
	if err != nil {
		return err
	}
	var expired []string
	for rows.Next() {
		var id, expiry string
		if err = rows.Scan(&id, &expiry); err != nil {
			rows.Close()
			return err
		}
		deadline, parseErr := time.Parse(time.RFC3339Nano, expiry)
		if parseErr != nil {
			rows.Close()
			return parseErr
		}
		if !now.Before(deadline) {
			expired = append(expired, id)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range expired {
		if _, err = tx.ExecContext(ctx, "UPDATE commands SET status='expired',status_at=? WHERE id=?", now.Format(time.RFC3339Nano), id); err != nil {
			return err
		}
	}
	return nil
}

// Fetch returns pending commands again until a terminal acknowledgement commits.
// Marking delivered means an HTTP delivery attempt, never execution success.
func (s *Service) Fetch(ctx context.Context, token string) ([]Command, error) {
	commands := make([]Command, 0)
	err := storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		now := s.now().UTC()
		deviceID, err := deviceAuthorization(ctx, tx, token, now)
		if err != nil {
			return err
		}
		if !s.remoteCallsEnabled {
			if _, err = tx.ExecContext(ctx, `UPDATE commands SET status='expired',status_at=?
				WHERE device_id=? AND type IN ('ANSWER_CALL','REJECT_CALL','END_CALL')
				AND status IN ('queued','delivered')`, now.Format(time.RFC3339Nano), deviceID); err != nil {
				return err
			}
		}
		if err = expireCommands(ctx, tx, deviceID, now); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, `SELECT id,type,payload,issued_at,expires_at FROM commands
   WHERE device_id=? AND status IN ('queued','delivered') ORDER BY julianday(issued_at),id LIMIT 50`, deviceID)
		if err != nil {
			return err
		}
		for rows.Next() {
			c := Command{Version: 1, DeviceID: deviceID, Status: "delivered"}
			var payload, issued, expires string
			if err = rows.Scan(&c.ID, &c.Type, &payload, &issued, &expires); err != nil {
				rows.Close()
				return err
			}
			c.Payload = json.RawMessage(payload)
			c.IssuedAt, err = time.Parse(time.RFC3339Nano, issued)
			if err != nil {
				rows.Close()
				return err
			}
			c.ExpiresAt, err = time.Parse(time.RFC3339Nano, expires)
			if err != nil {
				rows.Close()
				return err
			}
			commands = append(commands, c)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, c := range commands {
			if _, err = tx.ExecContext(ctx, "UPDATE commands SET status='delivered',status_at=? WHERE id=? AND status='queued'", now.Format(time.RFC3339Nano), c.ID); err != nil {
				return err
			}
		}
		return nil
	})
	return commands, err
}

func (s *Service) Acknowledge(ctx context.Context, token, commandID, result string) error {
	if result != "succeeded" && result != "failed" {
		return ErrInvalid
	}
	var outcome error
	err := storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		now := s.now().UTC()
		deviceID, err := deviceAuthorization(ctx, tx, token, now)
		if err != nil {
			return err
		}
		if err = expireCommands(ctx, tx, deviceID, now); err != nil {
			return err
		}
		var status string
		err = tx.QueryRowContext(ctx, "SELECT status FROM commands WHERE id=? AND device_id=?", commandID, deviceID).Scan(&status)
		if errors.Is(err, sql.ErrNoRows) {
			outcome = ErrNotFound
			return nil
		}
		if err != nil {
			return err
		}
		if status == result {
			return nil
		} // A lost HTTP response can be retried safely.
		if status != "delivered" {
			outcome = ErrCommandState
			return nil
		}
		_, err = tx.ExecContext(ctx, "UPDATE commands SET status=?,status_at=? WHERE id=?", result, now.Format(time.RFC3339Nano), commandID)
		return err
	})
	if err != nil {
		return err
	}
	return outcome
}

// DevicePolicy defaults to the original format for legacy clients.
func (s *Service) DevicePolicy(ctx context.Context, token string) (Policy, error) {
	return s.DevicePolicyWithFormat(ctx, token, 1)
}

func (s *Service) DevicePolicyWithFormat(ctx context.Context, token string, maxFormat int) (Policy, error) {
	var policy Policy
	err := storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		deviceID, err := deviceAuthorization(ctx, tx, token, s.now().UTC())
		if err != nil {
			return err
		}
		policy, err = readPolicy(ctx, tx, deviceID)
		if err != nil {
			return err
		}
		if maxFormat < policy.FormatVersion {
			return ErrPolicyFormat
		}
		return nil
	})
	return policy, err
}
