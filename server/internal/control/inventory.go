package control

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"appcontrol/server/internal/storage"
)

type InstalledApp struct {
	PackageName string `json:"packageName"`
	Label       string `json:"label"`
	System      bool   `json:"system"`
	Blockable   bool   `json:"blockable"`
	Suspended   bool   `json:"suspended"`
<<<<<<< HEAD
	Launchable  bool   `json:"launchable"`
=======
>>>>>>> origin/main
}
type AppInventory struct {
	Apps      []InstalledApp `json:"apps"`
	UpdatedAt *time.Time     `json:"updatedAt,omitempty"`
}

func protectedNamespace(name string) bool {
	return name == "android" || strings.HasPrefix(name, "com.android.") || strings.HasPrefix(name, "com.google.android.") || strings.HasPrefix(name, "com.appcontrol.")
}

func (s *Service) UpdateInventory(ctx context.Context, token string, apps []InstalledApp) error {
	if len(apps) > 2000 {
		return ErrInvalid
	}
	normalized := make([]InstalledApp, len(apps))
	copy(normalized, apps)
	seen := map[string]bool{}
	for i := range normalized {
		a := &normalized[i]
		if len(a.PackageName) > 255 || a.PackageName != "android" && !packageName.MatchString(a.PackageName) || len(a.Label) > 512 || seen[a.PackageName] {
			return ErrInvalid
		}
		seen[a.PackageName] = true
		if a.System || protectedNamespace(a.PackageName) {
			a.Blockable = false
		}
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].PackageName < normalized[j].PackageName })
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return err
	}
	return storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		now := s.now().UTC()
		deviceID, err := deviceAuthorization(ctx, tx, token, now)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO device_app_inventory(device_id,parent_account_id,apps_json,updated_at)
   SELECT id,parent_account_id,?,? FROM devices WHERE id=?
   ON CONFLICT(device_id) DO UPDATE SET apps_json=excluded.apps_json,updated_at=excluded.updated_at
   WHERE device_app_inventory.apps_json!=excluded.apps_json`, string(encoded), now.Format(time.RFC3339Nano), deviceID)
		return err
	})
}

func readInventory(ctx context.Context, tx *sql.Tx, deviceID string) (AppInventory, bool, error) {
	result := AppInventory{Apps: []InstalledApp{}}
	var encoded, timestamp string
	err := tx.QueryRowContext(ctx, "SELECT apps_json,updated_at FROM device_app_inventory WHERE device_id=?", deviceID).Scan(&encoded, &timestamp)
	if errors.Is(err, sql.ErrNoRows) {
		return result, false, nil
	}
	if err != nil {
		return result, false, err
	}
	if err = json.Unmarshal([]byte(encoded), &result.Apps); err != nil {
		return result, false, err
	}
	updated, err := time.Parse(time.RFC3339Nano, timestamp)
	result.UpdatedAt = &updated
	return result, true, err
}

func (s *Service) Inventory(ctx context.Context, parentID, deviceID string) (AppInventory, error) {
	result := AppInventory{Apps: []InstalledApp{}}
	err := storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		if err := owned(ctx, tx, parentID, deviceID); err != nil {
			return err
		}
		var err error
		result, _, err = readInventory(ctx, tx, deviceID)
		return err
	})
	return result, err
}

func validateInventoryTargets(ctx context.Context, tx *sql.Tx, deviceID string, targets []string) error {
	inventory, exists, err := readInventory(ctx, tx, deviceID)
	if err != nil || !exists {
		return err
	}
	eligible := map[string]bool{}
	for _, a := range inventory.Apps {
		eligible[a.PackageName] = a.Blockable
	}
	for _, target := range targets {
		if !eligible[target] {
			return ErrInvalid
		}
	}
	return nil
}
