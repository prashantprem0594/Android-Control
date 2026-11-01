package control

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"appcontrol/server/internal/storage"
)

// RemoteSession is signaling metadata only; media still requires a deployed WebRTC/TURN path.
type RemoteSession struct {
	ID        string    `json:"sessionId"`
	DeviceID  string    `json:"deviceId"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
	OfferSDP  string    `json:"offerSdp,omitempty"`
	AnswerSDP string    `json:"answerSdp,omitempty"`
	ICE       []string  `json:"ice,omitempty"`
}

type RemoteSessionStart struct {
	RemoteSession
	ParentToken string `json:"parentToken"`
}

func newRemoteID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	h := hex.EncodeToString(b)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], nil
}

func newRemoteToken() (string, []byte, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	token := "rs_" + base64.RawURLEncoding.EncodeToString(b)
	hash := sha256.Sum256([]byte(token))
	return token, hash[:], nil
}

func (s *Service) StartRemoteSession(ctx context.Context, parentID, deviceID string, ttlSeconds int) (RemoteSessionStart, error) {
	if ttlSeconds < 30 || ttlSeconds > 3600 {
		return RemoteSessionStart{}, ErrInvalid
	}
	var result RemoteSessionStart
	err := storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		if err := owned(ctx, tx, parentID, deviceID); err != nil {
			return err
		}
		now := s.now().UTC()
		// Starting a new session deterministically closes the previous one, preserving
		// the one-active-session-per-device invariant without relying on cleanup jobs.
		_, err := tx.ExecContext(ctx, `UPDATE remote_assistance_sessions SET status='closed',closed_at=? WHERE device_id=? AND status='active'`, now.Format(time.RFC3339Nano), deviceID)
		if err != nil {
			return err
		}
		id, err := newRemoteID()
		if err != nil {
			return err
		}
		token, hash, err := newRemoteToken()
		if err != nil {
			return err
		}
		expires := now.Add(time.Duration(ttlSeconds) * time.Second)
		_, err = tx.ExecContext(ctx, `INSERT INTO remote_assistance_sessions(id,device_id,parent_account_id,token_hash,status,created_at,expires_at) VALUES(?,?,?,?,?,?,?)`, id, deviceID, parentID, hash, "active", now.Format(time.RFC3339Nano), expires.Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		result = RemoteSessionStart{RemoteSession: RemoteSession{ID: id, DeviceID: deviceID, Status: "active", CreatedAt: now, ExpiresAt: expires, ICE: []string{}}, ParentToken: token}
		return nil
	})
	return result, err
}

func scanRemoteSession(row *sql.Row) (RemoteSession, error) {
	var s RemoteSession
	var created, expires, ice string
	err := row.Scan(&s.ID, &s.DeviceID, &s.Status, &created, &expires, &s.OfferSDP, &s.AnswerSDP, &ice)
	if err != nil {
		return s, err
	}
	s.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return s, err
	}
	s.ExpiresAt, err = time.Parse(time.RFC3339Nano, expires)
	if err != nil {
		return s, err
	}
	if err = json.Unmarshal([]byte(ice), &s.ICE); err != nil {
		return s, err
	}
	return s, nil
}

func (s *Service) RemoteSession(ctx context.Context, parentID, deviceID, sessionID string) (RemoteSession, error) {
	var result RemoteSession
	err := storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		if err := owned(ctx, tx, parentID, deviceID); err != nil {
			return err
		}
		var err error
		result, err = scanRemoteSession(tx.QueryRowContext(ctx, `SELECT id,device_id,status,created_at,expires_at,offer_sdp,answer_sdp,ice_json FROM remote_assistance_sessions WHERE id=? AND device_id=? AND parent_account_id=?`, sessionID, deviceID, parentID))
		return err
	})
	if errors.Is(err, sql.ErrNoRows) {
		return RemoteSession{}, ErrNotFound
	}
	return result, err
}

func (s *Service) UpdateRemoteSignal(ctx context.Context, token, sessionID, field, value string) error {
	if len(value) > 256*1024 || (field != "answer_sdp" && field != "ice_json") {
		return ErrInvalid
	}
	return storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		deviceID, err := deviceAuthorization(ctx, tx, token, s.now().UTC())
		if err != nil {
			return err
		}
		now := s.now().UTC().Format(time.RFC3339Nano)
		var query string
		if field == "answer_sdp" {
			query = `UPDATE remote_assistance_sessions SET answer_sdp=? WHERE id=? AND device_id=? AND status='active' AND expires_at>?`
		} else {
			query = `UPDATE remote_assistance_sessions SET ice_json=? WHERE id=? AND device_id=? AND status='active' AND expires_at>?`
		}
		res, err := tx.ExecContext(ctx, query, value, sessionID, deviceID, now)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return ErrNotFound
		}
		return nil
	})
}

func (s *Service) ActiveRemoteSession(ctx context.Context, token string) (RemoteSession, error) {
	var result RemoteSession
	err := storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		deviceID, err := deviceAuthorization(ctx, tx, token, s.now().UTC())
		if err != nil {
			return err
		}
		now := s.now().UTC().Format(time.RFC3339Nano)
		_, _ = tx.ExecContext(ctx, `UPDATE remote_assistance_sessions SET status='expired',closed_at=? WHERE device_id=? AND status='active' AND expires_at<=?`, now, deviceID, now)
		result, err = scanRemoteSession(tx.QueryRowContext(ctx, `SELECT id,device_id,status,created_at,expires_at,offer_sdp,answer_sdp,ice_json FROM remote_assistance_sessions WHERE device_id=? AND status='active' AND expires_at>?`, deviceID, now))
		return err
	})
	if errors.Is(err, sql.ErrNoRows) {
		return RemoteSession{}, ErrNotFound
	}
	return result, err
}

func (s *Service) UpdateRemoteOffer(ctx context.Context, parentID, deviceID, sessionID, offer string) error {
	if strings.TrimSpace(offer) == "" || len(offer) > 256*1024 {
		return ErrInvalid
	}
	return storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		if err := owned(ctx, tx, parentID, deviceID); err != nil {
			return err
		}
		now := s.now().UTC().Format(time.RFC3339Nano)
		res, err := tx.ExecContext(ctx, `UPDATE remote_assistance_sessions SET offer_sdp=? WHERE id=? AND device_id=? AND parent_account_id=? AND status='active' AND expires_at>?`, offer, sessionID, deviceID, parentID, now)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return ErrNotFound
		}
		return nil
	})
}

func (s *Service) CloseRemoteSession(ctx context.Context, parentID, deviceID, sessionID string) error {
	return storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		if err := owned(ctx, tx, parentID, deviceID); err != nil {
			return err
		}
		now := s.now().UTC().Format(time.RFC3339Nano)
		res, err := tx.ExecContext(ctx, `UPDATE remote_assistance_sessions SET status='closed',closed_at=? WHERE id=? AND device_id=? AND parent_account_id=? AND status='active'`, now, sessionID, deviceID, parentID)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return ErrNotFound
		}
		return nil
	})
}
