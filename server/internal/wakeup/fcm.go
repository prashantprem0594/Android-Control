// Package wakeup carries opaque synchronization hints, never command content.
package wakeup

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var ErrUnavailable = errors.New("wake-up provider unavailable")

type SecretSource func(context.Context, string) (string, error)
type AccessTokenSource func(context.Context) (string, error)

func validProject(project string) bool {
	return regexp.MustCompile(`^[a-z0-9][a-z0-9-]{4,62}$`).MatchString(project)
}

type FCM struct {
	endpoint     string
	access       AccessTokenSource
	registration SecretSource
	client       *http.Client
}

// access supplies a short-lived OAuth token; registration resolves the routing
// token for a device. ServiceAccount and the encrypted control registry provide
// these sources in the optional server integration.
func NewFCM(project string, access AccessTokenSource, registration SecretSource, client *http.Client) (*FCM, error) {
	if !validProject(project) || access == nil || registration == nil {
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
	return &FCM{"https://fcm.googleapis.com/v1/projects/" + project + "/messages:send", access, registration, &bounded}, nil
}

// The only application data is a fresh random nonce. The registration token is
// required FCM routing metadata and is never included in the data object.
func hint(registration string) ([]byte, error) {
	if len(registration) == 0 || len(registration) > 4096 || strings.ContainsAny(registration, "\r\n") {
		return nil, ErrUnavailable
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return nil, ErrUnavailable
	}
	message := struct {
		Message struct {
			Token string `json:"token"`
			Data  struct {
				SyncHint string `json:"syncHint"`
			} `json:"data"`
		} `json:"message"`
	}{}
	message.Message.Token = registration
	message.Message.Data.SyncHint = base64.RawURLEncoding.EncodeToString(nonce)
	return json.Marshal(message)
}

func (f *FCM) Wake(ctx context.Context, deviceID string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	registration, err := f.registration(ctx, deviceID)
	if err != nil {
		return ErrUnavailable
	}
	body, err := hint(registration)
	if err != nil {
		return err
	}
	access, err := f.access(ctx)
	if err != nil || access == "" || strings.ContainsAny(access, "\r\n") {
		return ErrUnavailable
	}
	request, err := http.NewRequestWithContext(ctx, "POST", f.endpoint, bytes.NewReader(body))
	if err != nil {
		return ErrUnavailable
	}
	request.Header.Set("Authorization", "Bearer "+access)
	request.Header.Set("Content-Type", "application/json")
	response, err := f.client.Do(request)
	if err != nil {
		return ErrUnavailable
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("FCM send status %d", response.StatusCode)
	}
	return nil
}
