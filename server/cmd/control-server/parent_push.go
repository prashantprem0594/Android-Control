package main

import (
	"appcontrol/server/internal/config"
	"appcontrol/server/internal/parentpush"
	"appcontrol/server/internal/wakeup"
	"database/sql"
	"io"
	"os"
)

func configureParentPush(db *sql.DB, settings config.Config) (*parentpush.Store, error) {
	if settings.ParentWakeupTokenKeyPath == "" {
		return nil, nil
	}
	file, err := os.Open(settings.ParentWakeupTokenKeyPath)
	if err != nil {
		return nil, parentpush.ErrUnavailable
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, parentpush.ErrUnavailable
	}
	key, err := io.ReadAll(io.LimitReader(file, 33))
	if err != nil || len(key) != 32 {
		clear(key)
		return nil, parentpush.ErrUnavailable
	}
	defer clear(key)
	store, err := parentpush.NewStore(db, key)
	if err != nil {
		return nil, parentpush.ErrUnavailable
	}
	return store, nil
}

func configureParentPushSender(settings config.Config) (parentpush.Sender, error) {
	if !settings.ParentWakeupSenderEnabled {
		return nil, nil
	}
	if settings.ParentWakeupTokenKeyPath == "" || settings.ParentWakeupServiceAccountPath == "" {
		return nil, parentpush.ErrNotConfigured
	}
	file, err := os.Open(settings.ParentWakeupServiceAccountPath)
	if err != nil {
		return nil, parentpush.ErrSenderUnavailable
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, parentpush.ErrSenderUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(file, 64*1024+1))
	if err != nil || len(raw) > 64*1024 {
		clear(raw)
		return nil, parentpush.ErrSenderUnavailable
	}
	defer clear(raw)
	account, err := wakeup.NewServiceAccount(raw, nil)
	if err != nil {
		return nil, parentpush.ErrSenderUnavailable
	}
	sender, err := parentpush.NewFCMSender(account.Project(), account.AccessToken, nil)
	if err != nil {
		return nil, parentpush.ErrSenderUnavailable
	}
	return sender, nil
}
