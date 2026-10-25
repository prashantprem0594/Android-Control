package storage

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	backupMagic     = "ACBKP002"
	backupChunkSize = 64 * 1024
)

func CreateEncryptedBackup(ctx context.Context, database *sql.DB, destination string, key []byte) error {
	if database == nil {
		return fmt.Errorf("database is required")
	}
	if err := validateBackupArguments(destination, key); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := os.Stat(destination); err == nil {
		return fmt.Errorf("backup destination already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect backup destination: %w", err)
	}

	directory := filepath.Dir(destination)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create backup directory: %w", err)
	}
	plaintext, err := os.CreateTemp(directory, ".app-control-snapshot-*.db")
	if err != nil {
		return fmt.Errorf("create temporary snapshot: %w", err)
	}
	plaintextPath := plaintext.Name()
	if err := plaintext.Close(); err != nil {
		_ = os.Remove(plaintextPath)
		return fmt.Errorf("close temporary snapshot: %w", err)
	}
	defer os.Remove(plaintextPath)

	if _, err := database.ExecContext(ctx, "VACUUM INTO ?", plaintextPath); err != nil {
		return fmt.Errorf("create consistent SQLite snapshot: %w", err)
	}
	if err := verifyDatabaseFile(ctx, plaintextPath); err != nil {
		return fmt.Errorf("verify SQLite snapshot: %w", err)
	}

	input, err := os.Open(plaintextPath)
	if err != nil {
		return fmt.Errorf("open SQLite snapshot: %w", err)
	}
	defer input.Close()
	encrypted, err := os.CreateTemp(directory, ".app-control-encrypted-*.tmp")
	if err != nil {
		return fmt.Errorf("create encrypted backup: %w", err)
	}
	encryptedPath := encrypted.Name()
	defer os.Remove(encryptedPath)

	if err := encryptBackup(contextWriter{ctx, encrypted}, contextReader{ctx, input}, key); err != nil {
		_ = encrypted.Close()
		return err
	}
	if err := encrypted.Sync(); err != nil {
		_ = encrypted.Close()
		return fmt.Errorf("sync encrypted backup: %w", err)
	}
	if err := encrypted.Close(); err != nil {
		return fmt.Errorf("close encrypted backup: %w", err)
	}
	if err := os.Chmod(encryptedPath, 0o600); err != nil {
		return fmt.Errorf("protect encrypted backup: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := publishNoReplace(encryptedPath, destination); err != nil {
		return fmt.Errorf("publish encrypted backup: %w", err)
	}
	return nil
}

func RestoreEncryptedBackup(ctx context.Context, source, destination string, key []byte) error {
	if err := validateBackupArguments(source, key); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(destination) == "" {
		return fmt.Errorf("restore destination is required")
	}
	if filepath.Clean(source) == filepath.Clean(destination) {
		return fmt.Errorf("backup source and restore destination must differ")
	}
	if _, err := os.Stat(destination); err == nil {
		return fmt.Errorf("restore destination already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect restore destination: %w", err)
	}

	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open encrypted backup: %w", err)
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("regular backup file is required")
	}
	directory := filepath.Dir(destination)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create restore directory: %w", err)
	}
	output, err := os.CreateTemp(directory, ".app-control-restore-*.db")
	if err != nil {
		return fmt.Errorf("create restore file: %w", err)
	}
	outputPath := output.Name()
	defer os.Remove(outputPath)

	if err := decryptBackup(contextWriter{ctx, output}, contextReader{ctx, input}, key); err != nil {
		_ = output.Close()
		return err
	}
	if err := output.Sync(); err != nil {
		_ = output.Close()
		return fmt.Errorf("sync restored database: %w", err)
	}
	if err := output.Close(); err != nil {
		return fmt.Errorf("close restored database: %w", err)
	}
	if err := verifyDatabaseFile(ctx, outputPath); err != nil {
		return fmt.Errorf("verify restored database: %w", err)
	}
	if err := recordRecovery(ctx, outputPath); err != nil {
		return fmt.Errorf("audit restored database: %w", err)
	}
	if err := os.Chmod(outputPath, 0o600); err != nil {
		return fmt.Errorf("protect restored database: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := publishNoReplace(outputPath, destination); err != nil {
		return fmt.Errorf("publish restored database: %w", err)
	}
	return nil
}

// VerifyEncryptedBackup authenticates a full backup and checks its SQLite
// contents without running migrations, recording recovery, or publishing a DB.
// scratchDirectory must be a private, operator-controlled directory.
func VerifyEncryptedBackup(ctx context.Context, source, scratchDirectory string, key []byte) error {
	if err := validateBackupArguments(source, key); err != nil {
		return err
	}
	if strings.TrimSpace(scratchDirectory) == "" {
		return fmt.Errorf("private scratch directory is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open encrypted backup: %w", err)
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("regular backup file is required")
	}
	if err := os.MkdirAll(scratchDirectory, 0700); err != nil {
		return fmt.Errorf("create private scratch directory: %w", err)
	}
	output, err := os.CreateTemp(scratchDirectory, ".app-control-verify-*.db")
	if err != nil {
		return fmt.Errorf("create verification snapshot: %w", err)
	}
	path := output.Name()
	defer os.Remove(path)
	if err := decryptBackup(contextWriter{ctx, output}, contextReader{ctx, input}, key); err != nil {
		_ = output.Close()
		return err
	}
	if err := output.Close(); err != nil {
		return fmt.Errorf("close verification snapshot: %w", err)
	}
	return verifyDatabaseFile(ctx, path)
}

// Cancellation is checked between chunks and writes. A blocked filesystem
// syscall is still controlled by the operating system, rather than preempted.
type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

type contextWriter struct {
	ctx    context.Context
	writer io.Writer
}

func (w contextWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	return w.writer.Write(p)
}

// Both files are in the same directory. Linking publishes the complete staging
// file atomically and fails if the destination exists, including a racing writer
// or dangling symlink. The caller removes its private staging name afterwards.
// Filesystems without hard links fail closed; never fall back to overwriting.
func publishNoReplace(staging, destination string) error {
	return os.Link(staging, destination)
}

// Record in the private staging database before publishing it. Checkpointing is
// required because only the main database file is renamed into place.
func recordRecovery(ctx context.Context, path string) error {
	db, err := Open(ctx, path)
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `INSERT INTO audit_events
		(occurred_at,parent_account_id,actor_type,actor_id,action,target_id)
		VALUES(?,'','system','','database_restored','')`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	var busy, frames, checkpointed int
	if err := db.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &frames, &checkpointed); err != nil {
		return err
	}
	if busy != 0 {
		return fmt.Errorf("recovery audit checkpoint is busy")
	}
	return db.Close()
}

func validateBackupArguments(path string, key []byte) error {
	if path == "" {
		return fmt.Errorf("backup path is required")
	}
	if len(key) != 32 {
		return fmt.Errorf("backup key must contain exactly 32 bytes")
	}
	return nil
}

func verifyDatabaseFile(ctx context.Context, path string) error {
	database, err := openRaw(ctx, path)
	if err != nil {
		return err
	}
	defer database.Close()
	return IntegrityCheck(ctx, database)
}

func encryptBackup(destination io.Writer, source io.Reader, key []byte) error {
	aead, err := backupCipher(key)
	if err != nil {
		return err
	}
	noncePrefix := make([]byte, 8)
	if _, err := io.ReadFull(rand.Reader, noncePrefix); err != nil {
		return fmt.Errorf("generate backup nonce: %w", err)
	}
	if _, err := io.WriteString(destination, backupMagic); err != nil {
		return fmt.Errorf("write backup header: %w", err)
	}
	if err := binary.Write(destination, binary.BigEndian, uint32(backupChunkSize)); err != nil {
		return fmt.Errorf("write backup chunk size: %w", err)
	}
	if _, err := destination.Write(noncePrefix); err != nil {
		return fmt.Errorf("write backup nonce prefix: %w", err)
	}

	buffer := make([]byte, backupChunkSize)
	var counter uint32
	for ; ; counter++ {
		if counter == math.MaxUint32 {
			return fmt.Errorf("backup exceeds supported size")
		}
		read, readErr := io.ReadFull(source, buffer)
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil && !errors.Is(readErr, io.ErrUnexpectedEOF) {
			return fmt.Errorf("read SQLite snapshot: %w", readErr)
		}
		plaintextLength := uint32(read)
		if err := binary.Write(destination, binary.BigEndian, plaintextLength); err != nil {
			return fmt.Errorf("write backup record length: %w", err)
		}
		nonce := backupNonce(noncePrefix, counter)
		ciphertext := aead.Seal(nil, nonce, buffer[:read], backupRecordAAD(counter, plaintextLength, backupChunkSize))
		if _, err := destination.Write(ciphertext); err != nil {
			return fmt.Errorf("write encrypted backup record: %w", err)
		}
		if errors.Is(readErr, io.ErrUnexpectedEOF) {
			counter++
			break
		}
	}
	if counter == math.MaxUint32 {
		return fmt.Errorf("backup exceeds supported size")
	}
	if err := binary.Write(destination, binary.BigEndian, uint32(0)); err != nil {
		return err
	}
	// A unique nonce authenticates the final record count. A plain zero marker
	// would let an attacker discard complete records and forge an earlier end.
	_, err = destination.Write(aead.Seal(nil, backupNonce(noncePrefix, counter), nil, backupRecordAAD(counter, 0, backupChunkSize)))
	return err
}

func decryptBackup(destination io.Writer, source io.Reader, key []byte) error {
	aead, err := backupCipher(key)
	if err != nil {
		return err
	}
	header := make([]byte, len(backupMagic))
	if _, err := io.ReadFull(source, header); err != nil {
		return fmt.Errorf("read encrypted backup header: %w", err)
	}
	if string(header) != backupMagic {
		return fmt.Errorf("invalid encrypted backup header")
	}
	var chunkSize uint32
	if err := binary.Read(source, binary.BigEndian, &chunkSize); err != nil {
		return fmt.Errorf("read encrypted backup chunk size: %w", err)
	}
	if chunkSize == 0 || chunkSize > 1024*1024 {
		return fmt.Errorf("invalid encrypted backup chunk size")
	}
	noncePrefix := make([]byte, 8)
	if _, err := io.ReadFull(source, noncePrefix); err != nil {
		return fmt.Errorf("read encrypted backup nonce: %w", err)
	}

	for counter := uint32(0); ; counter++ {
		if counter == math.MaxUint32 {
			return fmt.Errorf("encrypted backup exceeds supported size")
		}
		var plaintextLength uint32
		if err := binary.Read(source, binary.BigEndian, &plaintextLength); err != nil {
			return fmt.Errorf("read encrypted backup record length: %w", err)
		}
		if plaintextLength == 0 {
			tag := make([]byte, aead.Overhead())
			if _, err := io.ReadFull(source, tag); err != nil {
				return fmt.Errorf("read encrypted backup final tag: %w", err)
			}
			if _, err := aead.Open(nil, backupNonce(noncePrefix, counter), tag, backupRecordAAD(counter, 0, chunkSize)); err != nil {
				return fmt.Errorf("authenticate encrypted backup end: %w", err)
			}
			var trailing [1]byte
			if read, err := source.Read(trailing[:]); err != io.EOF || read != 0 {
				return fmt.Errorf("encrypted backup has trailing data")
			}
			return nil
		}
		if plaintextLength > chunkSize {
			return fmt.Errorf("encrypted backup record exceeds chunk size")
		}
		ciphertext := make([]byte, int(plaintextLength)+aead.Overhead())
		if _, err := io.ReadFull(source, ciphertext); err != nil {
			return fmt.Errorf("read encrypted backup record: %w", err)
		}
		plaintext, err := aead.Open(nil, backupNonce(noncePrefix, counter), ciphertext, backupRecordAAD(counter, plaintextLength, chunkSize))
		if err != nil {
			return fmt.Errorf("authenticate encrypted backup record: %w", err)
		}
		if _, err := destination.Write(plaintext); err != nil {
			return fmt.Errorf("write restored database: %w", err)
		}
	}
}

func backupCipher(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("backup key must contain exactly 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create backup cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create backup AEAD: %w", err)
	}
	return aead, nil
}

func backupNonce(prefix []byte, counter uint32) []byte {
	nonce := make([]byte, 12)
	copy(nonce, prefix)
	binary.BigEndian.PutUint32(nonce[8:], counter)
	return nonce
}

func backupRecordAAD(counter, plaintextLength, chunkSize uint32) []byte {
	aad := make([]byte, len(backupMagic)+12)
	copy(aad, backupMagic)
	binary.BigEndian.PutUint32(aad[len(backupMagic):], counter)
	binary.BigEndian.PutUint32(aad[len(backupMagic)+4:], plaintextLength)
	binary.BigEndian.PutUint32(aad[len(backupMagic)+8:], chunkSize)
	return aad
}
