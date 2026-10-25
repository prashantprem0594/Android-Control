package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

func IntegrityCheck(ctx context.Context, database *sql.DB) error {
	if database == nil {
		return fmt.Errorf("database is required")
	}
	rows, err := database.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return fmt.Errorf("run SQLite integrity check: %w", err)
	}
	defer rows.Close()

	var findings []string
	for rows.Next() {
		var finding string
		if err := rows.Scan(&finding); err != nil {
			return fmt.Errorf("scan SQLite integrity result: %w", err)
		}
		if !strings.EqualFold(finding, "ok") {
			findings = append(findings, finding)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read SQLite integrity result: %w", err)
	}
	if len(findings) != 0 {
		return fmt.Errorf("SQLite integrity check failed: %s", strings.Join(findings, "; "))
	}
	return nil
}
