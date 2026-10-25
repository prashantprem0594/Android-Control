package control

import (
	"appcontrol/server/internal/storage"
	"context"
	"database/sql"
	"errors"
	"time"
)

type Capabilities struct {
	AndroidAPI  int  `json:"androidApi"`
	DeviceOwner bool `json:"deviceOwner"`
	DialerRole  bool `json:"dialerRole"`
	VPNConsent  bool `json:"vpnConsent"`
	ExactAlarms bool `json:"exactAlarms"`
}
type DeviceStatus struct {
	ServerTime   time.Time     `json:"serverTime"`
	ReceivedAt   *time.Time    `json:"receivedAt,omitempty"`
	Capabilities *Capabilities `json:"capabilities,omitempty"`
}

func (s *Service) ReportStatus(ctx context.Context, token string, capabilities Capabilities) error {
	if capabilities.AndroidAPI < 33 || capabilities.AndroidAPI > 100 {
		return ErrInvalid
	}
	return storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		now := s.now().UTC()
		id, err := deviceAuthorization(ctx, tx, token, now)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO device_status(device_id,parent_account_id,received_at,android_api,device_owner,dialer_role,vpn_consent,exact_alarms)
		 SELECT id,parent_account_id,?,?,?,?,?,? FROM devices WHERE id=?
		 ON CONFLICT(device_id) DO UPDATE SET received_at=excluded.received_at,android_api=excluded.android_api,
		 device_owner=excluded.device_owner,dialer_role=excluded.dialer_role,vpn_consent=excluded.vpn_consent,exact_alarms=excluded.exact_alarms`,
			now.Format(time.RFC3339Nano), capabilities.AndroidAPI, capabilities.DeviceOwner, capabilities.DialerRole, capabilities.VPNConsent, capabilities.ExactAlarms, id)
		return err
	})
}
func (s *Service) Status(ctx context.Context, parentID, deviceID string) (DeviceStatus, error) {
	status := DeviceStatus{ServerTime: s.now().UTC()}
	err := storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		if err := owned(ctx, tx, parentID, deviceID); err != nil {
			return err
		}
		var received string
		var capabilities Capabilities
		err := tx.QueryRowContext(ctx, `SELECT received_at,android_api,device_owner,dialer_role,vpn_consent,exact_alarms
		 FROM device_status WHERE device_id=? AND parent_account_id=?`, deviceID, parentID).Scan(&received, &capabilities.AndroidAPI, &capabilities.DeviceOwner, &capabilities.DialerRole, &capabilities.VPNConsent, &capabilities.ExactAlarms)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		when, err := time.Parse(time.RFC3339Nano, received)
		if err != nil {
			return err
		}
		status.ReceivedAt = &when
		status.Capabilities = &capabilities
		return nil
	})
	return status, err
}
