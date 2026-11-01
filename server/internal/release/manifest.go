// Package release signs public update metadata, never device commands.
package release

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const domain = "app-control-release-manifest-v1\n"

var ErrInvalid = errors.New("invalid release manifest")
var hashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var versionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+\-]{0,63}$`)
var labelPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

type Artifact struct {
	Package                  string `json:"package"`
	VersionCode              int    `json:"versionCode"`
	VersionName              string `json:"versionName"`
	MinSDK                   int    `json:"minSdk"`
	TargetSDK                int    `json:"targetSdk"`
	URL                      string `json:"url"`
	Bytes                    int64  `json:"bytes"`
	SHA256                   string `json:"sha256"`
	SigningCertificateSHA256 string `json:"signingCertificateSHA256"`
}
type Manifest struct {
	Version   int        `json:"version"`
	Sequence  int        `json:"sequence"`
	IssuedAt  time.Time  `json:"issuedAt"`
	ExpiresAt time.Time  `json:"expiresAt"`
	Origin    string     `json:"origin"`
	Apps      []Artifact `json:"apps"`
}
type envelope struct {
	Version   int    `json:"version"`
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

func strictJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil || decoder.Decode(new(any)) != io.EOF {
		return ErrInvalid
	}
	return nil
}
func Decode(raw []byte) (Manifest, error) {
	var manifest Manifest
	if len(raw) == 0 || len(raw) > 32*1024 || strictJSON(raw, &manifest) != nil {
		return Manifest{}, ErrInvalid
	}
	return manifest, nil
}
func canonicalOrigin(origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Opaque != "" || parsed.Path != "" || parsed.RawPath != "" || parsed.RawQuery != "" || parsed.ForceQuery || strings.Contains(origin, "#") {
		return false
	}
	host := parsed.Hostname()
	if host != strings.ToLower(host) || len(host) < 1 || len(host) > 253 || !strings.ContainsAny(host, "abcdefghijklmnopqrstuvwxyz") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) < 1 || len(label) > 63 || !labelPattern.MatchString(label) {
			return false
		}
	}
	canonical := "https://" + host
	if port := parsed.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 || number == 443 {
			return false
		}
		canonical += ":" + strconv.Itoa(number)
	}
	return origin == canonical
}
func (manifest Manifest) Validate(origin, certificate string, minimumSequence int, now time.Time) error {
	if manifest.Version != 1 || manifest.Sequence < 1 || manifest.Sequence > 2100000000 || minimumSequence < 0 || manifest.Sequence < minimumSequence || manifest.Origin != origin || !canonicalOrigin(origin) || !hashPattern.MatchString(certificate) {
		return ErrInvalid
	}
	if manifest.IssuedAt.IsZero() || manifest.ExpiresAt.IsZero() || !manifest.ExpiresAt.After(manifest.IssuedAt) || manifest.ExpiresAt.Sub(manifest.IssuedAt) > 30*24*time.Hour || !now.Before(manifest.ExpiresAt) || manifest.IssuedAt.After(now.Add(5*time.Minute)) {
		return ErrInvalid
	}
	if len(manifest.Apps) != 2 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, app := range manifest.Apps {
		name := ""
		switch app.Package {
		case "com.appcontrol.parent":
			name = "parent"
		case "com.appcontrol.child":
			name = "child"
		default:
			return ErrInvalid
		}
<<<<<<< HEAD
		minimumSDK := 33
		if name == "child" {
			minimumSDK = 28
		}
		if seen[app.Package] || app.VersionCode < 1 || app.VersionCode > 2100000000 || !versionPattern.MatchString(app.VersionName) || app.MinSDK < minimumSDK || app.TargetSDK < 33 || app.TargetSDK < app.MinSDK || app.TargetSDK > 100 || app.Bytes < 1 || app.Bytes > 256*1024*1024 || !hashPattern.MatchString(app.SHA256) || app.SigningCertificateSHA256 != certificate {
=======
		if seen[app.Package] || app.VersionCode < 1 || app.VersionCode > 2100000000 || !versionPattern.MatchString(app.VersionName) || app.MinSDK < 33 || app.TargetSDK < app.MinSDK || app.TargetSDK > 100 || app.Bytes < 1 || app.Bytes > 256*1024*1024 || !hashPattern.MatchString(app.SHA256) || app.SigningCertificateSHA256 != certificate {
>>>>>>> origin/main
			return ErrInvalid
		}
		if app.URL != origin+"/releases/"+strconv.Itoa(manifest.Sequence)+"/"+name+"-release.apk" {
			return ErrInvalid
		}
		seen[app.Package] = true
	}
	return nil
}
func Sign(manifest Manifest, key ed25519.PrivateKey, now time.Time) ([]byte, error) {
	if len(key) != ed25519.PrivateKeySize || len(manifest.Apps) != 2 {
		return nil, ErrInvalid
	}
	if manifest.Validate(manifest.Origin, manifest.Apps[0].SigningCertificateSHA256, 0, now) != nil {
		return nil, ErrInvalid
	}
	manifest.Apps = append([]Artifact(nil), manifest.Apps...)
	sort.Slice(manifest.Apps, func(i, j int) bool { return manifest.Apps[i].Package < manifest.Apps[j].Package })
	raw, err := json.Marshal(manifest)
	if err != nil || len(raw) > 32*1024 {
		return nil, ErrInvalid
	}
	signature := ed25519.Sign(key, append([]byte(domain), raw...))
	return json.Marshal(envelope{1, base64.RawURLEncoding.EncodeToString(raw), base64.RawURLEncoding.EncodeToString(signature)})
}
func verifiedPayload(raw []byte, key ed25519.PublicKey) (Manifest, error) {
	var wrapper envelope
	if len(raw) == 0 || len(raw) > 64*1024 || len(key) != ed25519.PublicKeySize || strictJSON(raw, &wrapper) != nil || wrapper.Version != 1 {
		return Manifest{}, ErrInvalid
	}
	decode := base64.RawURLEncoding.Strict().DecodeString
	payload, err := decode(wrapper.Payload)
	if err != nil || len(payload) > 32*1024 || base64.RawURLEncoding.EncodeToString(payload) != wrapper.Payload {
		return Manifest{}, ErrInvalid
	}
	signature, err := decode(wrapper.Signature)
	if err != nil || base64.RawURLEncoding.EncodeToString(signature) != wrapper.Signature || !ed25519.Verify(key, append([]byte(domain), payload...), signature) {
		return Manifest{}, ErrInvalid
	}
	return Decode(payload)
}
func Verify(raw []byte, key ed25519.PublicKey, origin, certificate string, minimumSequence int, now time.Time) (Manifest, error) {
	manifest, err := verifiedPayload(raw, key)
	if err != nil || manifest.Validate(origin, certificate, minimumSequence, now) != nil {
		return Manifest{}, ErrInvalid
	}
	return manifest, nil
}
