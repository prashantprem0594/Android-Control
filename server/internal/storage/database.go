package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

func Open(ctx context.Context, path string) (*sql.DB, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("database path is required")
	}

	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve database path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	database, err := openRaw(ctx, absolutePath)
	if err != nil {
		return nil, err
	}
	closeOnError := func(openErr error) (*sql.DB, error) {
		_ = database.Close()
		return nil, openErr
	}

	if err := Migrate(ctx, database); err != nil {
		return closeOnError(fmt.Errorf("migrate database: %w", err))
	}
	if err := IntegrityCheck(ctx, database); err != nil {
		return closeOnError(fmt.Errorf("verify database: %w", err))
	}
	return database, nil
}

// OpenExisting verifies an existing database without running migrations. This
// prevents administration commands from silently creating an empty database.
func OpenExisting(ctx context.Context, path string) (*sql.DB, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("an existing regular database file is required")
	}
	database, err := openRaw(ctx, path)
	if err != nil {
		return nil, err
	}
	if err := IntegrityCheck(ctx, database); err != nil {
		_ = database.Close()
		return nil, err
	}
	return database, nil
}

func openRaw(ctx context.Context, path string) (*sql.DB, error) {
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)

	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("ping SQLite database: %w", err)
	}

	for _, statement := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA synchronous = FULL",
		"PRAGMA busy_timeout = 5000",
	} {
		if _, err := database.ExecContext(ctx, statement); err != nil {
			_ = database.Close()
			return nil, fmt.Errorf("configure SQLite: %w", err)
		}
	}

	var journalMode string
	if err := database.QueryRowContext(ctx, "PRAGMA journal_mode = WAL").Scan(&journalMode); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("enable SQLite WAL: %w", err)
	}
	if !strings.EqualFold(journalMode, "wal") {
		_ = database.Close()
		return nil, fmt.Errorf("enable SQLite WAL: database reported %q", journalMode)
	}
	return database, nil
}
