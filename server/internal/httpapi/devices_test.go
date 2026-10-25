package httpapi

import (
	"appcontrol/server/internal/auth"
	"appcontrol/server/internal/pairing"
	"appcontrol/server/internal/storage"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestDeviceListHTTPIsolationAndValidation(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "devices.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	authentication, err := auth.NewService(db, auth.Options{PasswordParameters: auth.PasswordParameters{MemoryKiB: 8192, Iterations: 1, Parallelism: 1, SaltBytes: 16, KeyBytes: 32}})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := authentication.Register(ctx, "owner@example.com", "long-fixture-password")
	if err != nil {
		t.Fatal(err)
	}
	service, err := pairing.NewService(db, pairing.Options{})
	if err != nil {
		t.Fatal(err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		code, err := service.CreateCode(ctx, owner.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.StartEnrollment(ctx, code.Code, "Test device", base64.RawURLEncoding.EncodeToString(der), pairing.KeyAlgorithmECDSAP256SHA256); err != nil {
			t.Fatal(err)
		}
	}
	fake := completeFakeAuth()
	fake.authenticate = func(_ context.Context, token string) (auth.Parent, string, error) {
		if token == "owner" {
			return owner, "", nil
		}
		if token == "other" {
			return auth.Parent{ID: "other-parent"}, "", nil
		}
		return auth.Parent{}, "", auth.ErrInvalidCredentials
	}
	handler := NewHandler(Options{Auth: fake, Pairing: service})
	call := func(query, token string, expected int) pairing.DevicePage {
		t.Helper()
		request := httptest.NewRequest("GET", "/v1/devices"+query, nil)
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != expected {
			t.Fatalf("list status=%d expected=%d", response.Code, expected)
		}
		var page pairing.DevicePage
		if expected == 200 {
			if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
				t.Fatal(err)
			}
		}
		return page
	}
	call("", "", 401)
	call("", "bad", 401)
	first := call("?limit=1", "owner", 200)
	if len(first.Devices) != 1 || first.NextCursor == "" {
		t.Fatal("first page missing continuation")
	}
	second := call("?limit=1&after="+first.NextCursor, "owner", 200)
	if len(second.Devices) != 1 || second.NextCursor != "" || second.Devices[0].ID == first.Devices[0].ID {
		t.Fatal("second page repeated or lost device")
	}
	for _, query := range []string{"", "?after=" + first.NextCursor} {
		page := call(query, "other", 200)
		if page.Devices == nil || len(page.Devices) != 0 || page.NextCursor != "" {
			t.Fatal("parent isolation or empty JSON list failed")
		}
	}
	for _, query := range []string{"?limit=0", "?limit=101", "?limit=x", "?limit=", "?after=bad", "?limit=1&limit=2", "?unknown=1", "?after=%zz"} {
		call(query, "owner", 400)
	}
}
