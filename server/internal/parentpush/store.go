package parentpush

import (
	"appcontrol/server/internal/storage"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"time"
)

var (
	ErrUnauthorized = errors.New("parent push registration is unavailable")
	ErrInvalid      = errors.New("invalid parent push request")
	ErrUnavailable  = errors.New("parent push registration is unavailable")
)

const (
	maxClaims          = 100
	maxEventCandidates = 20
	maxFanoutFamilies  = 50
	maxMaintenanceRows = 500
	maxAttempts        = 5
	eventFreshness     = 30 * time.Second
	leaseDuration      = 20 * time.Second
	eventRetention     = 7 * 24 * time.Hour
	maintenancePeriod  = 5 * time.Second
	encryptionAADScope = "app-control-parent-push-registration-v1"
)

type Store struct {
	db     *sql.DB
	cipher cipher.AEAD
	now    func() time.Time
}

type Delivery struct {
	ID                  string
	EventID             string
	ParentID            string
	FamilyID            string
	DeviceID            string
	CallID              string
	SnapshotSequence    int64
	RegistrationToken   []byte
	RegistrationVersion int64
	LeaseID             string
	Attempt             int
	ExpiresAt           time.Time
	LeaseExpiresAt      time.Time
}

func NewStore(db *sql.DB, key []byte) (*Store, error) {
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
	return &Store{db: db, cipher: aead, now: time.Now}, nil
}

func validateRegistration(token string) error {
	if len(token) < 1 || len(token) > 4096 {
		return ErrInvalid
	}
	for _, value := range token {
		if value < 33 || value > 126 {
			return ErrInvalid
		}
	}
	return nil
}

func registrationAAD(parentID, familyID string) []byte {
	result := []byte(encryptionAADScope)
	for _, value := range []string{parentID, familyID} {
		var length [4]byte
		binary.BigEndian.PutUint32(length[:], uint32(len(value)))
		result = append(result, length[:]...)
		result = append(result, value...)
	}
	return result
}

func (s *Store) Register(ctx context.Context, parentID, familyID, token string) error {
	if parentID == "" || familyID == "" || validateRegistration(token) != nil {
		return ErrInvalid
	}
	return storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		var storedParentID, expiry string
		var revoked sql.NullString
		err := tx.QueryRowContext(ctx, `SELECT parent_account_id,expires_at,revoked_at
			FROM auth_session_families WHERE id=?`, familyID).Scan(&storedParentID, &expiry, &revoked)
		if errors.Is(err, sql.ErrNoRows) || err == nil && (storedParentID != parentID || revoked.Valid) {
			return ErrUnauthorized
		}
		if err != nil {
			return err
		}
		deadline, err := time.Parse(time.RFC3339Nano, expiry)
		if err != nil {
			return err
		}
		now := s.now().UTC()
		if !now.Before(deadline) {
			return ErrUnauthorized
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO parent_wakeup_registration_versions(family_id,registration_version)
			VALUES(?,1) ON CONFLICT(family_id) DO UPDATE SET registration_version=registration_version+1`, familyID); err != nil {
			return err
		}
		var version int64
		if err := tx.QueryRowContext(ctx, `SELECT registration_version FROM parent_wakeup_registration_versions
			WHERE family_id=?`, familyID).Scan(&version); err != nil {
			return err
		}
		nonce := make([]byte, s.cipher.NonceSize())
		if _, err := rand.Read(nonce); err != nil {
			return ErrUnavailable
		}
		sealed := s.cipher.Seal(nonce, nonce, []byte(token), registrationAAD(parentID, familyID))
		_, err = tx.ExecContext(ctx, `INSERT INTO parent_wakeup_registrations
			(family_id,parent_account_id,token_ciphertext,registration_version,updated_at)
			VALUES(?,?,?,?,?)
			ON CONFLICT(family_id) DO UPDATE SET token_ciphertext=excluded.token_ciphertext,
			registration_version=excluded.registration_version,updated_at=excluded.updated_at
			WHERE parent_wakeup_registrations.parent_account_id=excluded.parent_account_id`,
			familyID, parentID, sealed, version, now.Format(time.RFC3339Nano))
		return err
	})
}

func (s *Store) Remove(ctx context.Context, parentID, familyID string) error {
	if parentID == "" || familyID == "" {
		return ErrInvalid
	}
	return storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		if err := s.requireActiveFamily(ctx, tx, parentID, familyID, s.now().UTC()); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM parent_wakeup_registrations
			WHERE family_id=? AND parent_account_id=?`, familyID, parentID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE parent_call_event_deliveries SET status='cancelled',
			lease_id=NULL,lease_expires_at_ns=NULL WHERE family_id=? AND status IN ('pending','leased')`, familyID)
		return err
	})
}

func (s *Store) requireActiveFamily(ctx context.Context, tx *sql.Tx, parentID, familyID string, now time.Time) error {
	var storedParentID, expiry string
	var revoked sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT parent_account_id,expires_at,revoked_at
		FROM auth_session_families WHERE id=?`, familyID).Scan(&storedParentID, &expiry, &revoked)
	if errors.Is(err, sql.ErrNoRows) || err == nil && (storedParentID != parentID || revoked.Valid) {
		return ErrUnauthorized
	}
	if err != nil {
		return err
	}
	deadline, err := time.Parse(time.RFC3339Nano, expiry)
	if err != nil {
		return err
	}
	if !now.Before(deadline) {
		return ErrUnauthorized
	}
	return nil
}

func validRingingSnapshot(raw, callID string, observed time.Time, now time.Time) bool {
	age := now.Sub(observed)
	if age < 0 || age > eventFreshness {
		return false
	}
	var calls []struct {
		CallID string `json:"callId"`
		State  string `json:"state"`
	}
	if json.Unmarshal([]byte(raw), &calls) != nil {
		return false
	}
	for _, call := range calls {
		if call.CallID == callID {
			return call.State == "RINGING"
		}
	}
	return false
}

type eventCandidate struct {
	id, deviceID, parentID, callID, observedAt, callsJSON string
	sequence, latestSequence, expiresAt                   int64
}

func maintainTx(ctx context.Context, tx *sql.Tx, now time.Time, limit int) error {
	nowNS := now.UnixNano()
	if _, err := tx.ExecContext(ctx, `UPDATE parent_call_events SET status='expired'
		WHERE event_id IN (
			SELECT event_id FROM parent_call_events
			WHERE status='active' AND expires_at_ns<=?
			ORDER BY expires_at_ns,event_id LIMIT ?
		)`, nowNS, limit); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE parent_call_event_deliveries
		SET status='expired',lease_id=NULL,lease_expires_at_ns=NULL
		WHERE delivery_id IN (
			SELECT d.delivery_id FROM parent_call_event_deliveries d
			JOIN parent_call_events e ON e.event_id=d.event_id
			WHERE e.status='expired' AND e.expires_at_ns<=?
			AND d.status IN ('pending','leased')
			ORDER BY e.expires_at_ns,e.event_id,d.delivery_id LIMIT ?
		)`, nowNS, limit); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM parent_call_event_deliveries
		WHERE delivery_id IN (
			SELECT d.delivery_id FROM parent_call_event_deliveries d
			JOIN parent_call_events e ON e.event_id=d.event_id
			WHERE e.status='expired' AND e.expires_at_ns<?
			ORDER BY e.expires_at_ns,e.event_id,d.delivery_id LIMIT ?
		)`, now.Add(-eventRetention).UnixNano(), limit); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM parent_call_events
		WHERE event_id IN (
			SELECT e.event_id FROM parent_call_events e
			WHERE e.status='expired' AND e.expires_at_ns<?
			AND NOT EXISTS(SELECT 1 FROM parent_call_event_deliveries d WHERE d.event_id=e.event_id)
			ORDER BY e.expires_at_ns,e.event_id LIMIT ?
		)`, now.Add(-eventRetention).UnixNano(), limit)
	return err
}

// Maintain expires stale events and removes retained events in bounded batches.
func Maintain(ctx context.Context, db *sql.DB, limit int) error {
	if db == nil || limit < 1 || limit > maxMaintenanceRows {
		return ErrInvalid
	}
	now := time.Now().UTC()
	return storage.WithinTransaction(ctx, db, func(tx *sql.Tx) error {
		return maintainTx(ctx, tx, now, limit)
	})
}

func RunMaintenance(ctx context.Context, db *sql.DB, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if err := Maintain(ctx, db, maxMaintenanceRows); err != nil && ctx.Err() == nil {
		logger.Error("parent push maintenance batch failed")
	}
	ticker := time.NewTicker(maintenancePeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
		if err := Maintain(ctx, db, maxMaintenanceRows); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			logger.Error("parent push maintenance batch failed")
		}
	}
}

func loadEventCandidates(ctx context.Context, tx *sql.Tx, after string, limit int) ([]eventCandidate, error) {
	readRange := func(comparison string, cursor string, count int) ([]eventCandidate, error) {
		rows, err := tx.QueryContext(ctx, `SELECT e.event_id,e.device_id,e.parent_account_id,e.call_id,
			e.snapshot_sequence,e.expires_at_ns,s.calls_json,s.observed_at,s.snapshot_sequence
			FROM parent_call_events e
			JOIN devices d ON d.id=e.device_id AND d.state='active' AND d.parent_account_id=e.parent_account_id
			JOIN remote_call_snapshots s ON s.device_id=e.device_id AND s.parent_account_id=e.parent_account_id
			WHERE e.status='active' AND e.event_id `+comparison+` ?
			ORDER BY e.event_id LIMIT ?`, cursor, count)
		if err != nil {
			return nil, err
		}
		var candidates []eventCandidate
		for rows.Next() {
			var item eventCandidate
			var observed string
			if err := rows.Scan(&item.id, &item.deviceID, &item.parentID, &item.callID,
				&item.sequence, &item.expiresAt, &item.callsJSON, &observed, &item.latestSequence); err != nil {
				rows.Close()
				return nil, err
			}
			parsed, err := time.Parse(time.RFC3339Nano, observed)
			if err != nil {
				rows.Close()
				return nil, err
			}
			item.observedAt = parsed.Format(time.RFC3339Nano)
			candidates = append(candidates, item)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
		return candidates, nil
	}
	candidates, err := readRange(">", after, limit)
	if err != nil {
		return nil, err
	}
	if after != "" && len(candidates) < limit {
		wrapped, err := readRange("<=", after, limit-len(candidates))
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, wrapped...)
	}
	return candidates, nil
}

func (s *Store) fanoutEvent(ctx context.Context, tx *sql.Tx, event eventCandidate, now time.Time) error {
	observed, err := time.Parse(time.RFC3339Nano, event.observedAt)
	if err != nil {
		return err
	}
	if event.latestSequence < event.sequence || now.UnixNano() >= event.expiresAt ||
		!validRingingSnapshot(event.callsJSON, event.callID, observed, now) {
		if _, err := tx.ExecContext(ctx, `UPDATE parent_call_events SET status='expired' WHERE event_id=?`, event.id); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE parent_call_event_deliveries
			SET status='expired',lease_id=NULL,lease_expires_at_ns=NULL
			WHERE event_id=? AND status IN ('pending','leased')`, event.id)
		return err
	}

	var afterFamily string
	if err := tx.QueryRowContext(ctx, `SELECT last_family_id FROM parent_call_event_fanout_cursors WHERE event_id=?`,
		event.id).Scan(&afterFamily); errors.Is(err, sql.ErrNoRows) {
		afterFamily = ""
	} else if err != nil {
		return err
	}
	families, err := tx.QueryContext(ctx, `SELECT r.family_id,r.registration_version
		FROM parent_wakeup_registrations r JOIN auth_session_families f ON f.id=r.family_id
		WHERE r.parent_account_id=? AND f.parent_account_id=? AND f.revoked_at IS NULL
		AND f.expires_at>? AND r.family_id>?
		ORDER BY r.family_id LIMIT ?`, event.parentID, event.parentID,
		now.Format(time.RFC3339Nano), afterFamily, maxFanoutFamilies)
	if err != nil {
		return err
	}
	type familyRegistration struct {
		id      string
		version int64
	}
	var eligible []familyRegistration
	for families.Next() {
		var item familyRegistration
		if err := families.Scan(&item.id, &item.version); err != nil {
			families.Close()
			return err
		}
		eligible = append(eligible, item)
	}
	if err := families.Err(); err != nil {
		families.Close()
		return err
	}
	if err := families.Close(); err != nil {
		return err
	}
	for _, family := range eligible {
		id, err := randomID()
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO parent_call_event_deliveries
			(delivery_id,event_id,family_id,status,attempts,next_attempt_at_ns,registration_version,created_at_ns)
			VALUES(?,?,?,'pending',0,?,?,?)`, id, event.id, family.id, now.UnixNano(), family.version, now.UnixNano()); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE parent_call_event_deliveries SET registration_version=?
			WHERE event_id=? AND family_id=? AND status='pending' AND attempts=0`,
			family.version, event.id, family.id); err != nil {
			return err
		}
	}
	nextFamily := ""
	if len(eligible) == maxFanoutFamilies {
		nextFamily = eligible[len(eligible)-1].id
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO parent_call_event_fanout_cursors(event_id,last_family_id)
		VALUES(?,?) ON CONFLICT(event_id) DO UPDATE SET last_family_id=excluded.last_family_id`,
		event.id, nextFamily)
	return err
}

func (s *Store) Claim(ctx context.Context, limit int) ([]Delivery, error) {
	if limit < 1 || limit > maxClaims {
		return nil, ErrInvalid
	}
	now := s.now().UTC()
	nowNS := now.UnixNano()
	var deliveries []Delivery
	err := storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		if err := maintainTx(ctx, tx, now, maxMaintenanceRows); err != nil {
			return err
		}

		var afterEvent string
		if err := tx.QueryRowContext(ctx, `SELECT last_event_id FROM parent_push_processing_cursors WHERE name='fanout'`).Scan(&afterEvent); errors.Is(err, sql.ErrNoRows) {
			afterEvent = ""
		} else if err != nil {
			return err
		}
		candidates, err := loadEventCandidates(ctx, tx, afterEvent, maxEventCandidates)
		if err != nil {
			return err
		}
		for _, event := range candidates {
			if err := s.fanoutEvent(ctx, tx, event, now); err != nil {
				return err
			}
		}
		if len(candidates) != 0 {
			if _, err := tx.ExecContext(ctx, `INSERT INTO parent_push_processing_cursors(name,last_event_id)
				VALUES('fanout',?) ON CONFLICT(name) DO UPDATE SET last_event_id=excluded.last_event_id`,
				candidates[len(candidates)-1].id); err != nil {
				return err
			}
		}

		// Expired leases have consumed their attempt. A crashed fifth attempt is terminal.
		if _, err := tx.ExecContext(ctx, `UPDATE parent_call_event_deliveries
			SET status='failed',lease_id=NULL,lease_expires_at_ns=NULL
			WHERE status='leased' AND lease_expires_at_ns<=? AND attempts>=?`, nowNS, maxAttempts); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, `SELECT d.delivery_id,e.event_id,e.parent_account_id,e.device_id,e.call_id,
			e.snapshot_sequence,d.family_id,d.attempts,r.token_ciphertext,r.registration_version,
			s.calls_json,s.observed_at,e.expires_at_ns
			FROM parent_call_event_deliveries d
			JOIN parent_call_events e ON e.event_id=d.event_id AND e.status='active'
			JOIN devices v ON v.id=e.device_id AND v.state='active' AND v.parent_account_id=e.parent_account_id
			JOIN auth_session_families f ON f.id=d.family_id AND f.parent_account_id=e.parent_account_id AND f.revoked_at IS NULL
			JOIN parent_wakeup_registrations r ON r.family_id=d.family_id AND r.parent_account_id=e.parent_account_id
			JOIN remote_call_snapshots s ON s.device_id=e.device_id AND s.parent_account_id=e.parent_account_id
			WHERE d.attempts<? AND d.next_attempt_at_ns<=? AND e.expires_at_ns>?
			AND (d.status='pending' OR d.status='leased' AND d.lease_expires_at_ns<=?)
			ORDER BY e.created_at_ns,d.created_at_ns LIMIT ?`,
			maxAttempts, nowNS, nowNS, nowNS, limit)
		if err != nil {
			return err
		}
		type claimCandidate struct {
			delivery            Delivery
			ciphertext          []byte
			callsJSON, observed string
			expiresAt           int64
		}
		var claimCandidates []claimCandidate
		for rows.Next() {
			var item claimCandidate
			if err := rows.Scan(&item.delivery.ID, &item.delivery.EventID, &item.delivery.ParentID, &item.delivery.DeviceID,
				&item.delivery.CallID, &item.delivery.SnapshotSequence, &item.delivery.FamilyID, &item.delivery.Attempt,
				&item.ciphertext, &item.delivery.RegistrationVersion, &item.callsJSON, &item.observed, &item.expiresAt); err != nil {
				rows.Close()
				return err
			}
			claimCandidates = append(claimCandidates, item)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}

		for _, item := range claimCandidates {
			observed, err := time.Parse(time.RFC3339Nano, item.observed)
			if err != nil {
				return err
			}
			if nowNS >= item.expiresAt || !validRingingSnapshot(item.callsJSON, item.delivery.CallID, observed, now) {
				if _, err := tx.ExecContext(ctx, `UPDATE parent_call_events SET status='expired' WHERE event_id=?`, item.delivery.EventID); err != nil {
					return err
				}
				if _, err := tx.ExecContext(ctx, `UPDATE parent_call_event_deliveries SET status='expired',lease_id=NULL,lease_expires_at_ns=NULL
					WHERE event_id=? AND status IN ('pending','leased')`, item.delivery.EventID); err != nil {
					return err
				}
				continue
			}
			var familyExpiry string
			var revoked sql.NullString
			if err := tx.QueryRowContext(ctx, `SELECT expires_at,revoked_at FROM auth_session_families
				WHERE id=? AND parent_account_id=?`, item.delivery.FamilyID, item.delivery.ParentID).Scan(&familyExpiry, &revoked); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					continue
				}
				return err
			}
			deadline, err := time.Parse(time.RFC3339Nano, familyExpiry)
			if err != nil {
				return err
			}
			if revoked.Valid || !now.Before(deadline) {
				if _, err := tx.ExecContext(ctx, `UPDATE parent_call_event_deliveries SET status='cancelled'
					WHERE delivery_id=? AND status IN ('pending','leased')`, item.delivery.ID); err != nil {
					return err
				}
				continue
			}
			nonceSize := s.cipher.NonceSize()
			if len(item.ciphertext) < nonceSize+s.cipher.Overhead()+1 || len(item.ciphertext) > nonceSize+s.cipher.Overhead()+4096 {
				return ErrUnavailable
			}
			plain, err := s.cipher.Open(nil, item.ciphertext[:nonceSize], item.ciphertext[nonceSize:],
				registrationAAD(item.delivery.ParentID, item.delivery.FamilyID))
			if err != nil {
				return ErrUnavailable
			}
			leaseID, err := randomID()
			if err != nil {
				clear(plain)
				return err
			}
			result, err := tx.ExecContext(ctx, `UPDATE parent_call_event_deliveries SET status='leased',
				attempts=attempts+1,lease_id=?,lease_expires_at_ns=?,registration_version=?
				WHERE delivery_id=? AND status IN ('pending','leased') AND (status='pending' OR lease_expires_at_ns<=?)`,
				leaseID, now.Add(leaseDuration).UnixNano(), item.delivery.RegistrationVersion,
				item.delivery.ID, nowNS)
			if err != nil {
				clear(plain)
				return err
			}
			changed, err := result.RowsAffected()
			if err != nil {
				clear(plain)
				return err
			}
			if changed != 1 {
				clear(plain)
				continue
			}
			item.delivery.RegistrationToken = plain
			item.delivery.LeaseID = leaseID
			item.delivery.Attempt++
			item.delivery.ExpiresAt = time.Unix(0, item.expiresAt).UTC()
			item.delivery.LeaseExpiresAt = now.Add(leaseDuration)
			deliveries = append(deliveries, item.delivery)
		}
		return nil
	})
	if err != nil {
		for i := range deliveries {
			clear(deliveries[i].RegistrationToken)
			deliveries[i].RegistrationToken = nil
		}
		return nil, err
	}
	return deliveries, nil
}

func (s *Store) ValidateLease(ctx context.Context, delivery Delivery) (time.Duration, bool, error) {
	if delivery.ID == "" || delivery.LeaseID == "" || delivery.FamilyID == "" {
		return 0, false, ErrInvalid
	}
	now := s.now().UTC()
	eligible := false
	var remaining time.Duration
	err := storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		var eventParent, eventDevice, eventCall, deviceParent, familyParent, familyExpiry, registrationParent, observed string
		var eventSequence, latestSequence, eventExpiry, leaseExpiry, registrationVersion int64
		var callsJSON, deviceState string
		var revoked sql.NullString
		err := tx.QueryRowContext(ctx, `SELECT e.parent_account_id,e.device_id,e.call_id,e.snapshot_sequence,e.expires_at_ns,
			d.lease_expires_at_ns,s.snapshot_sequence,s.calls_json,s.observed_at,v.parent_account_id,v.state,
			f.parent_account_id,f.expires_at,f.revoked_at,r.parent_account_id,r.registration_version
			FROM parent_call_event_deliveries d
			JOIN parent_call_events e ON e.event_id=d.event_id
			JOIN devices v ON v.id=e.device_id
			JOIN auth_session_families f ON f.id=d.family_id
			JOIN parent_wakeup_registrations r ON r.family_id=d.family_id
			JOIN remote_call_snapshots s ON s.device_id=e.device_id AND s.parent_account_id=e.parent_account_id
			WHERE d.delivery_id=? AND d.family_id=? AND d.status='leased' AND d.lease_id=? AND e.status='active'
			AND e.event_id=? AND d.registration_version=?`, delivery.ID, delivery.FamilyID, delivery.LeaseID,
			delivery.EventID, delivery.RegistrationVersion).Scan(&eventParent, &eventDevice, &eventCall, &eventSequence,
			&eventExpiry, &leaseExpiry, &latestSequence, &callsJSON, &observed, &deviceParent,
			&deviceState, &familyParent, &familyExpiry, &revoked, &registrationParent, &registrationVersion)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if eventParent != delivery.ParentID || eventDevice != delivery.DeviceID || eventCall != delivery.CallID ||
			deviceParent != eventParent || familyParent != eventParent || registrationParent != eventParent ||
			deviceState != "active" || revoked.Valid ||
			registrationVersion != delivery.RegistrationVersion {
			return nil
		}
		familyDeadline, err := time.Parse(time.RFC3339Nano, familyExpiry)
		if err != nil {
			return err
		}
		observedAt, err := time.Parse(time.RFC3339Nano, observed)
		if err != nil {
			return err
		}
		if latestSequence < eventSequence || !now.Before(familyDeadline) ||
			now.UnixNano() >= eventExpiry || now.UnixNano() >= leaseExpiry ||
			!validRingingSnapshot(callsJSON, eventCall, observedAt, now) {
			if latestSequence < eventSequence || now.UnixNano() >= eventExpiry ||
				!validRingingSnapshot(callsJSON, eventCall, observedAt, now) {
				if _, err := tx.ExecContext(ctx, `UPDATE parent_call_events SET status='expired' WHERE event_id=?`, delivery.EventID); err != nil {
					return err
				}
				_, err = tx.ExecContext(ctx, `UPDATE parent_call_event_deliveries
					SET status='expired',lease_id=NULL,lease_expires_at_ns=NULL
					WHERE event_id=? AND status IN ('pending','leased')`, delivery.EventID)
				return err
			}
			if !now.Before(familyDeadline) || revoked.Valid {
				_, err = tx.ExecContext(ctx, `UPDATE parent_call_event_deliveries SET status='cancelled',
					lease_id=NULL,lease_expires_at_ns=NULL WHERE delivery_id=? AND status='leased' AND lease_id=?`,
					delivery.ID, delivery.LeaseID)
				return err
			}
			return nil
		}
		remaining = time.Unix(0, eventExpiry).Sub(now)
		if leaseRemaining := time.Unix(0, leaseExpiry).Sub(now); leaseRemaining < remaining {
			remaining = leaseRemaining
		}
		if freshRemaining := observedAt.Add(eventFreshness).Sub(now); freshRemaining < remaining {
			remaining = freshRemaining
		}
		eligible = remaining > 0
		return nil
	})
	return remaining, eligible, err
}

func (s *Store) Finish(ctx context.Context, deliveryID, leaseID string, succeeded bool) error {
	if deliveryID == "" || leaseID == "" {
		return ErrInvalid
	}
	now := s.now().UTC()
	return storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		var attempts int
		var leaseExpiry int64
		err := tx.QueryRowContext(ctx, `SELECT attempts,lease_expires_at_ns FROM parent_call_event_deliveries
			WHERE delivery_id=? AND status='leased' AND lease_id=?`, deliveryID, leaseID).Scan(&attempts, &leaseExpiry)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUnauthorized
		}
		if err != nil {
			return err
		}
		if leaseExpiry <= now.UnixNano() {
			return ErrUnauthorized
		}
		if succeeded {
			_, err = tx.ExecContext(ctx, `UPDATE parent_call_event_deliveries
				SET status='sent',lease_id=NULL,lease_expires_at_ns=NULL WHERE delivery_id=? AND lease_id=?`, deliveryID, leaseID)
			return err
		}
		status := "pending"
		if attempts >= maxAttempts {
			status = "failed"
		}
		delay := time.Second << (attempts - 1)
		if delay > 8*time.Second {
			delay = 8 * time.Second
		}
		_, err = tx.ExecContext(ctx, `UPDATE parent_call_event_deliveries SET status=?,
			next_attempt_at_ns=?,lease_id=NULL,lease_expires_at_ns=NULL
			WHERE delivery_id=? AND lease_id=?`, status, now.Add(delay).UnixNano(), deliveryID, leaseID)
		return err
	})
}

// RemoveInvalidRegistration avoids deleting a newer token after an older send fails.
func (s *Store) RemoveInvalidRegistration(ctx context.Context, parentID, familyID string, version int64) (bool, error) {
	if parentID == "" || familyID == "" || version < 1 {
		return false, ErrInvalid
	}
	removed := false
	err := storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `DELETE FROM parent_wakeup_registrations
			WHERE parent_account_id=? AND family_id=? AND registration_version=?`, parentID, familyID, version)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		removed = rows == 1
		if removed {
			_, err = tx.ExecContext(ctx, `UPDATE parent_call_event_deliveries SET status='cancelled',
				lease_id=NULL,lease_expires_at_ns=NULL WHERE family_id=? AND status IN ('pending','leased')`, familyID)
		}
		return err
	})
	return removed, err
}

func randomID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", ErrUnavailable
	}
	return hex.EncodeToString(raw), nil
}
