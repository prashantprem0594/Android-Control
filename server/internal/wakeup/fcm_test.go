package wakeup

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestFCMSendsOnlyOpaqueData(t *testing.T) {
	const registration = "routing-registration-value"
	const credential = "private-oauth-value"
	var previous string
	calls := 0
	original := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "POST" || r.URL.String() != "https://fcm.googleapis.com/v1/projects/test-project/messages:send" || r.Header.Get("Authorization") != "Bearer "+credential {
			t.Fatal("incorrect FCM endpoint or authentication")
		}
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) > 10*time.Second {
			t.Fatal("missing bounded context")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), credential) || strings.Contains(string(body), "private-device-id") {
			t.Fatal("sensitive context in payload")
		}
		var root map[string]json.RawMessage
		if err = json.Unmarshal(body, &root); err != nil || len(root) != 1 {
			t.Fatal("unexpected payload fields")
		}
		var message map[string]json.RawMessage
		if err = json.Unmarshal(root["message"], &message); err != nil || len(message) != 2 {
			t.Fatal("unexpected message fields")
		}
		var target string
		if err = json.Unmarshal(message["token"], &target); err != nil || target != registration {
			t.Fatal("invalid routing metadata")
		}
		var data map[string]string
		if err = json.Unmarshal(message["data"], &data); err != nil || len(data) != 1 {
			t.Fatal("data contains more than an opaque hint")
		}
		raw, err := base64.RawURLEncoding.Strict().DecodeString(data["syncHint"])
		if err != nil || len(raw) != 16 || data["syncHint"] == previous {
			t.Fatal("hint is not a fresh opaque nonce")
		}
		previous = data["syncHint"]
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"name":"accepted"}`)), Header: make(http.Header)}, nil
	})}
	f, err := NewFCM("test-project", func(context.Context) (string, error) { return credential, nil }, func(_ context.Context, id string) (string, error) {
		if id != "private-device-id" {
			t.Fatal("wrong target")
		}
		return registration, nil
	}, original)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = f.Wake(context.Background(), "private-device-id"); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 2 || original.Timeout != 0 || original.CheckRedirect != nil {
		t.Fatal("unexpected request count or mutated caller client")
	}
}

func TestFCMRedirectAndErrorsDoNotLeakSecrets(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://untrusted.example/"}}, Body: io.NopCloser(strings.NewReader("private-response")), Request: r}, nil
	})}
	f, err := NewFCM("test-project", func(context.Context) (string, error) { return "private-access", nil }, func(context.Context, string) (string, error) { return "private-registration", nil }, client)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Wake(context.Background(), "device"); err == nil || strings.Contains(err.Error(), "private") {
		t.Fatal("redirect accepted or secrets exposed")
	}
	if calls != 1 {
		t.Fatal("FCM redirect followed")
	}
	f.access = func(context.Context) (string, error) { return "", errors.New("private-source-diagnostic") }
	if err = f.Wake(context.Background(), "device"); !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "private") {
		t.Fatal("source diagnostic leaked")
	}
	for _, project := range []string{"../project", "project/path", "project?query"} {
		if _, err = NewFCM(project, f.access, f.registration, nil); err == nil {
			t.Fatal("invalid project accepted")
		}
	}
	for _, registration := range []string{"", strings.Repeat("a", 4097), "header\ninjection"} {
		if _, err = hint(registration); err == nil {
			t.Fatal("invalid routing token accepted")
		}
	}
}
