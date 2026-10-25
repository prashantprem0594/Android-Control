package main

import (
	"appcontrol/server/internal/auth"
	"appcontrol/server/internal/storage"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "Parent recovery failed. Check private input, migrated database and operator access.")
		os.Exit(1)
	}
}
func run(ctx context.Context, args []string, in io.Reader, out io.Writer) error {
	flags := flag.NewFlagSet("parent-recovery", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := flags.String("db", "", "existing migrated database")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *path == "" || flags.NArg() != 0 {
		return errors.New("database required")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	raw, err := io.ReadAll(io.LimitReader(in, 8193))
	if err != nil {
		return err
	}
	defer clear(raw)
	if len(raw) > 8192 {
		return errors.New("input too large")
	}
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return errors.New("invalid private input")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("invalid private input")
	}
	if input.Email == "" || input.Password == "" {
		return errors.New("private input required")
	}
	db, err := storage.OpenExisting(ctx, *path)
	if err != nil {
		return err
	}
	defer db.Close()
	var version int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version=14 AND name='0014_parent_password_recovery.sql'").Scan(&version); err != nil {
		return err
	}
	if version != 1 {
		return errors.New("database migration required")
	}
	service, err := auth.NewService(db, auth.Options{})
	if err != nil {
		return err
	}
	if err := service.RecoverPassword(ctx, input.Email, input.Password); err != nil {
		return err
	}
	input.Password = ""
	_, err = fmt.Fprintln(out, "Parent password recovered. Parent sessions and unused enrollment access revoked; active child enrollment preserved.")
	return err
}
