package release

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func fixture() (Manifest, time.Time) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	certificate := strings.Repeat("a", 64)
	origin := "https://updates.example.org"
	manifest := Manifest{Version: 1, Sequence: 7, IssuedAt: now, ExpiresAt: now.Add(24 * time.Hour), Origin: origin}
	for _, name := range []string{"parent", "child"} {
		manifest.Apps = append(manifest.Apps, Artifact{Package: "com.appcontrol." + name, VersionCode: 2, VersionName: "0.1.1", MinSDK: 33, TargetSDK: 37, URL: origin + "/releases/7/" + name + "-release.apk", Bytes: 1024, SHA256: strings.Repeat("b", 64), SigningCertificateSHA256: certificate})
	}
	return manifest, now
}
func TestSignedManifestBindsOriginSignerSequenceAndExpiry(t *testing.T) {
	manifest, now := fixture()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := Sign(manifest, private, now)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := Verify(raw, public, manifest.Origin, manifest.Apps[0].SigningCertificateSHA256, 7, now)
	if err != nil || verified.Sequence != 7 || len(verified.Apps) != 2 {
		t.Fatal("signed metadata did not verify")
	}
	manifest.Apps[0], manifest.Apps[1] = manifest.Apps[1], manifest.Apps[0]
	second, err := Sign(manifest, private, now)
	if err != nil || string(second) != string(raw) {
		t.Fatal("signing order is not deterministic")
	}
	other, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		key                 ed25519.PublicKey
		origin, certificate string
		minimum             int
		time                time.Time
	}{
		{other, manifest.Origin, strings.Repeat("a", 64), 7, now},
		{public, "https://other.example.org", strings.Repeat("a", 64), 7, now},
		{public, manifest.Origin, strings.Repeat("c", 64), 7, now},
		{public, manifest.Origin, strings.Repeat("a", 64), 8, now},
		{public, manifest.Origin, strings.Repeat("a", 64), 7, manifest.ExpiresAt},
	} {
		if _, err := Verify(raw, test.key, test.origin, test.certificate, test.minimum, test.time); err != ErrInvalid {
			t.Fatal("wrong trust, replay or expired metadata accepted")
		}
	}
	var wrapper envelope
	if json.Unmarshal(raw, &wrapper) != nil {
		t.Fatal("fixture envelope decode failed")
	}
	payload, _ := base64.RawURLEncoding.DecodeString(wrapper.Payload)
	payload[0] ^= 1
	wrapper.Payload = base64.RawURLEncoding.EncodeToString(payload)
	tampered, _ := json.Marshal(wrapper)
	if _, err := Verify(tampered, public, manifest.Origin, strings.Repeat("a", 64), 0, now); err != ErrInvalid {
		t.Fatal("altered signed metadata accepted")
	}
}
<<<<<<< HEAD
func TestChildAndroid9ReleaseMetadata(t *testing.T) {
	manifest, now := fixture()
	manifest.Apps[1].MinSDK = 28
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := Sign(manifest, private, now)
	if err != nil {
		t.Fatal("Android 9 child release rejected", err)
	}
	if _, err := Verify(raw, public, manifest.Origin, manifest.Apps[0].SigningCertificateSHA256, 7, now); err != nil {
		t.Fatal(err)
	}
	manifest.Apps[1].MinSDK = 27
	if manifest.Validate(manifest.Origin, manifest.Apps[0].SigningCertificateSHA256, 7, now) != ErrInvalid {
		t.Fatal("unsupported child SDK accepted")
	}
	manifest.Apps[1].MinSDK = 28
	manifest.Apps[0].MinSDK = 28
	if manifest.Validate(manifest.Origin, manifest.Apps[0].SigningCertificateSHA256, 7, now) != ErrInvalid {
		t.Fatal("unsupported parent SDK accepted")
	}
}
=======
>>>>>>> origin/main
func TestManifestRejectsUnsafeArtifactAndLifetimeMetadata(t *testing.T) {
	mutations := []func(*Manifest){
		func(m *Manifest) { m.Apps[0].URL = "https://other.example.org/releases/7/parent-release.apk" },
		func(m *Manifest) { m.Apps[0].URL = m.Origin + "/releases/7/../parent-release.apk" },
		func(m *Manifest) { m.Apps[0].URL += "?secret=value" },
		func(m *Manifest) { m.Apps[1] = m.Apps[0] },
		func(m *Manifest) { m.Apps[0].Package = "another.app" },
		func(m *Manifest) { m.Apps[0].Bytes = 0 },
		func(m *Manifest) { m.Apps[0].VersionCode = 0 },
		func(m *Manifest) { m.Apps[0].MinSDK = 32 },
		func(m *Manifest) { m.Apps[0].SHA256 = "bad" },
		func(m *Manifest) { m.ExpiresAt = m.IssuedAt.Add(31 * 24 * time.Hour) },
		func(m *Manifest) { m.IssuedAt = m.IssuedAt.Add(6 * time.Minute) },
	}
	for _, mutate := range mutations {
		manifest, now := fixture()
		mutate(&manifest)
		if manifest.Validate(manifest.Origin, strings.Repeat("a", 64), 0, now) != ErrInvalid {
			t.Fatal("unsafe manifest accepted")
		}
	}
	for _, origin := range []string{"http://updates.example.org", "https://updates.example.org/", "https://updates.example.org:443", "https://127.0.0.1", "https://updates.example.org#", "https://u@updates.example.org", "https://updates.example.org?"} {
		manifest, now := fixture()
		manifest.Origin = origin
		if manifest.Validate(origin, strings.Repeat("a", 64), 0, now) != ErrInvalid {
			t.Fatal("noncanonical or unsafe origin accepted")
		}
	}
}
func TestManifestStrictDecodeEvenWithValidSignature(t *testing.T) {
	manifest, now := fixture()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(manifest)
	raw = append(raw[:len(raw)-1], []byte(`,"unexpected":true}`)...)
	signature := ed25519.Sign(private, append([]byte(domain), raw...))
	wrapped, _ := json.Marshal(envelope{1, base64.RawURLEncoding.EncodeToString(raw), base64.RawURLEncoding.EncodeToString(signature)})
	if _, err := Verify(wrapped, public, manifest.Origin, strings.Repeat("a", 64), 0, now); err != ErrInvalid {
		t.Fatal("unknown signed fields accepted")
	}
	if _, err := Decode(append(raw, raw...)); err != ErrInvalid {
		t.Fatal("trailing JSON accepted")
	}
}
