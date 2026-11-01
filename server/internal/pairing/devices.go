package pairing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrInvalidPage = errors.New("invalid device page")

type DevicePage struct {
	Devices    []Device `json:"devices"`
	NextCursor string   `json:"nextCursor,omitempty"`
}

// Devices uses an opaque ID keyset rather than offsets. Every page, including
// pages using a cursor supplied by another parent, remains parent-scoped.
func (service *Service) Devices(ctx context.Context, parentID, after string, limit int) (DevicePage, error) {
	page := DevicePage{Devices: []Device{}}
	if parentID == "" || limit < 1 || limit > 100 || (after != "" && !validOpaqueValue(after, "dev_", 16)) {
		return page, ErrInvalidPage
	}
	rows, err := service.database.QueryContext(ctx, `SELECT id, parent_account_id, display_name, key_algorithm, state, created_at, enrolled_at
<<<<<<< HEAD
		FROM devices WHERE parent_account_id = ? AND state != 'revoked' AND id > ? ORDER BY id LIMIT ?`, parentID, after, limit+1)
=======
		FROM devices WHERE parent_account_id = ? AND id > ? ORDER BY id LIMIT ?`, parentID, after, limit+1)
>>>>>>> origin/main
	if err != nil {
		return page, fmt.Errorf("list devices: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var device Device
		var created string
		var enrolled sql.NullString
		if err := rows.Scan(&device.ID, &device.ParentID, &device.DisplayName, &device.KeyAlgorithm, &device.State, &created, &enrolled); err != nil {
			return DevicePage{}, fmt.Errorf("scan device: %w", err)
		}
		device.CreatedAt, err = parseTime(created)
		if err != nil {
			return DevicePage{}, err
		}
		if enrolled.Valid {
			value, err := parseTime(enrolled.String)
			if err != nil {
				return DevicePage{}, err
			}
			device.EnrolledAt = &value
		}
		page.Devices = append(page.Devices, device)
	}
	if err := rows.Err(); err != nil {
		return DevicePage{}, fmt.Errorf("read devices: %w", err)
	}
	if len(page.Devices) > limit {
		page.Devices = page.Devices[:limit]
		page.NextCursor = page.Devices[limit-1].ID
	}
	return page, nil
}
