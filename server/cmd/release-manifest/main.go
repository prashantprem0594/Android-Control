package main

import (
	"appcontrol/server/internal/release"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "Release manifest operation failed. Check inputs, trusted metadata and private operator files.")
		os.Exit(1)
	}
}
func readBounded(path string, maximum int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, release.ErrInvalid
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maximum {
		return nil, release.ErrInvalid
	}
	raw, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil || int64(len(raw)) > maximum {
		return nil, release.ErrInvalid
	}
	return raw, nil
}
func createNew(path string, raw []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return release.ErrInvalid
	}
	_, writeErr := file.Write(raw)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		_ = os.Remove(path)
		return release.ErrInvalid
	}
	return nil
}
func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		return release.ErrInvalid
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var input, keyPath, publicPath, output, origin, certificate string
	var minimum int
	switch args[0] {
	case "keygen":
		flags.StringVar(&keyPath, "private-key", "", "new private seed file")
		flags.StringVar(&publicPath, "public-key", "", "new DER public key file")
	case "sign":
		flags.StringVar(&input, "input", "", "reviewed public manifest payload JSON")
		flags.StringVar(&keyPath, "private-key", "", "private seed file")
		flags.StringVar(&output, "out", "", "new signed manifest file")
	case "verify":
		flags.StringVar(&input, "input", "", "signed manifest")
		flags.StringVar(&publicPath, "public-key", "", "trusted DER public key file")
		flags.StringVar(&origin, "origin", "", "trusted canonical HTTPS origin")
		flags.StringVar(&certificate, "certificate-sha256", "", "trusted APK signer certificate")
		flags.IntVar(&minimum, "minimum-sequence", 0, "lowest permitted release sequence")
	case "fixture":
		flags.StringVar(&output, "out", "", "new public isolated fixture file")
	default:
		return release.ErrInvalid
	}
	if flags.Parse(args[1:]) != nil || flags.NArg() != 0 {
		return release.ErrInvalid
	}
	now := time.Now().UTC()
	switch args[0] {
	case "keygen":
		if keyPath == "" || publicPath == "" {
			return release.ErrInvalid
		}
		privateFull, err := filepath.Abs(keyPath)
		if err != nil {
			return release.ErrInvalid
		}
		publicFull, err := filepath.Abs(publicPath)
		if err != nil || strings.EqualFold(privateFull, publicFull) {
			return release.ErrInvalid
		}
		public, private, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return release.ErrInvalid
		}
		defer clear(private)
		der, err := x509.MarshalPKIXPublicKey(public)
		if err != nil {
			return release.ErrInvalid
		}
		seed := private.Seed()
		defer clear(seed)
		if err := createNew(keyPath, seed, 0600); err != nil {
			return err
		}
		if err := createNew(publicPath, der, 0644); err != nil {
			_ = os.Remove(keyPath)
			return err
		}
		_, err = fmt.Fprintln(out, "Release metadata key files created. Protect and separately back up the private key.")
		return err
	case "sign":
		if input == "" || keyPath == "" || output == "" {
			return release.ErrInvalid
		}
		raw, err := readBounded(input, 32*1024)
		if err != nil {
			return err
		}
		manifest, err := release.Decode(raw)
		if err != nil {
			return err
		}
		seed, err := readBounded(keyPath, ed25519.SeedSize)
		if err != nil || len(seed) != ed25519.SeedSize {
			return release.ErrInvalid
		}
		defer clear(seed)
		private := ed25519.NewKeyFromSeed(seed)
		defer clear(private)
		signed, err := release.Sign(manifest, private, now)
		if err != nil {
			return err
		}
		if err := createNew(output, signed, 0644); err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, "Release metadata signed locally. Nothing published or installed.")
		return err
	case "verify":
		if input == "" || publicPath == "" || origin == "" || certificate == "" {
			return release.ErrInvalid
		}
		raw, err := readBounded(input, 64*1024)
		if err != nil {
			return err
		}
		der, err := readBounded(publicPath, 128)
		if err != nil {
			return err
		}
		parsed, err := x509.ParsePKIXPublicKey(der)
		if err != nil {
			return release.ErrInvalid
		}
		public, ok := parsed.(ed25519.PublicKey)
		if !ok {
			return release.ErrInvalid
		}
		if _, err := release.Verify(raw, public, origin, certificate, minimum, now); err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, "Signed release metadata, origin, APK signer, lifetime and release sequence verified.")
		return err
	case "fixture":
		if output == "" {
			return release.ErrInvalid
		}
		public, private, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return release.ErrInvalid
		}
		defer clear(private)
		now = now.Truncate(time.Second)
		origin = "https://release-manifest-fixture.example.org"
		certificate = strings.Repeat("a", 64)
		manifest := release.Manifest{Version: 1, Sequence: 1, IssuedAt: now, ExpiresAt: now.Add(24 * time.Hour), Origin: origin}
		for _, name := range []string{"parent", "child"} {
			manifest.Apps = append(manifest.Apps, release.Artifact{Package: "com.appcontrol." + name, VersionCode: 2, VersionName: "0.1.1-fixture", MinSDK: 33, TargetSDK: 37, URL: origin + "/releases/1/" + name + "-release.apk", Bytes: 1, SHA256: strings.Repeat("b", 64), SigningCertificateSHA256: certificate})
		}
		signed, err := release.Sign(manifest, private, now)
		if err != nil {
			return err
		}
		der, err := x509.MarshalPKIXPublicKey(public)
		if err != nil {
			return err
		}
		fixture, err := json.Marshal(struct {
			FixtureOnly bool            `json:"fixtureOnly"`
			PublicKey   string          `json:"publicKey"`
			Manifest    json.RawMessage `json:"manifest"`
			Origin      string          `json:"origin"`
			Certificate string          `json:"certificateSHA256"`
			Now         time.Time       `json:"now"`
		}{true, base64.StdEncoding.EncodeToString(der), signed, origin, certificate, now})
		if err != nil {
			return err
		}
		if err := createNew(output, fixture, 0644); err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, "Public isolated release fixture generated; no private signing key retained.")
		return err
	}
	return errors.New("unreachable command")
}
