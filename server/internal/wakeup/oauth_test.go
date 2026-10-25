package wakeup

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func accountFixture(t *testing.T) ([]byte, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal("fixture key generation failed")
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal("fixture key encoding failed")
	}
	raw, err := json.Marshal(map[string]string{"type": "service_account", "project_id": "fixture-project", "client_email": "fixture@fixture-project.iam.gserviceaccount.com", "private_key_id": "fixture-key-id", "private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), "token_uri": googleTokenEndpoint})
	if err != nil {
		t.Fatal("fixture credential encoding failed")
	}
	return raw, key
}

func TestOAuthJWTSignatureScopeCachingAndConcurrentRefresh(t *testing.T) {
	raw, key := accountFixture(t)
	now := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	calls := 0
	client := &http.Client{Transport: transportFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.Method != "POST" || request.URL.String() != googleTokenEndpoint || request.Header.Get("Content-Type") != "application/x-www-form-urlencoded" || request.Header.Get("Authorization") != "" {
			t.Fatal("OAuth endpoint or request headers differ")
		}
		deadline, ok := request.Context().Deadline()
		if !ok || time.Until(deadline) > 10*time.Second {
			t.Fatal("OAuth context unbounded")
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal("OAuth body read failed")
		}
		form, err := url.ParseQuery(string(body))
		if err != nil || len(form) != 2 || form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
			t.Fatal("OAuth grant shape differs")
		}
		parts := strings.Split(form.Get("assertion"), ".")
		if len(parts) != 3 {
			t.Fatal("JWT shape differs")
		}
		decode := base64.RawURLEncoding.Strict().DecodeString
		headerRaw, err := decode(parts[0])
		if err != nil {
			t.Fatal("JWT header invalid")
		}
		var header map[string]any
		if json.Unmarshal(headerRaw, &header) != nil || len(header) != 3 || header["alg"] != "RS256" || header["typ"] != "JWT" || header["kid"] != "fixture-key-id" {
			t.Fatal("JWT header differs")
		}
		claimRaw, err := decode(parts[1])
		if err != nil {
			t.Fatal("JWT claims invalid")
		}
		var claims map[string]any
		if json.Unmarshal(claimRaw, &claims) != nil || len(claims) != 5 || claims["iss"] != "fixture@fixture-project.iam.gserviceaccount.com" || claims["scope"] != messagingScope || claims["aud"] != googleTokenEndpoint || claims["iat"] != float64(now.Unix()) || claims["exp"] != float64(now.Add(time.Hour).Unix()) {
			t.Fatal("JWT claims differ")
		}
		signature, err := decode(parts[2])
		if err != nil {
			t.Fatal("JWT signature invalid")
		}
		digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
		if rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], signature) != nil {
			t.Fatal("JWT signature did not verify")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"access_token":"fixture-oauth-access","token_type":"Bearer","expires_in":3600}`))}, nil
	})}
	source, err := NewServiceAccount(raw, client)
	if err != nil {
		t.Fatal("OAuth source rejected fixture")
	}
	source.now = func() time.Time { return now }
	if source.Project() != "fixture-project" {
		t.Fatal("project lost")
	}
	var workers sync.WaitGroup
	for index := 0; index < 8; index++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			token, err := source.AccessToken(context.Background())
			if err != nil || token != "fixture-oauth-access" {
				t.Error("cached token unavailable")
			}
		}()
	}
	workers.Wait()
	if calls != 1 {
		t.Fatal("concurrent requests duplicated OAuth exchange")
	}
	now = now.Add(3541 * time.Second)
	if _, err := source.AccessToken(context.Background()); err != nil || calls != 2 {
		t.Fatal("near-expiry OAuth token not refreshed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := source.AccessToken(ctx); !errors.Is(err, ErrUnavailable) || calls != 2 {
		t.Fatal("cancelled OAuth request returned credentials")
	}
}

func TestOAuthRejectsCredentialEndpointOverrideAndMalformedKeys(t *testing.T) {
	raw, _ := accountFixture(t)
	var input map[string]string
	if json.Unmarshal(raw, &input) != nil {
		t.Fatal("fixture decode failed")
	}
	for _, field := range []string{"type", "project_id", "client_email", "token_uri", "private_key"} {
		original := input[field]
		input[field] = "invalid-sensitive-fixture-value"
		if field == "project_id" {
			input[field] = "invalid_project"
		}
		bad, _ := json.Marshal(input)
		if _, err := NewServiceAccount(bad, nil); !errors.Is(err, ErrUnavailable) {
			t.Fatal("invalid credential field accepted")
		}
		input[field] = original
	}
	input["private_key"] = "prefix-junk\n" + input["private_key"]
	bad, _ := json.Marshal(input)
	if _, err := NewServiceAccount(bad, nil); !errors.Is(err, ErrUnavailable) {
		t.Fatal("private key prefix junk accepted")
	}
	if _, err := NewServiceAccount(make([]byte, 64*1024+1), nil); !errors.Is(err, ErrUnavailable) {
		t.Fatal("oversized credential accepted")
	}
}

func TestOAuthRedirectMalformedResponsesAndErrorPrivacy(t *testing.T) {
	raw, _ := accountFixture(t)
	for _, body := range []string{`{"access_token":"sensitive-response","token_type":"Bearer","expires_in":0}`, `{"access_token":"sensitive-response","token_type":"Other","expires_in":3600}`, `{"access_token":"bad\nvalue","token_type":"Bearer","expires_in":3600}`, strings.Repeat("sensitive-response", 2000)} {
		source, err := NewServiceAccount(raw, &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
		})})
		if err != nil {
			t.Fatal("fixture source failed")
		}
		if _, err := source.AccessToken(context.Background()); err != ErrUnavailable || strings.Contains(err.Error(), "sensitive-response") {
			t.Fatal("OAuth response accepted or error exposed private response")
		}
	}
	calls := 0
	source, err := NewServiceAccount(raw, &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 307, Header: http.Header{"Location": []string{"https://other.example.org/token"}}, Body: io.NopCloser(strings.NewReader("sensitive-response"))}, nil
	})})
	if err != nil {
		t.Fatal("fixture source failed")
	}
	if _, err := source.AccessToken(context.Background()); err != ErrUnavailable || calls != 1 {
		t.Fatal("OAuth followed credential redirect")
	}
}
