package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRotationPreflightTrustHostnameValidityAndOverlap(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	makeKey := func() *ecdsa.PrivateKey {
		t.Helper()
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		return key
	}
	serial := int64(0)
	issue := func(template, parent *x509.Certificate, key, issuer *ecdsa.PrivateKey) *x509.Certificate {
		t.Helper()
		serial++
		template.SerialNumber = big.NewInt(serial)
		if parent == nil {
			parent = template
			issuer = key
		}
		der, err := x509.CreateCertificate(rand.Reader, template, parent, &key.PublicKey, issuer)
		if err != nil {
			t.Fatal(err)
		}
		certificate, err := x509.ParseCertificate(der)
		if err != nil {
			t.Fatal(err)
		}
		return certificate
	}
	caTemplate := func(name string) *x509.Certificate {
		return &x509.Certificate{Subject: pkix.Name{CommonName: name}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(48 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	}
	rootKey, intermediateKey := makeKey(), makeKey()
	root := issue(caTemplate("Fixture root"), nil, rootKey, nil)
	intermediate := issue(caTemplate("Fixture intermediate"), root, intermediateKey, rootKey)
	roots := x509.NewCertPool()
	roots.AddCert(root)
	leafTemplate := func() *x509.Certificate {
		return &x509.Certificate{DNSNames: []string{"rotation.example.test"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour), ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, KeyUsage: x509.KeyUsageDigitalSignature}
	}
	currentKey, backupKey := makeKey(), makeKey()
	current := issue(leafTemplate(), intermediate, currentKey, intermediateKey)
	backup := issue(leafTemplate(), intermediate, backupKey, intermediateKey)
	chain := func(leaf *x509.Certificate) []*x509.Certificate { return []*x509.Certificate{leaf, intermediate} }
	result, err := validateRotationCertificates("ROTATION.example.test", chain(current), chain(backup), roots, now)
	if err != nil || result.Hostname != "rotation.example.test" || result.CurrentPin == result.BackupPin || result.CurrentPin == "" || !result.CurrentNotAfter.Equal(current.NotAfter) {
		t.Fatal("valid rotation rejected or public metadata incorrect")
	}
	renewed := issue(leafTemplate(), intermediate, currentKey, intermediateKey)
	expiredTemplate := leafTemplate()
	expiredTemplate.NotAfter = now.Add(-time.Minute)
	futureTemplate := leafTemplate()
	futureTemplate.NotBefore = now.Add(time.Hour)
	clientTemplate := leafTemplate()
	clientTemplate.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	for name, pair := range map[string][2][]*x509.Certificate{
		"same key":             {chain(current), chain(renewed)},
		"expired backup":       {chain(current), chain(issue(expiredTemplate, intermediate, backupKey, intermediateKey))},
		"future backup":        {chain(current), chain(issue(futureTemplate, intermediate, backupKey, intermediateKey))},
		"client-only backup":   {chain(current), chain(issue(clientTemplate, intermediate, backupKey, intermediateKey))},
		"missing intermediate": {{current}, chain(backup)},
		"CA as leaf":           {{intermediate, root}, chain(backup)},
		"empty chain":          {nil, chain(backup)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := validateRotationCertificates("rotation.example.test", pair[0], pair[1], roots, now); err == nil {
				t.Fatal("invalid rotation accepted")
			}
		})
	}
	for _, host := range []string{"other.example.test", "https://rotation.example.test", "rotation.example.test:443", "rotation.example.test.", "127.0.0.1", "999.999.999.999", "-bad.example", "bad_.example", ""} {
		if _, err := validateRotationCertificates(host, chain(current), chain(backup), roots, now); err == nil {
			t.Fatal("invalid or mismatching hostname accepted")
		}
	}
	if _, err := validateRotationCertificates("rotation.example.test", []*x509.Certificate{current, intermediate, root}, []*x509.Certificate{backup, intermediate, root}, x509.NewCertPool(), now); err == nil {
		t.Fatal("chain-supplied root became trusted")
	}
	currentPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: current.Raw})
	intermediatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: intermediate.Raw})
	file := filepath.Join(t.TempDir(), "chain.pem")
	valid := append(append([]byte(nil), currentPEM...), intermediatePEM...)
	if err := os.WriteFile(file, valid, 0600); err != nil {
		t.Fatal(err)
	}
	parsed, err := readCertificateChain(file)
	if err != nil || len(parsed) != 2 {
		t.Fatal("leaf-first bundle rejected")
	}
	malformedPrefix := append([]byte("-----BEGIN CERTIFICATE-----\ninvalid\n-----END CERTIFICATE-----\n"), valid...)
	for _, invalid := range [][]byte{malformedPrefix, nil, []byte("garbage"), append([]byte("garbage\n"), valid...), append(append([]byte(nil), valid...), []byte("garbage")...), append(append([]byte(nil), valid...), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("fixture")})...), bytes.Repeat(currentPEM, 17), bytes.Repeat([]byte("x"), (1<<20)+1)} {
		if err := os.WriteFile(file, invalid, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := readCertificateChain(file); err == nil {
			t.Fatal("invalid bundle accepted")
		}
	}
	// The production entry point cannot use caller-supplied trust anchors.
	if err := os.WriteFile(file, valid, 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := run(context.Background(), []string{"tls-preflight", "-hostname", "rotation.example.test", "-certificate", file, "-backup-certificate", file}, &output); err == nil || output.Len() != 0 {
		t.Fatal("untrusted CLI fixture accepted or emitted partial metadata")
	}
}

func TestRotationPreflightRejectsMixedAndIncompleteArguments(t *testing.T) {
	for _, args := range [][]string{
		{"tls-preflight"},
		{"tls-preflight", "-hostname", "rotation.example.test", "-certificate", "missing"},
		{"tls-preflight", "-hostname", "rotation.example.test", "-certificate", "missing", "-backup-certificate", "missing", "-db", "unexpected"},
		{"tls-pin", "-certificate", "missing", "-hostname", "rotation.example.test"},
		{"keygen", "-key-file", "unused", "-backup-certificate", "missing"},
	} {
		var output bytes.Buffer
		if err := run(context.Background(), args, &output); err == nil || output.Len() != 0 {
			t.Fatal("mixed/incomplete preflight arguments accepted")
		}
	}
}
