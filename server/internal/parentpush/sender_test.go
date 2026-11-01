package parentpush

import (
	"appcontrol/server/internal/wakeup"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	fixtureFCMToken = "private-parent-routing-token"
	fixtureOAuth    = "private-parent-oauth-access"
)

type remapOAuthTransport struct {
	next   http.RoundTripper
	target *url.URL
}

func (t remapOAuthTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Host == "oauth2.googleapis.com" && request.URL.Path == "/token" {
		clone := request.Clone(request.Context())
		copyURL := *t.target
		clone.URL = &copyURL
		clone.Host = copyURL.Host
		return t.next.RoundTrip(clone)
	}
	return t.next.RoundTrip(request)
}

func fixtureServiceAccount(t *testing.T, project string) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return mustJSON(t, map[string]string{
		"type": "service_account", "project_id": project,
		"client_email":   "fixture@" + project + ".iam.gserviceaccount.com",
		"private_key_id": "fixture-key",
		"private_key":    string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		"token_uri":      "https://oauth2.googleapis.com/token",
	})
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func fixtureFCMSender(t *testing.T, fcmURL string, client *http.Client) *FCMSender {
	t.Helper()
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		form, err := url.ParseQuery(string(body))
		if err != nil || form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" ||
			form.Get("assertion") == "" {
			t.Fatal("invalid fixture OAuth exchange")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"`+fixtureOAuth+`","token_type":"Bearer","expires_in":3600}`)
	}))
	t.Cleanup(oauth.Close)
	oauthURL, err := url.Parse(oauth.URL)
	if err != nil {
		t.Fatal(err)
	}
	transport := remapOAuthTransport{next: http.DefaultTransport, target: oauthURL}
	account, err := wakeup.NewServiceAccount(fixtureServiceAccount(t, "fixture-project"), &http.Client{Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	sender, err := newFCMSenderAt(fcmURL, account.AccessToken, client)
	if err != nil {
		t.Fatal(err)
	}
	return sender
}

func TestParentFCMSenderUsesOnlyFreshOpaqueHintAndBoundedTTL(t *testing.T) {
	var previous string
	requests := 0
	fcm := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer "+fixtureOAuth {
			t.Fatal("incorrect FCM request authentication")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "private-device-id") || strings.Contains(string(body), "caller-name") ||
			strings.Contains(string(body), "caller-number") || strings.Contains(string(body), fixtureOAuth) {
			t.Fatal("private content was included in FCM payload")
		}
		var root struct {
			Message struct {
				Token   string            `json:"token"`
				Data    map[string]string `json:"data"`
				Android struct {
					TTL      string `json:"ttl"`
					Priority string `json:"priority"`
				} `json:"android"`
				Notification json.RawMessage `json:"notification"`
			} `json:"message"`
		}
		if err := json.Unmarshal(body, &root); err != nil {
			t.Fatal(err)
		}
		ttl, err := time.ParseDuration(root.Message.Android.TTL)
		if err != nil || ttl < time.Second || ttl > 17*time.Second+900*time.Millisecond ||
			root.Message.Token != fixtureFCMToken || root.Message.Android.Priority != "HIGH" ||
			len(root.Message.Data) != 1 || root.Message.Notification != nil {
			t.Fatal("FCM payload was not token-only, opaque data-only, high priority and TTL-bounded")
		}
		var fields map[string]map[string]json.RawMessage
		if err := json.Unmarshal(body, &fields); err != nil || len(fields) != 1 || len(fields["message"]) != 3 {
			t.Fatal("unexpected FCM top-level or message fields")
		}
		for _, name := range []string{"token", "data", "android"} {
			if fields["message"][name] == nil {
				t.Fatal("FCM message field missing or replaced")
			}
		}
		var android map[string]json.RawMessage
		if err := json.Unmarshal(fields["message"]["android"], &android); err != nil || len(android) != 2 || android["ttl"] == nil || android["priority"] == nil {
			t.Fatal("unexpected Android message metadata")
		}
		hint := root.Message.Data["syncHint"]
		decoded, err := base64.RawURLEncoding.Strict().DecodeString(hint)
		if err != nil || len(decoded) != 16 || hint == previous {
			t.Fatal("syncHint is not a fresh opaque nonce")
		}
		previous = hint
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"name":"projects/fixture/messages/accepted"}`)
	}))
	defer fcm.Close()
	sender := fixtureFCMSender(t, fcm.URL, fcm.Client())
	for i := 0; i < 2; i++ {
		result, err := sender.Send(context.Background(), []byte(fixtureFCMToken), 17*time.Second+900*time.Millisecond)
		if err != nil || result != SendAccepted {
			t.Fatalf("send not accepted: %v %v", result, err)
		}
	}
	if requests != 2 {
		t.Fatal("expected two independently randomized sends")
	}
	if _, err := parentHintPayload([]byte(fixtureFCMToken), 31*time.Second); err == nil {
		t.Fatal("TTL beyond call eligibility accepted")
	}
}

func TestParentFCMClassifiesOnlyExplicitUnregisteredAndRetriesOtherErrors(t *testing.T) {
	var mode atomic.Int32
	fcm := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		switch mode.Load() {
		case 0:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"status":"INVALID_ARGUMENT"}}`)
		case 1:
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":{"status":"UNAUTHENTICATED"}}`)
		case 2:
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"error":{"details":[{"@type":"type.googleapis.com/google.firebase.fcm.v1.FcmError","errorCode":"SENDER_ID_MISMATCH"}]}}`)
		case 3:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":{"details":[{"@type":"type.googleapis.com/google.firebase.fcm.v1.FcmError","errorCode":"UNREGISTERED"}]}}`)
		case 4:
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, "private-response-body")
		case 5:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, "private-response-body")
		case 6:
			_, _ = io.WriteString(w, "private-malformed-response")
		case 7, 8, 9, 10:
			status := map[int32]int{7: http.StatusTooManyRequests, 8: http.StatusServiceUnavailable,
				9: http.StatusUnauthorized, 10: http.StatusTemporaryRedirect}[mode.Load()]
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"error":{"details":[{"@type":"type.googleapis.com/google.firebase.fcm.v1.FcmError","errorCode":"UNREGISTERED"}]}}`)
		case 11:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":{"status":"NOT_FOUND"}}`)
		}
	}))
	defer fcm.Close()
	sender := fixtureFCMSender(t, fcm.URL, fcm.Client())
	for _, test := range []struct {
		mode             int32
		wantUnregistered bool
	}{
		{0, false}, {1, false}, {2, false}, {3, true}, {4, false}, {5, false}, {6, false},
		{7, false}, {8, false}, {9, false}, {10, false}, {11, false},
	} {
		mode.Store(test.mode)
		result, err := sender.Send(context.Background(), []byte(fixtureFCMToken), 5*time.Second)
		if test.wantUnregistered {
			if err != nil || result != SendUnregistered {
				t.Fatalf("explicit UNREGISTERED not identified: %v %v", result, err)
			}
		} else if err == nil || result == SendUnregistered ||
			strings.Contains(err.Error(), "private-") {
			t.Fatalf("non-token provider error was misclassified or leaked: %v %v", result, err)
		}
	}
}

func TestParentFCMSenderHonorsNetworkDeadlineAndCancellation(t *testing.T) {
	fcm := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_, _ = io.WriteString(w, `{"name":"late"}`)
	}))
	defer fcm.Close()
	sender, err := newFCMSenderAt(fcm.URL, func(context.Context) (string, error) {
		return fixtureOAuth, nil
	}, fcm.Client())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := sender.Send(ctx, []byte(fixtureFCMToken), 5*time.Second); err == nil {
		t.Fatal("stalled FCM request was accepted")
	}
	if time.Since(start) > time.Second {
		t.Fatal("FCM request exceeded its caller deadline")
	}
}

func TestParentFCMSenderRecomputesTTLAfterOAuthWithinOriginalEligibility(t *testing.T) {
	for _, callerBound := range []bool{false, true} {
		t.Run(fmt.Sprintf("caller-bound-%t", callerBound), func(t *testing.T) {
			ttls := make(chan time.Duration, 1)
			fcm := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var payload struct {
					Message struct {
						Android struct {
							TTL      string `json:"ttl"`
							Priority string `json:"priority"`
						} `json:"android"`
					} `json:"message"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error("invalid test payload")
					return
				}
				ttl, err := time.ParseDuration(payload.Message.Android.TTL)
				if err != nil || payload.Message.Android.Priority != "HIGH" {
					t.Error("invalid Android TTL or priority")
					return
				}
				ttls <- ttl
				_, _ = io.WriteString(w, `{"name":"projects/fixture/messages/accepted"}`)
			}))
			defer fcm.Close()
			started := time.Now()
			now := started
			ctx := context.Background()
			cancel := func() {}
			eligibility := started.Add(8 * time.Second)
			if callerBound {
				eligibility = started.Add(5 * time.Second)
				ctx, cancel = context.WithDeadline(ctx, eligibility)
			}
			defer cancel()
			var oauthDeadline time.Time
			sender, err := newFCMSenderAt(fcm.URL, func(attempt context.Context) (string, error) {
				var ok bool
				oauthDeadline, ok = attempt.Deadline()
				if !ok {
					t.Fatal("OAuth attempt lacks finite deadline")
				}
				now = now.Add(3 * time.Second)
				return fixtureOAuth, nil
			}, fcm.Client())
			if err != nil {
				t.Fatal(err)
			}
			sender.now = func() time.Time { return now }
			result, err := sender.Send(ctx, []byte(fixtureFCMToken), 8*time.Second)
			if err != nil || result != SendAccepted {
				t.Fatal("eligible delayed-OAuth send failed")
			}
			if oauthDeadline.After(eligibility) || oauthDeadline.After(started.Add(maxFCMSendTimeout)) {
				t.Fatal("OAuth budget exceeds original eligibility or network cap")
			}
			select {
			case ttl := <-ttls:
				remaining := eligibility.Sub(now)
				if ttl < time.Second || ttl > remaining || ttl >= 8*time.Second {
					t.Fatal("OAuth delay was not deducted from message TTL")
				}
			default:
				t.Fatal("FCM request was not observed")
			}
		})
	}
}

func TestParentFCMSenderExpiredOrCancelledOAuthNeverCallsFCM(t *testing.T) {
	for _, mode := range []string{"expired", "below-one-second", "cancelled", "already-cancelled", "oauth-error"} {
		t.Run(mode, func(t *testing.T) {
			var requests atomic.Int32
			fcm := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				_, _ = io.WriteString(w, `{"name":"unexpected"}`)
			}))
			defer fcm.Close()
			started := time.Now()
			now := started
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "already-cancelled" {
				cancel()
			}
			accessCalls := 0
			sender, err := newFCMSenderAt(fcm.URL, func(attempt context.Context) (string, error) {
				accessCalls++
				if _, ok := attempt.Deadline(); !ok {
					t.Fatal("OAuth attempt lacks finite deadline")
				}
				switch mode {
				case "expired":
					now = now.Add(6 * time.Second)
				case "below-one-second":
					now = now.Add(4500 * time.Millisecond)
				case "cancelled":
					cancel()
				case "oauth-error":
					return "", errors.New("private-oauth-response")
				}
				return fixtureOAuth, nil
			}, fcm.Client())
			if err != nil {
				t.Fatal(err)
			}
			sender.now = func() time.Time { return now }
			if _, err := sender.Send(ctx, []byte(fixtureFCMToken), 5*time.Second); err == nil || strings.Contains(err.Error(), "private-") {
				t.Fatal("ineligible OAuth result accepted or leaked")
			}
			if requests.Load() != 0 {
				t.Fatal("FCM called after eligibility or cancellation")
			}
			if mode == "already-cancelled" && accessCalls != 0 {
				t.Fatal("OAuth called for an already-cancelled send")
			}
		})
	}
}
