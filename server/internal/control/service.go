package control

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"time"

	"appcontrol/server/internal/storage"
)

var packageName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)+$`)
var callIDPattern = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`)

var (
	ErrInvalid            = errors.New("invalid control request")
	ErrNotFound           = errors.New("active device not found")
	ErrConflict           = errors.New("policy version conflict")
	ErrUnauthorized       = errors.New("invalid device credential")
	ErrCommandState       = errors.New("command state conflict")
	ErrIdempotency        = errors.New("idempotency key reused with different input")
	ErrPolicyFormat       = errors.New("unsupported policy format")
	ErrCallUnavailable    = errors.New("current call unavailable")
	ErrCallState          = errors.New("current call state conflict")
	ErrRemoteCallDisabled = errors.New("remote calls are disabled")
)

type Wakeup interface {
	Wake(context.Context, string) error
}

type Service struct {
	db                 *sql.DB
	now                func() time.Time
	wakeup             Wakeup
	remoteCallsEnabled bool
}

func NewService(db *sql.DB) *Service { return &Service{db: db, now: time.Now} }

func NewServiceWithWakeup(db *sql.DB, provider Wakeup) *Service {
	s := NewService(db)
	s.wakeup = provider
	return s
}

func (s *Service) SetRemoteCallsEnabled(enabled bool) { s.remoteCallsEnabled = enabled }

type Command struct {
	Version   int             `json:"version"`
	ID        string          `json:"commandId"`
	DeviceID  string          `json:"deviceId"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	IssuedAt  time.Time       `json:"issuedAt"`
	ExpiresAt time.Time       `json:"expiresAt"`
	Status    string          `json:"status"`
}

// QueueLock persists intent only. Delivery and child acknowledgement are separate.
func (s *Service) QueueLock(ctx context.Context, parentID, deviceID string, ttlSeconds int) (Command, error) {
	return s.QueueLockWithKey(ctx, parentID, deviceID, ttlSeconds, "")
}

func (s *Service) QueueCallAction(ctx context.Context, parentID, deviceID, action, callID string, ttlSeconds int) (Command, error) {
	return s.QueueCallActionWithKey(ctx, parentID, deviceID, action, callID, ttlSeconds, "")
}

func (s *Service) QueueLockWithKey(ctx context.Context, parentID, deviceID string, ttlSeconds int, key string) (Command, error) {
	return s.queueCommand(ctx, parentID, deviceID, "LOCK_DEVICE", json.RawMessage(`{}`), ttlSeconds, 3600, key)
}

func (s *Service) QueueCallActionWithKey(ctx context.Context, parentID, deviceID, action, callID string, ttlSeconds int, key string) (Command, error) {
	if !callIDPattern.MatchString(callID) {
		return Command{}, ErrInvalid
	}
	if action != "ANSWER_CALL" && action != "REJECT_CALL" && action != "END_CALL" {
		return Command{}, ErrInvalid
	}
	payload, err := json.Marshal(struct {
		CallID string `json:"callId"`
	}{callID})
	if err != nil {
		return Command{}, err
	}
	return s.queueCommand(ctx, parentID, deviceID, action, payload, ttlSeconds, 30, key)
}

func (s *Service) queueCommand(ctx context.Context, parentID, deviceID, commandType string, payload json.RawMessage, ttlSeconds, maxTTL int, key string) (Command, error) {
	if ttlSeconds < 1 || ttlSeconds > maxTTL {
		return Command{}, ErrInvalid
	}
	if key != "" {
		if len(key) < 16 || len(key) > 128 {
			return Command{}, ErrInvalid
		}
		for _, c := range key {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
				return Command{}, ErrInvalid
			}
		}
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return Command{}, err
	}
	raw[6] = (raw[6] & 15) | 64
	raw[8] = (raw[8] & 63) | 128
	h := hex.EncodeToString(raw)
	id := h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
	now := s.now().UTC()
	command := Command{1, id, deviceID, commandType, payload, now, now.Add(time.Duration(ttlSeconds) * time.Second), "queued"}
	created := false
	err := storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		if err := owned(ctx, tx, parentID, deviceID); err != nil {
			return err
		}
		if commandType != "LOCK_DEVICE" && !s.remoteCallsEnabled {
			return ErrRemoteCallDisabled
		}
		if key != "" {
			if err := expireCommands(ctx, tx, deviceID, now); err != nil {
				return err
			}
			var issued, expires, storedType, storedPayload string
			var storedTTL int
			err := tx.QueryRowContext(ctx, `SELECT id,type,payload,issued_at,expires_at,status,ttl_seconds FROM commands WHERE parent_account_id=? AND device_id=? AND request_key=?`, parentID, deviceID, key).Scan(&command.ID, &storedType, &storedPayload, &issued, &expires, &command.Status, &storedTTL)
			if err == nil {
				if storedTTL != ttlSeconds || storedType != commandType || storedPayload != string(payload) {
					return ErrIdempotency
				}
				command.Type = storedType
				command.Payload = json.RawMessage(storedPayload)
				command.IssuedAt, err = time.Parse(time.RFC3339Nano, issued)
				if err != nil {
					return err
				}
				command.ExpiresAt, err = time.Parse(time.RFC3339Nano, expires)
				return err
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		if commandType != "LOCK_DEVICE" {
			var action struct {
				CallID string `json:"callId"`
			}
			if err := json.Unmarshal(payload, &action); err != nil {
				return err
			}
			if err := s.requireCurrentCall(ctx, tx, deviceID, action.CallID, commandType, now); err != nil {
				return err
			}
		}
		var requestKey any
		if key != "" {
			requestKey = key
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO commands(id,device_id,parent_account_id,type,payload,issued_at,expires_at,status,request_key,ttl_seconds) VALUES(?,?,?,?,?,?,?,'queued',?,?)`, id, deviceID, parentID, commandType, string(payload), now.Format(time.RFC3339Nano), command.ExpiresAt.Format(time.RFC3339Nano), requestKey, ttlSeconds)
		created = err == nil
		return err
	})
	if err == nil && created && s.wakeup != nil {
		s.notify(ctx, deviceID)
	}
	return command, err
}

type Policy struct {
	FormatVersion         int           `json:"formatVersion"`
	Version               int64         `json:"version"`
	InternetPaused        bool          `json:"internetPaused"`
	BlockedPackages       []string      `json:"blockedPackages"`
	UpdatedAt             time.Time     `json:"updatedAt"`
	Reported              *PolicyReport `json:"reported,omitempty"`
	AlwaysAllowedPackages []string      `json:"alwaysAllowedPackages"`
	Schedules             []Schedule    `json:"schedules"`
	EffectiveStateKey     string        `json:"effectiveStateKey,omitempty"`
}

// UpdatePolicy preserves the original base-policy API as a full replacement.
func (s *Service) UpdatePolicy(ctx context.Context, parentID, deviceID string, expected int64, paused bool, packages []string) (Policy, error) {
	return s.UpdatePolicyRules(ctx, parentID, deviceID, expected, paused, packages, nil, nil)
}

func (s *Service) notify(ctx context.Context, deviceID string) {
	if s.wakeup == nil {
		return
	}
	wakeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_ = s.wakeup.Wake(wakeCtx, deviceID) // Hint failure never undoes committed state.
}

func owned(ctx context.Context, tx *sql.Tx, parentID, deviceID string) error {
	var found int
	err := tx.QueryRowContext(ctx, "SELECT 1 FROM devices WHERE id=? AND parent_account_id=? AND state='active'", deviceID, parentID).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (s *Service) Policy(ctx context.Context, parentID, deviceID string) (Policy, error) {
	var policy Policy
	err := storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		if err := owned(ctx, tx, parentID, deviceID); err != nil {
			return err
		}
		var err error
		policy, err = readPolicy(ctx, tx, deviceID)
		if err != nil {
			return err
		}
		policy.Reported, err = readPolicyReport(ctx, tx, deviceID)
		if err != nil {
			return err
		}
		effective, err := ResolvePolicy(policy, s.now().UTC())
		if err != nil {
			return err
		}
		policy.EffectiveStateKey = effective.StateKey
		return nil
	})
	return policy, err
}

func (s *Service) Command(ctx context.Context, parentID, deviceID, commandID string) (Command, error) {
	command := Command{Version: 1, DeviceID: deviceID, ID: commandID}
	err := storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		var exists int
		if err := tx.QueryRowContext(ctx, "SELECT 1 FROM devices WHERE id=? AND parent_account_id=?", deviceID, parentID).Scan(&exists); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if err := expireCommands(ctx, tx, deviceID, s.now().UTC()); err != nil {
			return err
		}
		var payload, issued, expires string
		err := tx.QueryRowContext(ctx, `SELECT c.type,c.payload,c.issued_at,c.expires_at,c.status FROM commands c
  JOIN devices d ON d.id=c.device_id WHERE c.id=? AND c.device_id=? AND c.parent_account_id=? AND d.parent_account_id=?`, commandID, deviceID, parentID, parentID).Scan(&command.Type, &payload, &issued, &expires, &command.Status)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		command.Payload = json.RawMessage(payload)
		command.IssuedAt, err = time.Parse(time.RFC3339Nano, issued)
		if err != nil {
			return err
		}
		command.ExpiresAt, err = time.Parse(time.RFC3339Nano, expires)
		return err
	})
	return command, err
}
