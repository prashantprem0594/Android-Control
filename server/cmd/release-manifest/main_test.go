package main

import (
	"appcontrol/server/internal/release"
	"bytes"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManifestKeygenSignVerifyAndOverwriteRefusal(t *testing.T) {
	directory := t.TempDir()
	privatePath := filepath.Join(directory, "operator.key")
	publicPath := filepath.Join(directory, "operator.pub")
	var output bytes.Buffer
	if run([]string{"keygen", "-private-key", privatePath, "-public-key", publicPath}, &output) != nil {
		t.Fatal("key generation failed")
	}
	seed, err := os.ReadFile(privatePath)
	if err != nil || len(seed) != ed25519.SeedSize {
		t.Fatal("private seed file invalid")
	}
	defer clear(seed)
	der, err := os.ReadFile(publicPath)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		t.Fatal(err)
	}
	public, ok := parsed.(ed25519.PublicKey)
	if !ok || !bytes.Equal(public, ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)) {
		t.Fatal("public/private files differ")
	}
	now := time.Now().UTC()
	origin := "https://updates.example.org"
	certificate := strings.Repeat("a", 64)
	manifest := release.Manifest{Version: 1, Sequence: 3, Origin: origin, IssuedAt: now, ExpiresAt: now.Add(time.Hour)}
	for _, name := range []string{"parent", "child"} {
		manifest.Apps = append(manifest.Apps, release.Artifact{Package: "com.appcontrol." + name, VersionCode: 2, VersionName: "0.1.1", MinSDK: 33, TargetSDK: 37, URL: origin + "/releases/3/" + name + "-release.apk", Bytes: 1024, SHA256: strings.Repeat("b", 64), SigningCertificateSHA256: certificate})
	}
	raw, _ := json.Marshal(manifest)
	input := filepath.Join(directory, "payload.json")
	signed := filepath.Join(directory, "manifest.json")
	if os.WriteFile(input, raw, 0600) != nil {
		t.Fatal("fixture payload write failed")
	}
	if run([]string{"sign", "-input", input, "-private-key", privatePath, "-out", signed}, &output) != nil {
		t.Fatal("manifest sign failed")
	}
	args := []string{"verify", "-input", signed, "-public-key", publicPath, "-origin", origin, "-certificate-sha256", certificate, "-minimum-sequence", "3"}
	if run(args, &output) != nil {
		t.Fatal("trusted manifest verify failed")
	}
	before, err := os.ReadFile(signed)
	if err != nil {
		t.Fatal(err)
	}
	if run([]string{"sign", "-input", input, "-private-key", privatePath, "-out", signed}, &output) == nil {
		t.Fatal("existing manifest overwritten")
	}
	after, err := os.ReadFile(signed)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("overwrite refusal altered artifact")
	}
	if run([]string{"keygen", "-private-key", privatePath, "-public-key", publicPath}, &output) == nil {
		t.Fatal("existing key overwritten")
	}
	if bytes.Contains(output.Bytes(), seed) || bytes.Contains(output.Bytes(), []byte(base64.StdEncoding.EncodeToString(seed))) {
		t.Fatal("private key exposed through CLI output")
	}
}

func TestPublicFixtureRetainsNoPrivateKeyAndVerifies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture.json")
	var output bytes.Buffer
	if run([]string{"fixture", "-out", path}, &output) != nil {
		t.Fatal("public fixture creation failed")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		FixtureOnly bool            `json:"fixtureOnly"`
		PublicKey   string          `json:"publicKey"`
		Manifest    json.RawMessage `json:"manifest"`
		Origin      string          `json:"origin"`
		Certificate string          `json:"certificateSHA256"`
		Now         time.Time       `json:"now"`
	}
	if json.Unmarshal(raw, &fixture) != nil || !fixture.FixtureOnly || strings.Contains(string(raw), "private") {
		t.Fatal("invalid public fixture")
	}
	der, err := base64.StdEncoding.Strict().DecodeString(fixture.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := release.Verify(fixture.Manifest, parsed.(ed25519.PublicKey), fixture.Origin, fixture.Certificate, 1, fixture.Now); err != nil {
		t.Fatal("public fixture did not verify")
	}
}
