package control

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"appcontrol/server/internal/storage"
)

type PolicyReport struct {
	Version    int64     `json:"version"`
	Result     string    `json:"result"`
	Reason     string    `json:"reason,omitempty"`
	ReportedAt time.Time `json:"reportedAt"`
	StateKey   string    `json:"stateKey,omitempty"`
}

func validPolicyReport(result, reason string) bool {
	if result == "applied" {
		return reason == ""
	}
	if result != "partial" && result != "failed" {
		return false
	}
	switch reason {
	case "device_owner_required", "protected_package", "package_unavailable", "package_change_failed", "internet_pause_unavailable", "interrupted":
		return true
	default:
		return false
	}
}

func (s *Service) ReportPolicy(ctx context.Context, token string, version int64, result, reason string) error {
	return s.ReportPolicyState(ctx, token, version, result, reason, "")
}

func (s *Service) ReportPolicyState(ctx context.Context, token string, version int64, result, reason, stateKey string) error {
	if version < 1 || !validPolicyReport(result, reason) {
		return ErrInvalid
	}
	if stateKey != "" {
		if len(stateKey) != 64 {
			return ErrInvalid
		}
		for _, c := range stateKey {
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
				return ErrInvalid
			}
		}
	}
	return storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		now := s.now().UTC()
		deviceID, err := deviceAuthorization(ctx, tx, token, now)
		if err != nil {
			return err
		}
		var desired int64
		var parentID string
		err = tx.QueryRowContext(ctx, "SELECT version,parent_account_id FROM device_policies WHERE device_id=?", deviceID).Scan(&desired, &parentID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrConflict
		}
		if err != nil {
			return err
		}
		if version != desired {
			return ErrConflict
		}
		policy, err := readPolicy(ctx, tx, deviceID)
		if err != nil {
			return err
		}
		if policy.FormatVersion > 1 && stateKey == "" {
			return ErrPolicyFormat
		}
		if stateKey != "" {
			effective, err := ResolvePolicy(policy, now)
			if err != nil {
				return err
			}
			if effective.StateKey != stateKey {
				return ErrConflict
			}
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO device_policy_reports(device_id,parent_account_id,version,result,reason,reported_at,state_key)
   VALUES(?,?,?,?,?,?,?) ON CONFLICT(device_id) DO UPDATE SET version=excluded.version,result=excluded.result,reason=excluded.reason,reported_at=excluded.reported_at,state_key=excluded.state_key
   WHERE device_policy_reports.version!=excluded.version OR device_policy_reports.result!=excluded.result OR device_policy_reports.reason!=excluded.reason OR device_policy_reports.state_key!=excluded.state_key`, deviceID, parentID, version, result, reason, now.Format(time.RFC3339Nano), stateKey)
		return err
	})
}

func readPolicyReport(ctx context.Context, tx *sql.Tx, deviceID string) (*PolicyReport, error) {
	var report PolicyReport
	var timestamp string
	err := tx.QueryRowContext(ctx, "SELECT version,result,reason,reported_at,state_key FROM device_policy_reports WHERE device_id=?", deviceID).Scan(&report.Version, &report.Result, &report.Reason, &timestamp, &report.StateKey)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	report.ReportedAt, err = time.Parse(time.RFC3339Nano, timestamp)
	return &report, err
}
