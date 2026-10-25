package control

import (
	"appcontrol/server/internal/storage"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
)

var commandIDPattern = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`)

type CommandPage struct {
	Commands   []Command `json:"commands"`
	NextCursor string    `json:"nextCursor,omitempty"`
}

// SQLite's date comparison and the ID tie-breaker are identical in ORDER BY
// and the cursor predicate, including timestamps differing only in precision.
func (s *Service) Commands(ctx context.Context, parentID, deviceID, after string, limit int) (CommandPage, error) {
	page := CommandPage{Commands: []Command{}}
	if limit < 1 || limit > 100 || len(after) > 256 {
		return page, ErrInvalid
	}
	var boundary, id string
	if after != "" {
		raw, err := base64.RawURLEncoding.Strict().DecodeString(after)
		if err != nil {
			return page, ErrInvalid
		}
		parts := strings.Split(string(raw), "\n")
		if len(parts) != 2 || !commandIDPattern.MatchString(parts[1]) {
			return page, ErrInvalid
		}
		if _, err := time.Parse(time.RFC3339Nano, parts[0]); err != nil {
			return page, ErrInvalid
		}
		boundary, id = parts[0], parts[1]
	}
	err := storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		var exists int
		err := tx.QueryRowContext(ctx, "SELECT 1 FROM devices WHERE id=? AND parent_account_id=?", deviceID, parentID).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := expireCommands(ctx, tx, deviceID, s.now().UTC()); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, `SELECT id,type,payload,issued_at,expires_at,status FROM commands
		 WHERE device_id=? AND parent_account_id=? AND (?='' OR julianday(issued_at)<julianday(?) OR (julianday(issued_at)=julianday(?) AND id<?))
		 ORDER BY julianday(issued_at) DESC,id DESC LIMIT ?`, deviceID, parentID, boundary, boundary, boundary, id, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			command := Command{Version: 1, DeviceID: deviceID}
			var payload, issued, expires string
			if err := rows.Scan(&command.ID, &command.Type, &payload, &issued, &expires, &command.Status); err != nil {
				return err
			}
			command.Payload = json.RawMessage(payload)
			command.IssuedAt, err = time.Parse(time.RFC3339Nano, issued)
			if err != nil {
				return err
			}
			command.ExpiresAt, err = time.Parse(time.RFC3339Nano, expires)
			if err != nil {
				return err
			}
			page.Commands = append(page.Commands, command)
		}
		return rows.Err()
	})
	if err != nil {
		return CommandPage{}, err
	}
	if len(page.Commands) > limit {
		page.Commands = page.Commands[:limit]
		last := page.Commands[limit-1]
		page.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(last.IssuedAt.Format(time.RFC3339Nano) + "\n" + last.ID))
	}
	return page, nil
}
