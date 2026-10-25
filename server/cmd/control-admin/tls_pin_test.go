package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCertificatePinDerivesPublicSPKI(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	certificate := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), DNSNames: []string{"fixture.example.test"}}
	der, err := x509.CreateCertificate(rand.Reader, certificate, certificate, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	encoded := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	path := filepath.Join(t.TempDir(), "leaf.pem")
	if err := os.WriteFile(path, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	spki, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(spki)
	expected := base64.StdEncoding.EncodeToString(digest[:])
	var output bytes.Buffer
	if err := run(context.Background(), []string{"tls-pin", "-certificate", path}, &output); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(output.String()) != expected {
		t.Fatal("pin did not hash SPKI")
	}
	certificate.SerialNumber = big.NewInt(2)
	renewed, err := x509.CreateCertificate(rand.Reader, certificate, certificate, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(der, renewed) {
		t.Fatal("renewal fixture did not change certificate")
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: renewed}), 0600); err != nil {
		t.Fatal(err)
	}
	if pin, err := certificatePin(path); err != nil || pin != expected {
		t.Fatal("renewal changed pin for the same public key")
	}
	backupKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	backup, err := x509.CreateCertificate(rand.Reader, certificate, certificate, &backupKey.PublicKey, backupKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: backup}), 0600); err != nil {
		t.Fatal(err)
	}
	if pin, err := certificatePin(path); err != nil || pin == expected {
		t.Fatal("independent backup key did not change pin")
	}
	malformedPrefix := append([]byte("-----BEGIN CERTIFICATE-----\ninvalid\n-----END CERTIFICATE-----\n"), encoded...)
	for _, invalid := range [][]byte{malformedPrefix, append(append([]byte(nil), encoded...), encoded...), []byte("private-invalid-fixture"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("fixture")}), bytes.Repeat([]byte("x"), (1<<20)+1)} {
		if err := os.WriteFile(path, invalid, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := certificatePin(path); err == nil {
			t.Fatal("invalid certificate input accepted")
		}
	}
}
