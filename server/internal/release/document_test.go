package release

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"testing"
)

func TestDocumentSnapshotsBytesAndRechecksExpiry(t *testing.T) {
	manifest, now := fixture()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := Sign(manifest, private, now)
	if err != nil {
		t.Fatal(err)
	}
	original := append([]byte(nil), raw...)
	der, err := x509.MarshalPKIXPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	document, err := NewDocument(raw, der, manifest.Origin, manifest.Apps[0].SigningCertificateSHA256, manifest.Sequence)
	if err != nil {
		t.Fatal(err)
	}
	raw[0] ^= 1
	der[0] ^= 1
	first, err := document.Bytes(now)
	if err != nil || !bytes.Equal(first, original) {
		t.Fatal("caller mutation changed signed snapshot")
	}
	first[0] ^= 1
	second, err := document.Bytes(now)
	if err != nil || !bytes.Equal(second, original) {
		t.Fatal("response mutation changed signed snapshot")
	}
	if _, err := document.Bytes(manifest.ExpiresAt); err != ErrInvalid {
		t.Fatal("long-running host served expired metadata")
	}
}
