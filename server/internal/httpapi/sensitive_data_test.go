package httpapi

import (
	"appcontrol/server/internal/audit"
	"appcontrol/server/internal/auth"
	"appcontrol/server/internal/control"
	"appcontrol/server/internal/pairing"
	"appcontrol/server/internal/storage"
	"appcontrol/server/internal/wakeup"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

type sensitiveDataRoundTripper func(*http.Request) (*http.Response, error)

func (f sensitiveDataRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestSensitiveDataStaysOutOfLogsAuditAndWakeupData(t *testing.T) {
	ctx := context.Background()
	const (
		emailMarker               = "SYNTHETIC_PARENT_EMAIL_MARKER@example.test"
		passwordMarker            = "SYNTHETIC_PARENT_PASSWORD_MARKER"
		wrongPasswordMarker       = "SYNTHETIC_FAILED_PASSWORD_MARKER"
		deviceNameMarker          = "SYNTHETIC_DEVICE_NAME_MARKER"
		deviceKeyMarker           = "SYNTHETIC_DEVICE_KEY_MARKER"
		enrollmentSignatureMarker = "SYNTHETIC_ENROLLMENT_SIGNATURE_MARKER"
		authSignatureMarker       = "SYNTHETIC_AUTH_SIGNATURE_MARKER"
		contactNameMarker         = "SYNTHETIC_CONTACT_NAME_MARKER"
		phoneNumberMarker         = "+15550001111"
		policyMarker              = "com.synthetic.privacy_policy_marker"
		callDetailsMarker         = "SYNTHETIC_CALL_DETAILS_MARKER"
		routingTokenMarker        = "SYNTHETIC_FCM_ROUTING_TOKEN_MARKER"
		oauthTokenMarker          = "SYNTHETIC_FCM_OAUTH_TOKEN_MARKER"
		serverErrorMarker         = "SYNTHETIC_SERVER_ERROR_MARKER"
		databasePathMarker        = "SYNTHETIC_DATABASE_PATH_MARKER"
		invalidRefreshMarker      = "SYNTHETIC_INVALID_REFRESH_TOKEN_MARKER"
		callID                    = "a38d3172-4194-4c6d-b468-6c1c4e75ab93"
	)

	databasePath := filepath.Join(t.TempDir(), databasePathMarker+".db")
	db, err := storage.Open(ctx, databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	authentication, err := auth.NewService(db, auth.Options{
		PasswordParameters: auth.PasswordParameters{
			MemoryKiB: 8192, Iterations: 1, Parallelism: 1, SaltBytes: 16, KeyBytes: 32,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	pairingService, err := pairing.NewService(db, pairing.Options{})
	if err != nil {
		t.Fatal(err)
	}
	wakeupTokens, err := control.NewWakeupTokens(db, bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}

	var fcmBodies [][]byte
	fcmClient := &http.Client{Transport: sensitiveDataRoundTripper(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://fcm.googleapis.com/v1/projects/fixture-project/messages:send" ||
			request.Header.Get("Authorization") != "Bearer "+oauthTokenMarker {
			t.Fatal("unexpected fixture FCM request")
		}
		body, readErr := io.ReadAll(request.Body)
		if readErr != nil {
			t.Fatal(readErr)
		}
		fcmBodies = append(fcmBodies, body)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"name":"fixture-message"}`)),
			Request:    request,
		}, nil
	})}
	fcm, err := wakeup.NewFCM("fixture-project",
		func(context.Context) (string, error) { return oauthTokenMarker, nil },
		func(ctx context.Context, deviceID string) (string, error) {
			return wakeupTokens.Registration(ctx, deviceID)
		},
		fcmClient)
	if err != nil {
		t.Fatal(err)
	}
	controlService := control.NewServiceWithWakeup(db, fcm)
	controlService.SetRemoteCallsEnabled(true)

	var logs bytes.Buffer
	var auditFailure error
	handler := NewHandler(Options{
		Auth:         authentication,
		Pairing:      pairingService,
		Control:      controlService,
		WakeupTokens: wakeupTokens,
		Logger:       slog.New(slog.NewJSONHandler(&logs, nil)),
		Ready: func(context.Context) error {
			return errors.New(serverErrorMarker)
		},
		Audit: func(ctx context.Context, parentID string, after int64, limit int) ([]audit.Event, error) {
			if auditFailure != nil {
				return nil, auditFailure
			}
			return audit.List(ctx, db, parentID, after, limit)
		},
	})
	call := func(method, path, bearer, body string, wantStatus int) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		if bearer != "" {
			request.Header.Set("Authorization", "Bearer "+bearer)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != wantStatus {
			t.Fatalf("%s %s: got status %d, want %d; body=%s", method, path, response.Code, wantStatus, response.Body.String())
		}
		return response
	}
	assertError := func(response *httptest.ResponseRecorder, code string) {
		t.Helper()
		expected, _ := json.Marshal(errorResponse{Error: code})
		if strings.TrimSpace(response.Body.String()) != string(expected) {
			t.Fatalf("unexpected fixed error response: %s", response.Body.String())
		}
	}

	body, err := json.Marshal(credentialsRequest{Email: emailMarker, Password: passwordMarker})
	if err != nil {
		t.Fatal(err)
	}
	call(http.MethodPost, "/v1/auth/register", "", string(body), http.StatusCreated)
	body, err = json.Marshal(credentialsRequest{Email: emailMarker, Password: wrongPasswordMarker})
	if err != nil {
		t.Fatal(err)
	}
	failedLogin := call(http.MethodPost, "/v1/auth/login", "", string(body), http.StatusUnauthorized)
	assertError(failedLogin, "invalid_credentials")
	body, err = json.Marshal(credentialsRequest{Email: emailMarker, Password: passwordMarker})
	if err != nil {
		t.Fatal(err)
	}
	login := call(http.MethodPost, "/v1/auth/login", "", string(body), http.StatusOK)
	var parentSession sessionResponse
	if err := json.Unmarshal(login.Body.Bytes(), &parentSession); err != nil {
		t.Fatal(err)
	}
	parentID := parentSession.Parent.ID
	parentAccess := parentSession.Tokens.AccessToken
	parentRefresh := parentSession.Tokens.RefreshToken
	initialParentAccess := parentAccess
	initialParentRefresh := parentRefresh

	badRefreshBody, err := json.Marshal(refreshRequest{RefreshToken: invalidRefreshMarker})
	if err != nil {
		t.Fatal(err)
	}
	badRefresh := call(http.MethodPost, "/v1/auth/refresh", "", string(badRefreshBody), http.StatusUnauthorized)
	assertError(badRefresh, "invalid_token")
	refreshBody, err := json.Marshal(refreshRequest{RefreshToken: parentRefresh})
	if err != nil {
		t.Fatal(err)
	}
	refreshed := call(http.MethodPost, "/v1/auth/refresh", "", string(refreshBody), http.StatusOK)
	var rotated auth.TokenPair
	if err := json.Unmarshal(refreshed.Body.Bytes(), &rotated); err != nil {
		t.Fatal(err)
	}
	parentAccess = rotated.AccessToken
	parentRefresh = rotated.RefreshToken

	codeResponse := call(http.MethodPost, "/v1/pairing/codes", parentAccess, `{}`, http.StatusCreated)
	var code pairing.PairingCode
	if err := json.Unmarshal(codeResponse.Body.Bytes(), &code); err != nil {
		t.Fatal(err)
	}
	deviceKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	attackerKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicKeyDER, err := x509.MarshalPKIXPublicKey(&deviceKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	encode := base64.RawURLEncoding.EncodeToString
	badEnrollmentBody, err := json.Marshal(map[string]string{
		"code": code.Code, "displayName": deviceNameMarker, "publicKey": deviceKeyMarker,
		"keyAlgorithm": pairing.KeyAlgorithmECDSAP256SHA256,
	})
	if err != nil {
		t.Fatal(err)
	}
	badEnrollment := call(http.MethodPost, "/v1/enrollments", "", string(badEnrollmentBody), http.StatusBadRequest)
	assertError(badEnrollment, "invalid_enrollment")
	enrollmentBody, err := json.Marshal(map[string]string{
		"code": code.Code, "displayName": deviceNameMarker, "publicKey": encode(publicKeyDER),
		"keyAlgorithm": pairing.KeyAlgorithmECDSAP256SHA256,
	})
	if err != nil {
		t.Fatal(err)
	}
	enrollmentResponse := call(http.MethodPost, "/v1/enrollments", "", string(enrollmentBody), http.StatusCreated)
	var enrollment pairing.Enrollment
	if err := json.Unmarshal(enrollmentResponse.Body.Bytes(), &enrollment); err != nil {
		t.Fatal(err)
	}
	challenge, err := base64.RawURLEncoding.DecodeString(enrollment.Challenge)
	if err != nil {
		t.Fatal(err)
	}
	enrollmentDigest := sha256.Sum256(append([]byte("app-control-device-enrollment-v1\n"+enrollment.ID+"\n"), challenge...))
	wrongEnrollmentSignature, err := ecdsa.SignASN1(rand.Reader, attackerKey, enrollmentDigest[:])
	if err != nil {
		t.Fatal(err)
	}
	badProofBody, err := json.Marshal(map[string]string{
		"enrollmentId": enrollment.ID, "challenge": enrollment.Challenge, "signature": enrollmentSignatureMarker,
	})
	if err != nil {
		t.Fatal(err)
	}
	badProof := call(http.MethodPost, "/v1/enrollments/complete", "", string(badProofBody), http.StatusBadRequest)
	assertError(badProof, "invalid_enrollment")
	wrongProofBody, err := json.Marshal(map[string]string{
		"enrollmentId": enrollment.ID, "challenge": enrollment.Challenge, "signature": encode(wrongEnrollmentSignature),
	})
	if err != nil {
		t.Fatal(err)
	}
	wrongProof := call(http.MethodPost, "/v1/enrollments/complete", "", string(wrongProofBody), http.StatusBadRequest)
	assertError(wrongProof, "invalid_enrollment")
	enrollmentSignature, err := ecdsa.SignASN1(rand.Reader, deviceKey, enrollmentDigest[:])
	if err != nil {
		t.Fatal(err)
	}
	enrollmentProof, err := json.Marshal(map[string]string{
		"enrollmentId": enrollment.ID, "challenge": enrollment.Challenge, "signature": encode(enrollmentSignature),
	})
	if err != nil {
		t.Fatal(err)
	}
	deviceResponse := call(http.MethodPost, "/v1/enrollments/complete", "", string(enrollmentProof), http.StatusOK)
	var device pairing.Device
	if err := json.Unmarshal(deviceResponse.Body.Bytes(), &device); err != nil {
		t.Fatal(err)
	}
	if device.ParentID != parentID || device.State != "active" || device.DisplayName != deviceNameMarker {
		t.Fatal("fixture enrollment did not create the expected device")
	}

	makeChallenge := func() pairing.AuthChallenge {
		t.Helper()
		requestBody, _ := json.Marshal(map[string]string{"deviceId": device.ID})
		response := call(http.MethodPost, "/v1/device-auth/challenges", "", string(requestBody), http.StatusCreated)
		var result pairing.AuthChallenge
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	signDeviceChallenge := func(ch pairing.AuthChallenge, key *ecdsa.PrivateKey) string {
		t.Helper()
		raw, decodeErr := base64.RawURLEncoding.DecodeString(ch.Challenge)
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		digest := sha256.Sum256(append([]byte("app-control-device-auth-v1\n"+device.ID+"\n"+ch.ID+"\n"), raw...))
		signature, signErr := ecdsa.SignASN1(rand.Reader, key, digest[:])
		if signErr != nil {
			t.Fatal(signErr)
		}
		return encode(signature)
	}
	failedChallenge := makeChallenge()
	badDeviceProof, err := json.Marshal(map[string]string{
		"deviceId": device.ID, "challengeId": failedChallenge.ID, "challenge": failedChallenge.Challenge,
		"signature": authSignatureMarker,
	})
	if err != nil {
		t.Fatal(err)
	}
	failedDeviceAuth := call(http.MethodPost, "/v1/device-auth/sessions", "", string(badDeviceProof), http.StatusUnauthorized)
	assertError(failedDeviceAuth, "invalid_device_credential")
	wrongChallenge := makeChallenge()
	wrongDeviceProof, err := json.Marshal(map[string]string{
		"deviceId": device.ID, "challengeId": wrongChallenge.ID, "challenge": wrongChallenge.Challenge,
		"signature": signDeviceChallenge(wrongChallenge, attackerKey),
	})
	if err != nil {
		t.Fatal(err)
	}
	failedDeviceAuth = call(http.MethodPost, "/v1/device-auth/sessions", "", string(wrongDeviceProof), http.StatusUnauthorized)
	assertError(failedDeviceAuth, "invalid_device_credential")
	deviceChallenge := makeChallenge()
	deviceSignature := signDeviceChallenge(deviceChallenge, deviceKey)
	deviceProof, err := json.Marshal(map[string]string{
		"deviceId": device.ID, "challengeId": deviceChallenge.ID, "challenge": deviceChallenge.Challenge,
		"signature": deviceSignature,
	})
	if err != nil {
		t.Fatal(err)
	}
	deviceSessionResponse := call(http.MethodPost, "/v1/device-auth/sessions", "", string(deviceProof), http.StatusCreated)
	var deviceSession pairing.DeviceSession
	if err := json.Unmarshal(deviceSessionResponse.Body.Bytes(), &deviceSession); err != nil {
		t.Fatal(err)
	}

	registrationBody, err := json.Marshal(map[string]string{"registrationToken": routingTokenMarker})
	if err != nil {
		t.Fatal(err)
	}
	failedRegistration := call(http.MethodPut, "/v1/device/wakeup-registration", "", string(registrationBody), http.StatusUnauthorized)
	assertError(failedRegistration, "invalid_device_credential")
	call(http.MethodPut, "/v1/device/wakeup-registration", deviceSession.AccessToken, string(registrationBody), http.StatusNoContent)

	contactsBody, err := json.Marshal(map[string]any{
		"expectedVersion": 0,
		"contacts": []map[string]any{{
			"label": contactNameMarker, "number": phoneNumberMarker, "isParent": true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	call(http.MethodPut, "/v1/devices/"+device.ID+"/call-contacts", parentAccess, string(contactsBody), http.StatusOK)
	policyBody, err := json.Marshal(map[string]any{
		"expectedVersion": 0, "internetPaused": true, "blockedPackages": []string{policyMarker},
		"alwaysAllowedPackages": []string{}, "schedules": []any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	call(http.MethodPut, "/v1/devices/"+device.ID+"/policy", parentAccess, string(policyBody), http.StatusOK)

	sequenceResponse := call(http.MethodPost, "/v1/device/calls/sequence", deviceSession.AccessToken, `{}`, http.StatusOK)
	var sequence struct {
		Sequence int64 `json:"sequence"`
	}
	if err := json.Unmarshal(sequenceResponse.Body.Bytes(), &sequence); err != nil {
		t.Fatal(err)
	}
	snapshotBody, err := json.Marshal(map[string]any{
		"sequence": sequence.Sequence,
		"calls": []map[string]any{{
			"callId": callID, "state": "RINGING", "remoteActionsAllowed": true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	call(http.MethodPost, "/v1/device/calls", deviceSession.AccessToken, string(snapshotBody), http.StatusOK)
	badSnapshot := strings.TrimSuffix(string(snapshotBody), "}") + `,"callerNumber":"` + callDetailsMarker + `"}`
	badCallSnapshot := call(http.MethodPost, "/v1/device/calls", deviceSession.AccessToken, badSnapshot, http.StatusBadRequest)
	assertError(badCallSnapshot, "invalid_request")
	callResponse := call(http.MethodGet, "/v1/devices/"+device.ID+"/calls", parentAccess, "", http.StatusOK)
	if strings.Contains(callResponse.Body.String(), phoneNumberMarker) || strings.Contains(callResponse.Body.String(), contactNameMarker) {
		t.Fatal("remote call response included contact data")
	}

	lockCommand := call(http.MethodPost, "/v1/devices/"+device.ID+"/commands", parentAccess, `{"type":"LOCK_DEVICE","ttlSeconds":60}`, http.StatusCreated)
	if !strings.Contains(lockCommand.Body.String(), `"type":"LOCK_DEVICE"`) {
		t.Fatal("lock command was not created")
	}
	callCommandBody, err := json.Marshal(map[string]any{
		"type": "REJECT_CALL", "callId": callID, "ttlSeconds": 15,
	})
	if err != nil {
		t.Fatal(err)
	}
	call(http.MethodPost, "/v1/devices/"+device.ID+"/commands", parentAccess, string(callCommandBody), http.StatusCreated)
	fetched := call(http.MethodGet, "/v1/device/commands", deviceSession.AccessToken, "", http.StatusOK)
	var delivered struct {
		Commands []control.Command `json:"commands"`
	}
	if err := json.Unmarshal(fetched.Body.Bytes(), &delivered); err != nil {
		t.Fatal(err)
	}
	if len(delivered.Commands) != 2 {
		t.Fatal("commands were not delivered to the authenticated device")
	}
	badAcknowledgement := call(http.MethodPost, "/v1/device/commands/"+delivered.Commands[0].ID+"/ack",
		deviceSession.AccessToken, `{"result":"SYNTHETIC_COMMAND_RESULT_MARKER"}`, http.StatusBadRequest)
	assertError(badAcknowledgement, "invalid_request")
	call(http.MethodPost, "/v1/device/commands/"+delivered.Commands[0].ID+"/ack",
		deviceSession.AccessToken, `{"result":"succeeded"}`, http.StatusNoContent)
	call(http.MethodPost, "/v1/device/commands/"+delivered.Commands[1].ID+"/ack",
		deviceSession.AccessToken, `{"result":"failed"}`, http.StatusNoContent)
	if len(fcmBodies) < 3 {
		t.Fatalf("expected wake-up hints for committed changes and commands, received %d", len(fcmBodies))
	}

	ready := call(http.MethodGet, "/readyz", "", "", http.StatusServiceUnavailable)
	if ready.Body.String() != "{\"status\":\"unavailable\"}\n" {
		t.Fatalf("readiness error was not fixed: %s", ready.Body.String())
	}
	auditResponse := call(http.MethodGet, "/v1/audit-events", parentAccess, "", http.StatusOK)
	var auditPage struct {
		Events []audit.Event `json:"events"`
	}
	if err := json.Unmarshal(auditResponse.Body.Bytes(), &auditPage); err != nil {
		t.Fatal(err)
	}
	auditJSON, err := json.Marshal(auditPage.Events)
	if err != nil {
		t.Fatal(err)
	}
	actions := make(map[string]bool)
	for _, event := range auditPage.Events {
		actions[event.Action] = true
	}
	for _, action := range []string{
		"parent_login", "parent_login_failed", "parent_refresh", "enrollment_completed",
		"enrollment_proof_failed", "device_authenticated", "device_auth_failed",
		"command_created", "command_delivered", "command_succeeded", "command_failed",
	} {
		if !actions[action] {
			t.Errorf("fixed security audit action %q was not preserved", action)
		}
	}

	auditFailure = errors.New(serverErrorMarker)
	serverError := call(http.MethodGet, "/v1/audit-events", parentAccess, "", http.StatusInternalServerError)
	assertError(serverError, "server_error")

	markers := []string{
		emailMarker, passwordMarker, wrongPasswordMarker, initialParentAccess, initialParentRefresh,
		parentAccess, parentRefresh, rotated.AccessToken, rotated.RefreshToken, deviceNameMarker,
		deviceKeyMarker, enrollmentSignatureMarker,
		authSignatureMarker, deviceSession.AccessToken, contactNameMarker, phoneNumberMarker, policyMarker,
		callDetailsMarker, routingTokenMarker, oauthTokenMarker, serverErrorMarker, databasePathMarker,
		invalidRefreshMarker, "SYNTHETIC_COMMAND_RESULT_MARKER", code.Code, enrollment.ID,
		enrollment.Challenge, encode(publicKeyDER), encode(enrollmentSignature), encode(wrongEnrollmentSignature),
		failedChallenge.Challenge, wrongChallenge.Challenge, deviceChallenge.Challenge, deviceSignature,
		deviceKey.D.Text(16), attackerKey.D.Text(16), callID,
	}
	var commandIDs []string
	for _, command := range delivered.Commands {
		commandIDs = append(commandIDs, command.ID)
	}
	for _, marker := range markers {
		if strings.Contains(logs.String(), marker) {
			t.Fatalf("request logs contain synthetic sensitive marker %q: %s", marker, logs.String())
		}
		if strings.Contains(string(auditJSON), marker) {
			t.Fatalf("audit events contain synthetic sensitive marker %q", marker)
		}
	}
	for _, commandID := range commandIDs {
		if strings.Contains(logs.String(), commandID) {
			t.Fatalf("request logs contain command identifier %q", commandID)
		}
	}
	for _, raw := range fcmBodies {
		var envelope struct {
			Message struct {
				Token string            `json:"token"`
				Data  map[string]string `json:"data"`
			} `json:"message"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Message.Token != routingTokenMarker || len(envelope.Message.Data) != 1 || envelope.Message.Data["syncHint"] == "" {
			t.Fatalf("unexpected FCM routing/data separation: %s", raw)
		}
		data, err := json.Marshal(envelope.Message.Data)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range append(append([]string(nil), markers...), commandIDs...) {
			if strings.Contains(string(data), marker) {
				t.Fatalf("FCM application data contains synthetic marker %q: %s", marker, data)
			}
		}
		for _, marker := range markers {
			if marker == routingTokenMarker {
				continue
			}
			if bytes.Contains(raw, []byte(marker)) {
				t.Fatalf("FCM request contains non-routing synthetic marker %q: %s", marker, raw)
			}
		}
	}
}
