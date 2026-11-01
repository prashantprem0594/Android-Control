package parentpush

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

var (
	ErrSenderUnavailable = errors.New("parent push sender unavailable")
	ErrNotConfigured     = errors.New("parent push sender is not configured")
)

const (
	maxFCMResponseBytes = 16 * 1024
	maxFCMSendTimeout   = 10 * time.Second
	maxMessageTTL       = 30 * time.Second
)

type SendResult uint8

const (
	SendAccepted SendResult = iota + 1
	SendUnregistered
)

type Sender interface {
	Send(context.Context, []byte, time.Duration) (SendResult, error)
}

type FCMSender struct {
	endpoint string
	access   func(context.Context) (string, error)
	client   *http.Client
	now      func() time.Time
}

func validFCMProject(project string) bool {
	return regexp.MustCompile(`^[a-z0-9][a-z0-9-]{4,62}$`).MatchString(project)
}

func NewFCMSender(project string, access func(context.Context) (string, error), client *http.Client) (*FCMSender, error) {
	if !validFCMProject(project) || access == nil {
		return nil, ErrSenderUnavailable
	}
	return newFCMSenderAt("https://fcm.googleapis.com/v1/projects/"+project+"/messages:send", access, client)
}

func newFCMSenderAt(endpoint string, access func(context.Context) (string, error), client *http.Client) (*FCMSender, error) {
	if access == nil || !strings.HasPrefix(endpoint, "https://") {
		return nil, ErrSenderUnavailable
	}
	bounded := http.Client{Timeout: maxFCMSendTimeout}
	if client != nil {
		bounded = *client
		if bounded.Timeout <= 0 || bounded.Timeout > maxFCMSendTimeout {
			bounded.Timeout = maxFCMSendTimeout
		}
	}
	bounded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &FCMSender{endpoint: endpoint, access: access, client: &bounded, now: time.Now}, nil
}

func parentHintPayload(registration []byte, ttl time.Duration) ([]byte, error) {
	if len(registration) < 1 || len(registration) > 4096 || ttl <= 0 || ttl > maxMessageTTL {
		return nil, ErrSenderUnavailable
	}
	for _, value := range registration {
		if value < 33 || value > 126 {
			return nil, ErrSenderUnavailable
		}
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return nil, ErrSenderUnavailable
	}
	ttlSeconds := int64(ttl / time.Second)
	if ttlSeconds < 1 {
		return nil, ErrSenderUnavailable
	}
	message := struct {
		Message struct {
			Token   string            `json:"token"`
			Data    map[string]string `json:"data"`
			Android struct {
				TTL      string `json:"ttl"`
				Priority string `json:"priority"`
			} `json:"android"`
		} `json:"message"`
	}{}
	message.Message.Token = string(registration)
	message.Message.Data = map[string]string{
		"syncHint": base64.RawURLEncoding.EncodeToString(nonce),
	}
	message.Message.Android.TTL = fmt.Sprintf("%ds", ttlSeconds)
	message.Message.Android.Priority = "HIGH"
	return json.Marshal(message)
}

func (s *FCMSender) Send(ctx context.Context, registration []byte, ttl time.Duration) (SendResult, error) {
	if s == nil || ctx == nil || ctx.Err() != nil || ttl < time.Second || ttl > maxMessageTTL ||
		validateRegistration(string(registration)) != nil {
		return 0, ErrSenderUnavailable
	}
	started := s.now()
	eligibilityDeadline := started.Add(ttl)
	if deadline, ok := ctx.Deadline(); ok && deadline.Before(eligibilityDeadline) {
		eligibilityDeadline = deadline
	}
	attemptDeadline := started.Add(maxFCMSendTimeout)
	if eligibilityDeadline.Before(attemptDeadline) {
		attemptDeadline = eligibilityDeadline
	}
	// OAuth and the FCM request share one finite attempt budget. OAuth latency
	// must neither reset that budget nor extend the call's message eligibility.
	requestContext, cancel := context.WithDeadline(ctx, attemptDeadline)
	defer cancel()
	if requestContext.Err() != nil {
		return 0, ErrSenderUnavailable
	}
	access, err := s.access(requestContext)
	if err != nil || requestContext.Err() != nil || len(access) == 0 || len(access) > 8192 {
		return 0, ErrSenderUnavailable
	}
	for _, value := range access {
		if value < 33 || value > 126 {
			return 0, ErrSenderUnavailable
		}
	}
	remaining := eligibilityDeadline.Sub(s.now())
	body, err := parentHintPayload(registration, remaining)
	if err != nil || requestContext.Err() != nil {
		return 0, ErrSenderUnavailable
	}
	request, err := http.NewRequestWithContext(requestContext, http.MethodPost, s.endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, ErrSenderUnavailable
	}
	request.Header.Set("Authorization", "Bearer "+access)
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return 0, ErrSenderUnavailable
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxFCMResponseBytes+1))
	if err != nil || len(raw) > maxFCMResponseBytes {
		return 0, ErrSenderUnavailable
	}
	if response.StatusCode == http.StatusOK {
		var accepted struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(raw, &accepted) != nil || accepted.Name == "" || len(accepted.Name) > 4096 {
			return 0, ErrSenderUnavailable
		}
		return SendAccepted, nil
	}
	// Only the provider's documented token-not-found response can invalidate a
	// registration. A transient/auth/proxy failure with an inconsistent body
	// must retain the routing token for retry or operator recovery.
	if response.StatusCode == http.StatusNotFound && explicitlyUnregistered(raw) {
		return SendUnregistered, nil
	}
	return 0, ErrSenderUnavailable
}

func explicitlyUnregistered(raw []byte) bool {
	var response struct {
		Error struct {
			Details []struct {
				Type      string `json:"@type"`
				ErrorCode string `json:"errorCode"`
			} `json:"details"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &response) != nil {
		return false
	}
	for _, detail := range response.Error.Details {
		if detail.Type == "type.googleapis.com/google.firebase.fcm.v1.FcmError" &&
			detail.ErrorCode == "UNREGISTERED" {
			return true
		}
	}
	return false
}
