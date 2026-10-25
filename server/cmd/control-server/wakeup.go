package main

import (
	"appcontrol/server/internal/config"
	"appcontrol/server/internal/control"
	"appcontrol/server/internal/wakeup"
	"database/sql"
	"io"
	"os"
)

func configureWakeup(db *sql.DB, settings config.Config) (control.Wakeup, *control.WakeupTokens, error) {
	if settings.WakeupServiceAccountPath == "" && settings.WakeupTokenKeyPath == "" {
		return nil, nil, nil
	}
	read := func(path string, limit int64) ([]byte, error) {
		file, err := os.Open(path)
		if err != nil {
			return nil, wakeup.ErrUnavailable
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			return nil, wakeup.ErrUnavailable
		}
		raw, err := io.ReadAll(io.LimitReader(file, limit+1))
		if err != nil || int64(len(raw)) > limit {
			return nil, wakeup.ErrUnavailable
		}
		return raw, nil
	}
	raw, err := read(settings.WakeupServiceAccountPath, 64*1024)
	if err != nil {
		return nil, nil, wakeup.ErrUnavailable
	}
	defer clear(raw)
	account, err := wakeup.NewServiceAccount(raw, nil)
	if err != nil {
		return nil, nil, wakeup.ErrUnavailable
	}
	key, err := read(settings.WakeupTokenKeyPath, 32)
	if err != nil {
		return nil, nil, wakeup.ErrUnavailable
	}
	defer clear(key)
	store, err := control.NewWakeupTokens(db, key)
	if err != nil {
		return nil, nil, wakeup.ErrUnavailable
	}
	provider, err := wakeup.NewFCM(account.Project(), account.AccessToken, store.Registration, nil)
	if err != nil {
		return nil, nil, wakeup.ErrUnavailable
	}
	return provider, store, nil
}
