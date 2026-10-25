package storage

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

type migration struct {
	version int
	name    string
	sql     string
}

func Migrate(ctx context.Context, database *sql.DB) error {
	return migrateFS(ctx, database, embeddedMigrations)
}

func migrateFS(ctx context.Context, database *sql.DB, migrationFS fs.FS) error {
	if database == nil {
		return fmt.Errorf("database is required")
	}
	if _, err := database.ExecContext(ctx, `
        CREATE TABLE IF NOT EXISTS schema_migrations (
            version INTEGER PRIMARY KEY NOT NULL,
            name TEXT NOT NULL,
            applied_at TEXT NOT NULL
        ) STRICT
    `); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}

	migrations, err := readMigrations(migrationFS)
	if err != nil {
		return err
	}
	applied, err := appliedMigrations(ctx, database)
	if err != nil {
		return err
	}

	for _, next := range migrations {
		if appliedName, exists := applied[next.version]; exists {
			if appliedName != next.name {
				return fmt.Errorf("migration %d name changed from %q to %q", next.version, appliedName, next.name)
			}
			continue
		}

		if err := WithinTransaction(ctx, database, func(transaction *sql.Tx) error {
			if _, err := transaction.ExecContext(ctx, next.sql); err != nil {
				return fmt.Errorf("execute migration %s: %w", next.name, err)
			}
			if _, err := transaction.ExecContext(ctx,
				"INSERT INTO schema_migrations(version, name, applied_at) VALUES (?, ?, ?)",
				next.version, next.name, time.Now().UTC().Format(time.RFC3339Nano),
			); err != nil {
				return fmt.Errorf("record migration %s: %w", next.name, err)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func readMigrations(migrationFS fs.FS) ([]migration, error) {
	paths, err := fs.Glob(migrationFS, "migrations/*.sql")
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(paths)
	result := make([]migration, 0, len(paths))
	versions := make(map[int]string, len(paths))
	for _, path := range paths {
		name := filepath.Base(path)
		separator := strings.IndexByte(name, '_')
		if separator <= 0 {
			return nil, fmt.Errorf("migration filename must begin with a numeric version: %s", name)
		}
		version, err := strconv.Atoi(name[:separator])
		if err != nil || version <= 0 {
			return nil, fmt.Errorf("invalid migration version in %s", name)
		}
		if previous, exists := versions[version]; exists {
			return nil, fmt.Errorf("migration version %d is duplicated by %s and %s", version, previous, name)
		}
		body, err := fs.ReadFile(migrationFS, path)
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", name, err)
		}
		if strings.TrimSpace(string(body)) == "" {
			return nil, fmt.Errorf("migration %s is empty", name)
		}
		versions[version] = name
		result = append(result, migration{version: version, name: name, sql: string(body)})
	}
	return result, nil
}

func appliedMigrations(ctx context.Context, database *sql.DB) (map[int]string, error) {
	rows, err := database.QueryContext(ctx, "SELECT version, name FROM schema_migrations ORDER BY version")
	if err != nil {
		return nil, fmt.Errorf("list applied migrations: %w", err)
	}
	defer rows.Close()
	result := make(map[int]string)
	for rows.Next() {
		var version int
		var name string
		if err := rows.Scan(&version, &name); err != nil {
			return nil, fmt.Errorf("scan applied migration: %w", err)
		}
		result[version] = name
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read applied migrations: %w", err)
	}
	return result, nil
}
