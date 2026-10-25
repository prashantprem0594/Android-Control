package main

import (
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"appcontrol/server/internal/storage"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		// Storage and flag errors may contain user-supplied paths. Keep console
		// output fixed and never emit key material or SQLite data.
		fmt.Fprintln(os.Stderr, "Administration failed: check arguments, input files, certificate trust, backup key and database integrity.")
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("command required")
	}
	command := args[0]
	if command != "keygen" && command != "backup" && command != "restore" && command != "integrity" && command != "verify" && command != "tls-pin" && command != "tls-preflight" {
		return errors.New("unknown command")
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	dbPath := flags.String("db", "", "existing database path (new destination for restore)")
	backupPath := flags.String("backup", "", "encrypted backup path")
	keyPath := flags.String("key-file", "", "file containing exactly 32 binary key bytes")
	scratchPath := flags.String("scratch-dir", "", "private temporary plaintext directory for verify")
	certificatePath := flags.String("certificate", "", "single PEM certificate for public SPKI pin derivation")
	backupCertificatePath := flags.String("backup-certificate", "", "backup leaf-first PEM certificate chain")
	hostname := flags.String("hostname", "", "deployment DNS hostname for certificate preflight")
	timeout := flags.Duration("timeout", 2*time.Minute, "operation deadline")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *timeout <= 0 || *timeout > time.Hour {
		return errors.New("invalid arguments")
	}
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	if command == "tls-preflight" {
		if *certificatePath == "" || *backupCertificatePath == "" || *hostname == "" || *dbPath != "" || *backupPath != "" || *keyPath != "" || *scratchPath != "" {
			return errors.New("tls-preflight requires hostname and current/backup certificates only")
		}
		return certificatePreflight(ctx, *hostname, *certificatePath, *backupCertificatePath, out)
	}
	if *backupCertificatePath != "" || *hostname != "" {
		return errors.New("rotation inputs are only valid for tls-preflight")
	}
	if command == "tls-pin" {
		if *certificatePath == "" || *dbPath != "" || *backupPath != "" || *keyPath != "" || *scratchPath != "" {
			return errors.New("tls-pin requires certificate only")
		}
		pin, err := certificatePin(*certificatePath)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, pin)
		return err
	}
	if *certificatePath != "" {
		return errors.New("certificate is only valid for tls-pin")
	}
	if command == "verify" {
		if *dbPath != "" || *backupPath == "" || *keyPath == "" || *scratchPath == "" {
			return errors.New("verify requires backup, key-file and scratch-dir only")
		}
		key, err := readKey(*keyPath)
		if err != nil {
			return err
		}
		defer clear(key)
		if err := storage.VerifyEncryptedBackup(ctx, *backupPath, *scratchPath, key); err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, "Encrypted backup authentication and database integrity verified. No database published.")
		return err
	}
	if *scratchPath != "" {
		return errors.New("scratch-dir is only valid for verify")
	}
	if command == "keygen" {
		if *keyPath == "" || *dbPath != "" || *backupPath != "" {
			return errors.New("keygen requires only key-file")
		}
		if err := createKey(*keyPath); err != nil {
			return err
		}
		_, err := fmt.Fprintln(out, "Backup key created. Protect this file separately from backups.")
		return err
	}
	if *dbPath == "" {
		return errors.New("db is required")
	}
	if command == "integrity" {
		if *backupPath != "" || *keyPath != "" {
			return errors.New("integrity requires only db")
		}
		db, err := storage.OpenExisting(ctx, *dbPath)
		if err != nil {
			return err
		}
		if err := db.Close(); err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, "Database integrity verified.")
		return err
	}
	if *backupPath == "" || *keyPath == "" {
		return errors.New("backup and key-file are required")
	}
	key, err := readKey(*keyPath)
	if err != nil {
		return err
	}
	defer clear(key)
	if command == "restore" {
		if err := storage.RestoreEncryptedBackup(ctx, *backupPath, *dbPath, key); err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, "Database restored and recovery audit recorded. Device reconnection remains to be verified.")
		return err
	}
	db, err := storage.OpenExisting(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := storage.CreateEncryptedBackup(ctx, db, *backupPath, key); err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, "Encrypted backup created and source snapshot integrity verified.")
	return err
}

func readKey(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("cannot open key file")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != 32 {
		return nil, errors.New("key file must contain exactly 32 bytes")
	}
	key, err := io.ReadAll(io.LimitReader(f, 33))
	if err != nil || len(key) != 32 {
		clear(key)
		return nil, errors.New("invalid key file")
	}
	return key, nil
}

func createKey(path string) error {
	key := make([]byte, 32)
	defer clear(key)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = f.Close()
		if !ok {
			_ = os.Remove(path)
		}
	}()
	if _, err := f.Write(key); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}
