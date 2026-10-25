package wakeup

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"sync"
	"time"
)

const googleTokenEndpoint = "https://oauth2.googleapis.com/token"
const messagingScope = "https://www.googleapis.com/auth/firebase.messaging"

// ServiceAccount exchanges a signed JWT for a short-lived messaging-only OAuth
// token. It neither logs credentials nor accepts an endpoint from the caller.
type ServiceAccount struct {
	project string
	email   string
	keyID   string
	key     *rsa.PrivateKey
	client  *http.Client
	now     func() time.Time
	mu      sync.Mutex
	token   string
	expires time.Time
}

func NewServiceAccount(raw []byte, client *http.Client) (*ServiceAccount, error) {
	if len(raw) == 0 || len(raw) > 64*1024 {
		return nil, ErrUnavailable
	}
	var input struct {
		Type       string `json:"type"`
		Project    string `json:"project_id"`
		Email      string `json:"client_email"`
		KeyID      string `json:"private_key_id"`
		PrivateKey string `json:"private_key"`
		TokenURI   string `json:"token_uri"`
	}
	if json.Unmarshal(raw, &input) != nil || input.Type != "service_account" || input.TokenURI != googleTokenEndpoint {
		return nil, ErrUnavailable
	}
	if !validProject(input.Project) || len(input.Email) > 254 || len(input.KeyID) > 128 {
		return nil, ErrUnavailable
	}
	address, err := mail.ParseAddress(input.Email)
	if err != nil || address.Address != input.Email || !strings.HasSuffix(input.Email, ".gserviceaccount.com") || strings.ContainsAny(input.Email, "\r\n") {
		return nil, ErrUnavailable
	}
	block, rest := pem.Decode([]byte(input.PrivateKey))
	if !bytes.HasPrefix(bytes.TrimSpace([]byte(input.PrivateKey)), []byte("-----BEGIN PRIVATE KEY-----")) || block == nil || block.Type != "PRIVATE KEY" || len(block.Headers) != 0 || len(bytes.TrimSpace(rest)) != 0 {
		return nil, ErrUnavailable
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, ErrUnavailable
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok || key.N.BitLen() < 2048 || key.N.BitLen() > 8192 || key.Validate() != nil {
		return nil, ErrUnavailable
	}
	bounded := http.Client{Timeout: 10 * time.Second}
	if client != nil {
		bounded = *client
	}
	if bounded.Timeout <= 0 || bounded.Timeout > 10*time.Second {
		bounded.Timeout = 10 * time.Second
	}
	bounded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &ServiceAccount{project: input.Project, email: input.Email, keyID: input.KeyID, key: key, client: &bounded, now: time.Now}, nil
}

func (s *ServiceAccount) Project() string { return s.project }

func (s *ServiceAccount) AccessToken(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return "", ErrUnavailable
	}
	now := s.now().UTC()
	if s.token != "" && now.Add(time.Minute).Before(s.expires) {
		return s.token, nil
	}
	header, _ := json.Marshal(struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
		KeyID     string `json:"kid,omitempty"`
	}{"RS256", "JWT", s.keyID})
	claims, _ := json.Marshal(struct {
		Issuer   string `json:"iss"`
		Scope    string `json:"scope"`
		Audience string `json:"aud"`
		Issued   int64  `json:"iat"`
		Expires  int64  `json:"exp"`
	}{s.email, messagingScope, googleTokenEndpoint, now.Unix(), now.Add(time.Hour).Unix()})
	encode := base64.RawURLEncoding.EncodeToString
	unsigned := encode(header) + "." + encode(claims)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", ErrUnavailable
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {unsigned + "." + encode(signature)}}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, "POST", googleTokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", ErrUnavailable
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := s.client.Do(request)
	if err != nil {
		return "", ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 16*1024+1))
	if err != nil || len(raw) > 16*1024 {
		return "", ErrUnavailable
	}
	var token struct {
		Access  string `json:"access_token"`
		Type    string `json:"token_type"`
		Expires int    `json:"expires_in"`
	}
	if json.Unmarshal(raw, &token) != nil || !strings.EqualFold(token.Type, "Bearer") || len(token.Access) == 0 || len(token.Access) > 8192 || token.Expires <= 60 || token.Expires > 3600 {
		return "", ErrUnavailable
	}
	for _, character := range token.Access {
		if character < 33 || character > 126 {
			return "", ErrUnavailable
		}
	}
	s.token = token.Access
	s.expires = now.Add(time.Duration(token.Expires) * time.Second)
	return s.token, nil
}
