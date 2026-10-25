package storage

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestBackupAuthenticatesCompletenessAndHeader(t *testing.T) {
	key := bytes.Repeat([]byte{37}, 32)
	for _, size := range []int{0, 1, backupChunkSize, backupChunkSize + 5} {
		plain := bytes.Repeat([]byte("x"), size)
		var encrypted, restored bytes.Buffer
		if err := encryptBackup(&encrypted, bytes.NewReader(plain), key); err != nil {
			t.Fatal(err)
		}
		if err := decryptBackup(&restored, bytes.NewReader(encrypted.Bytes()), key); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(plain, restored.Bytes()) {
			t.Fatal("round trip mismatch")
		}
		original := encrypted.Bytes()
		for _, cut := range []int{len(original) - 1, len(original) - 16, len(original) - 20} {
			if err := decryptBackup(&bytes.Buffer{}, bytes.NewReader(original[:cut]), key); err == nil {
				t.Fatal("accepted truncated end")
			}
		}
		forged := append([]byte(nil), original[:20]...)
		forged = append(forged, 0, 0, 0, 0)
		forged = append(forged, original[len(original)-16:]...)
		if size > 0 && decryptBackup(&bytes.Buffer{}, bytes.NewReader(forged), key) == nil {
			t.Fatal("accepted forged early end")
		}
		changed := append([]byte(nil), original...)
		binary.BigEndian.PutUint32(changed[8:12], backupChunkSize+1)
		if decryptBackup(&bytes.Buffer{}, bytes.NewReader(changed), key) == nil {
			t.Fatal("accepted altered chunk size")
		}
		changed = append([]byte(nil), original...)
		copy(changed[:8], "ACBKP001")
		if decryptBackup(&bytes.Buffer{}, bytes.NewReader(changed), key) == nil {
			t.Fatal("accepted legacy format")
		}
	}
}

type cancelOnRead struct {
	reader io.Reader
	cancel context.CancelFunc
}

func (r cancelOnRead) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.cancel()
	return n, err
}

func TestBackupStreamsHonorCancellationBetweenOperations(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	plain := bytes.Repeat([]byte{42}, 2*backupChunkSize)
	var encrypted bytes.Buffer
	if err := encryptBackup(&encrypted, bytes.NewReader(plain), key); err != nil {
		t.Fatal(err)
	}
	for _, encrypt := range []bool{true, false} {
		ctx, cancel := context.WithCancel(context.Background())
		var input io.Reader = bytes.NewReader(plain)
		if !encrypt {
			input = bytes.NewReader(encrypted.Bytes())
		}
		reader := contextReader{ctx, cancelOnRead{input, cancel}}
		var result bytes.Buffer
		writer := contextWriter{ctx, &result}
		var err error
		if encrypt {
			err = encryptBackup(writer, reader, key)
		} else {
			err = decryptBackup(writer, reader, key)
		}
		cancel()
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("stream ignored cancellation: %v", err)
		}
		if !encrypt && result.Len() != 0 {
			t.Fatal("decryption wrote plaintext after cancellation")
		}
	}
}

func TestVerifyRejectsTruncatedAndNonDatabaseBackups(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	key := bytes.Repeat([]byte{6}, 32)
	var encrypted bytes.Buffer
	if err := encryptBackup(&encrypted, bytes.NewReader([]byte("authenticated but not a database")), key); err != nil {
		t.Fatal(err)
	}
	source, scratch := filepath.Join(dir, "backup"), filepath.Join(dir, "scratch")
	for _, data := range [][]byte{encrypted.Bytes(), encrypted.Bytes()[:encrypted.Len()-1]} {
		if err := os.WriteFile(source, data, 0600); err != nil {
			t.Fatal(err)
		}
		if VerifyEncryptedBackup(ctx, source, scratch, key) == nil {
			t.Fatal("invalid backup verified")
		}
		entries, err := os.ReadDir(scratch)
		if err != nil || len(entries) != 0 {
			t.Fatal("failed verification left private files")
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if !errors.Is(VerifyEncryptedBackup(canceled, source, scratch, key), context.Canceled) {
		t.Fatal("verification ignored canceled context")
	}
}

func TestPublicationNeverReplacesDestination(t *testing.T) {
	dir := t.TempDir()
	staging, target := filepath.Join(dir, "staging"), filepath.Join(dir, "target")
	if err := os.WriteFile(staging, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	if publishNoReplace(staging, target) == nil {
		t.Fatal("replaced existing destination")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "existing" {
		t.Fatal("destination changed")
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := publishNoReplace(staging, target); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(staging); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(target)
	if err != nil || string(data) != "new" {
		t.Fatal("published file depended on staging name")
	}
}

func TestCanceledBackupOperationsDoNotCreateDirectories(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dir := t.TempDir()
	key := bytes.Repeat([]byte{9}, 32)
	db, err := Open(context.Background(), filepath.Join(dir, "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, operation := range []string{"backup", "restore", "verify"} {
		t.Run(operation, func(t *testing.T) {
			destinationDir := filepath.Join(dir, operation)
			var err error
			switch operation {
			case "backup":
				err = CreateEncryptedBackup(ctx, db, filepath.Join(destinationDir, "snapshot.acb"), key)
			case "restore":
				err = RestoreEncryptedBackup(ctx, filepath.Join(dir, "missing.acb"), filepath.Join(destinationDir, "restored.db"), key)
			case "verify":
				err = VerifyEncryptedBackup(ctx, filepath.Join(dir, "missing.acb"), destinationDir, key)
			}
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("operation did not reject cancelled context: %v", err)
			}
			if _, err := os.Stat(destinationDir); !os.IsNotExist(err) {
				t.Fatal("cancelled operation created a directory")
			}
		})
	}
}

func TestRestoreRejectsDirectoryBeforeCreatingStaging(t *testing.T) {
	dir := t.TempDir()
	destinationDir := filepath.Join(dir, "restore")
	if err := RestoreEncryptedBackup(context.Background(), dir, filepath.Join(destinationDir, "restored.db"), bytes.Repeat([]byte{3}, 32)); err == nil {
		t.Fatal("restore accepted a directory input")
	}
	if _, err := os.Stat(destinationDir); !os.IsNotExist(err) {
		t.Fatal("invalid input caused restore staging directory creation")
	}
}
