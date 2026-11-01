package control

import (
	"appcontrol/server/internal/storage"
	"context"
	"database/sql"
	"errors"
	"time"
)

// VisibleLocation is intentionally small: it represents a point the child
// voluntarily shared from its foreground session, not a tracking state.
type VisibleLocation struct {
	LatitudeE7     int64     `json:"latitudeE7"`
	LongitudeE7    int64     `json:"longitudeE7"`
	AccuracyMeters float64   `json:"accuracyMeters"`
	CapturedAt     time.Time `json:"capturedAt"`
	ReceivedAt     time.Time `json:"receivedAt"`
}

const visibleLocationRetention = 12 * time.Hour

func validVisibleLocation(location VisibleLocation, now time.Time) bool {
	if location.LatitudeE7 < -900000000 || location.LatitudeE7 > 900000000 ||
		location.LongitudeE7 < -1800000000 || location.LongitudeE7 > 1800000000 ||
		location.AccuracyMeters < 0 || location.AccuracyMeters > 100000 || location.CapturedAt.IsZero() {
		return false
	}
	// Do not accept an old location uploaded later as if it were current, nor
	// a client clock far in the future. A foreground update should be recent.
	return !location.CapturedAt.Before(now.Add(-15*time.Minute)) && !location.CapturedAt.After(now.Add(5*time.Minute))
}

// RecordVisibleLocation requires the enrolled device credential. There is no
// parent command path for this operation: recording can only originate on the
// child while its visible foreground service is running.
func (s *Service) RecordVisibleLocation(ctx context.Context, token string, location VisibleLocation) error {
	return storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		now := s.now().UTC()
		if !validVisibleLocation(location, now) {
			return ErrInvalid
		}
		deviceID, err := deviceAuthorization(ctx, tx, token, now)
		if err != nil {
			return err
		}
		if err := pruneVisibleLocations(ctx, tx, now); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO visible_location_samples(device_id,latitude_e7,longitude_e7,accuracy_meters,captured_at,received_at)
			VALUES(?,?,?,?,?,?)`, deviceID, location.LatitudeE7, location.LongitudeE7, location.AccuracyMeters,
			location.CapturedAt.UTC().Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
		return err
	})
}

// VisibleLocation returns only an active parent's newest retained point.
func (s *Service) VisibleLocation(ctx context.Context, parentID, deviceID string) (*VisibleLocation, error) {
	var result *VisibleLocation
	err := storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		now := s.now().UTC()
		if err := owned(ctx, tx, parentID, deviceID); err != nil {
			return err
		}
		if err := pruneVisibleLocations(ctx, tx, now); err != nil {
			return err
		}
		var value VisibleLocation
		var captured, received string
		err := tx.QueryRowContext(ctx, `SELECT latitude_e7,longitude_e7,accuracy_meters,captured_at,received_at
			FROM visible_location_samples WHERE device_id=? ORDER BY received_at DESC,id DESC LIMIT 1`, deviceID).
			Scan(&value.LatitudeE7, &value.LongitudeE7, &value.AccuracyMeters, &captured, &received)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		value.CapturedAt, err = time.Parse(time.RFC3339Nano, captured)
		if err != nil {
			return err
		}
		value.ReceivedAt, err = time.Parse(time.RFC3339Nano, received)
		if err != nil {
			return err
		}
		result = &value
		return nil
	})
	return result, err
}

// PruneVisibleLocations is used by the server maintenance loop so retention
// also happens when neither parent nor child is making requests.
func (s *Service) PruneVisibleLocations(ctx context.Context) error {
	return storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		return pruneVisibleLocations(ctx, tx, s.now().UTC())
	})
}

func pruneVisibleLocations(ctx context.Context, tx *sql.Tx, now time.Time) error {
	_, err := tx.ExecContext(ctx, "DELETE FROM visible_location_samples WHERE received_at <= ?", now.Add(-visibleLocationRetention).Format(time.RFC3339Nano))
	return err
}
