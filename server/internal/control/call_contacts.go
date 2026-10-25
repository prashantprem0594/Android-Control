package control

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"appcontrol/server/internal/storage"
)

// Call contacts are explicit local-dial shortcuts, never remote dial commands.
type CallContact struct {
	Label    string `json:"label"`
	Number   string `json:"number"`
	IsParent bool   `json:"isParent"`
}

type CallContacts struct {
	Version  int64         `json:"version"`
	Contacts []CallContact `json:"contacts"`
}

var contactNumber = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)

func validateCallContacts(contacts []CallContact) error {
	if contacts == nil || len(contacts) > 10 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	parents := 0
	for _, c := range contacts {
		if !utf8.ValidString(c.Label) || len(c.Label) > 160 || utf8.RuneCountInString(c.Label) > 40 || strings.TrimSpace(c.Label) != c.Label || c.Label == "" || !contactNumber.MatchString(c.Number) || seen[c.Number] {
			return ErrInvalid
		}
		for _, r := range c.Label {
			if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) {
				return ErrInvalid
			}
		}
		seen[c.Number] = true
		if c.IsParent {
			parents++
		}
	}
	if parents > 1 {
		return ErrInvalid
	}
	return nil
}

func readCallContacts(ctx context.Context, tx *sql.Tx, deviceID string) (CallContacts, error) {
	result := CallContacts{Contacts: []CallContact{}}
	var raw string
	err := tx.QueryRowContext(ctx, "SELECT version,contacts_json FROM device_call_contacts WHERE device_id=?", deviceID).Scan(&result.Version, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	err = json.Unmarshal([]byte(raw), &result.Contacts)
	return result, err
}

func (s *Service) CallContacts(ctx context.Context, parentID, deviceID string) (CallContacts, error) {
	var result CallContacts
	err := storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		if err := owned(ctx, tx, parentID, deviceID); err != nil {
			return err
		}
		var err error
		result, err = readCallContacts(ctx, tx, deviceID)
		return err
	})
	return result, err
}

func (s *Service) DeviceCallContacts(ctx context.Context, token string) (CallContacts, error) {
	var result CallContacts
	err := storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		id, err := deviceAuthorization(ctx, tx, token, s.now().UTC())
		if err != nil {
			return err
		}
		result, err = readCallContacts(ctx, tx, id)
		return err
	})
	return result, err
}

func (s *Service) UpdateCallContacts(ctx context.Context, parentID, deviceID string, expected int64, contacts []CallContact) (CallContacts, error) {
	if expected < 0 || expected >= 1<<53-1 {
		return CallContacts{}, ErrInvalid
	}
	if err := validateCallContacts(contacts); err != nil {
		return CallContacts{}, err
	}
	result := CallContacts{Version: expected + 1, Contacts: append([]CallContact{}, contacts...)}
	raw, err := json.Marshal(result.Contacts)
	if err != nil {
		return CallContacts{}, err
	}
	err = storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		if err := owned(ctx, tx, parentID, deviceID); err != nil {
			return err
		}
		current, err := readCallContacts(ctx, tx, deviceID)
		if err != nil {
			return err
		}
		if current.Version != expected {
			return ErrConflict
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO device_call_contacts(device_id,parent_account_id,version,contacts_json,updated_at) VALUES(?,?,?,?,?)
   ON CONFLICT(device_id) DO UPDATE SET version=excluded.version,contacts_json=excluded.contacts_json,updated_at=excluded.updated_at`, deviceID, parentID, result.Version, string(raw), s.now().UTC().Format(time.RFC3339Nano))
		return err
	})
	if err == nil {
		s.notify(ctx, deviceID)
	}
	return result, err
}
