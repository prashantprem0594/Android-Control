package control

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
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

func (s *Service) QueueOpenApp(ctx context.Context, parentID, deviceID, pkg string, ttlSeconds int, key string) (Command, error) {
	if len(pkg) > 255 || !packageName.MatchString(pkg) {
		return Command{}, ErrInvalid
	}
	payload, err := json.Marshal(struct {
		PackageName string `json:"packageName"`
	}{pkg})
	if err != nil {
		return Command{}, err
	}
	return s.queueCommand(ctx, parentID, deviceID, "OPEN_APP", payload, ttlSeconds, 3600, key)
}

func (s *Service) QueueOpenSettings(ctx context.Context, parentID, deviceID string, ttlSeconds int, key string) (Command, error) {
	return s.queueCommand(ctx, parentID, deviceID, "OPEN_SETTINGS", json.RawMessage(`{}`), ttlSeconds, 3600, key)
}

// QueueRemoteScreenshot requests one consented accessibility screenshot. It never
// enables the accessibility service or bypasses Android's user-controlled setting.
func (s *Service) QueueRemoteScreenshot(ctx context.Context, parentID, deviceID string, ttlSeconds int, key string) (Command, error) {
	return s.queueCommand(ctx, parentID, deviceID, "REMOTE_SCREENSHOT", json.RawMessage(`{}`), ttlSeconds, 120, key)
}

func (s *Service) QueueRemoteTap(ctx context.Context, parentID, deviceID string, x, y, ttlSeconds int, key string) (Command, error) {
	return s.queueRemoteGesture(ctx, parentID, deviceID, map[string]any{"action": "tap", "x": x, "y": y}, ttlSeconds, key)
}

func (s *Service) QueueRemoteSwipe(ctx context.Context, parentID, deviceID string, x, y, endX, endY, durationMillis, ttlSeconds int, key string) (Command, error) {
	return s.queueRemoteGesture(ctx, parentID, deviceID, map[string]any{"action": "swipe", "x": x, "y": y, "endX": endX, "endY": endY, "durationMillis": durationMillis}, ttlSeconds, key)
}

func (s *Service) QueueRemoteLongPress(ctx context.Context, parentID, deviceID string, x, y, durationMillis, ttlSeconds int, key string) (Command, error) {
	return s.queueRemoteGesture(ctx, parentID, deviceID, map[string]any{"action": "long_press", "x": x, "y": y, "durationMillis": durationMillis}, ttlSeconds, key)
}

func (s *Service) QueueRemoteGlobal(ctx context.Context, parentID, deviceID, action string, ttlSeconds int, key string) (Command, error) {
	return s.queueRemoteGesture(ctx, parentID, deviceID, map[string]any{"action": action}, ttlSeconds, key)
}

func (s *Service) queueRemoteGesture(ctx context.Context, parentID, deviceID string, gesture map[string]any, ttlSeconds int, key string) (Command, error) {
	action, ok := gesture["action"].(string)
	if !ok {
		return Command{}, ErrInvalid
	}
	coordinate := func(name string) bool {
		value, present := gesture[name].(int)
		return present && value >= 0 && value <= 10000
	}
	duration := func(minimum int) bool {
		value, present := gesture["durationMillis"].(int)
		return present && value >= minimum && value <= 2000
	}
	switch action {
	case "tap":
		if len(gesture) != 3 || !coordinate("x") || !coordinate("y") {
			return Command{}, ErrInvalid
		}
	case "swipe":
		if len(gesture) != 6 || !coordinate("x") || !coordinate("y") || !coordinate("endX") || !coordinate("endY") || !duration(50) {
			return Command{}, ErrInvalid
		}
	case "long_press":
		if len(gesture) != 4 || !coordinate("x") || !coordinate("y") || !duration(300) {
			return Command{}, ErrInvalid
		}
	case "back", "home", "recents":
		if len(gesture) != 1 {
			return Command{}, ErrInvalid
		}
	default:
		return Command{}, ErrInvalid
	}
	payload, err := json.Marshal(gesture)
	if err != nil {
		return Command{}, err
	}
	return s.queueCommand(ctx, parentID, deviceID, "REMOTE_GESTURE", payload, ttlSeconds, 30, key)
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
		if (commandType == "ANSWER_CALL" || commandType == "REJECT_CALL" || commandType == "END_CALL") && !s.remoteCallsEnabled {
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
		if commandType == "OPEN_APP" {
			var target struct {
				PackageName string `json:"packageName"`
			}
			if err := json.Unmarshal(payload, &target); err != nil || target.PackageName == "" {
				return ErrInvalid
			}
			inventory, exists, err := readInventory(ctx, tx, deviceID)
			if err != nil {
				return err
			}
			if !exists {
				return ErrInvalid
			}
			found := false
			for _, app := range inventory.Apps {
				if app.PackageName == target.PackageName && app.Launchable {
					found = true
					break
				}
			}
			if !found {
				return ErrInvalid
			}
		} else if commandType == "OPEN_SETTINGS" {
			if string(payload) != `{}` {
				return ErrInvalid
			}
		} else if commandType == "REMOTE_SCREENSHOT" {
			if string(payload) != `{}` {
				return ErrInvalid
			}
		} else if commandType == "REMOTE_GESTURE" {
			var gesture struct {
				Action         string `json:"action"`
				X              int    `json:"x"`
				Y              int    `json:"y"`
				EndX           int    `json:"endX"`
				EndY           int    `json:"endY"`
				DurationMillis int    `json:"durationMillis"`
			}
			if err := json.Unmarshal(payload, &gesture); err != nil || !validRemoteGesture(gesture.Action, gesture.X, gesture.Y, gesture.EndX, gesture.EndY, gesture.DurationMillis, payload) {
				return ErrInvalid
			}
		} else if commandType == "ANSWER_CALL" || commandType == "REJECT_CALL" || commandType == "END_CALL" {
			var action struct {
				CallID string `json:"callId"`
			}
			if err := json.Unmarshal(payload, &action); err != nil {
				return err
			}
			if err := s.requireCurrentCall(ctx, tx, deviceID, action.CallID, commandType, now); err != nil {
				return err
			}
		} else if commandType == "LOCK_DEVICE" {
			if string(payload) != `{}` {
				return ErrInvalid
			}
		} else {
			return ErrInvalid
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

func validRemoteGesture(action string, x, y, endX, endY, duration int, payload []byte) bool {
	var raw map[string]json.RawMessage
	if json.Unmarshal(payload, &raw) != nil {
		return false
	}
	coordinate := func(value int) bool { return value >= 0 && value <= 10000 }
	switch action {
	case "tap":
		return len(raw) == 3 && coordinate(x) && coordinate(y)
	case "swipe":
		return len(raw) == 6 && coordinate(x) && coordinate(y) && coordinate(endX) && coordinate(endY) && duration >= 50 && duration <= 2000
	case "long_press":
		return len(raw) == 4 && coordinate(x) && coordinate(y) && duration >= 300 && duration <= 2000
	case "back", "home", "recents":
		return len(raw) == 1
	default:
		return false
	}
}

type RemoteScreen struct {
	ImageBase64 string    `json:"imageBase64"`
	CreatedAt   time.Time `json:"createdAt"`
}

// SaveRemoteScreen accepts a bounded JPEG encoded as canonical base64 from the
// enrolled device that owns the screenshot command.
func (s *Service) SaveRemoteScreen(ctx context.Context, token, commandID, imageBase64 string) error {
	if len(imageBase64) < 4 || len(imageBase64) > 524288 {
		return ErrInvalid
	}
	image, err := base64.StdEncoding.DecodeString(imageBase64)
	if err != nil || len(image) < 16 || len(image) > 384*1024 || image[0] != 0xff || image[1] != 0xd8 {
		return ErrInvalid
	}
	if base64.StdEncoding.EncodeToString(image) != imageBase64 {
		return ErrInvalid
	}
	return storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		now := s.now().UTC()
		deviceID, err := deviceAuthorization(ctx, tx, token, now)
		if err != nil {
			return err
		}
		if err := expireCommands(ctx, tx, deviceID, now); err != nil {
			return err
		}
		var status, commandType string
		err = tx.QueryRowContext(ctx, "SELECT status,type FROM commands WHERE id=? AND device_id=?", commandID, deviceID).Scan(&status, &commandType)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if status != "delivered" || commandType != "REMOTE_SCREENSHOT" {
			return ErrCommandState
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO remote_assistance_screens(command_id,image_base64,created_at) VALUES(?,?,?) ON CONFLICT(command_id) DO UPDATE SET image_base64=excluded.image_base64,created_at=excluded.created_at", commandID, imageBase64, now.Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		// Remote assistance is interactive; retaining every captured screen would
		// let the database grow without bound. Keep the most recent 20 per device.
		_, err = tx.ExecContext(ctx, `DELETE FROM remote_assistance_screens WHERE command_id IN (
 SELECT r.command_id FROM remote_assistance_screens r JOIN commands c ON c.id=r.command_id
 WHERE c.device_id=? ORDER BY r.created_at DESC, r.command_id DESC LIMIT -1 OFFSET 20)`, deviceID)
		return err
	})
}

func (s *Service) RemoteScreen(ctx context.Context, parentID, deviceID, commandID string) (RemoteScreen, error) {
	var screen RemoteScreen
	err := storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		if err := owned(ctx, tx, parentID, deviceID); err != nil {
			return err
		}
		var created string
		err := tx.QueryRowContext(ctx, `SELECT r.image_base64,r.created_at FROM remote_assistance_screens r
 JOIN commands c ON c.id=r.command_id WHERE r.command_id=? AND c.device_id=? AND c.parent_account_id=? AND c.type='REMOTE_SCREENSHOT'`,
			commandID, deviceID, parentID).Scan(&screen.ImageBase64, &created)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		screen.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		return err
	})
	return screen, err
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
