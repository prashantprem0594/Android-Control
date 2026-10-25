package auth

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"appcontrol/server/internal/storage"
)

const testPassword = "correct horse battery staple"

type testClock struct{ current time.Time }

func (clock *testClock) Now() time.Time { return clock.current }
func (clock *testClock) Advance(duration time.Duration) {
	clock.current = clock.current.Add(duration)
}

func newTestService(t *testing.T, configure func(*Options)) (*Service, *sql.DB, *testClock) {
	t.Helper()
	ctx := context.Background()
	database, err := storage.Open(ctx, filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	clock := &testClock{current: time.Date(2026, 10, 23, 8, 0, 0, 0, time.UTC)}
	options := Options{
		Now: clock.Now,
		PasswordParameters: PasswordParameters{
			MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1, SaltBytes: 16, KeyBytes: 32,
		},
		AccessLifetime: 5 * time.Minute, RefreshLifetime: time.Hour,
		LoginWindow: time.Minute, LoginBlockDuration: 2 * time.Minute, MaxLoginFailures: 3,
	}
	if configure != nil {
		configure(&options)
	}
	service, err := NewService(database, options)
	if err != nil {
		t.Fatalf("create auth service: %v", err)
	}
	return service, database, clock
}

func registerTestParent(t *testing.T, service *Service) Parent {
	t.Helper()
	parent, err := service.Register(context.Background(), " Parent@Example.com ", testPassword)
	if err != nil {
		t.Fatalf("register parent: %v", err)
	}
	if parent.Email != "parent@example.com" {
		t.Fatalf("expected canonical email, got %q", parent.Email)
	}
	return parent
}

func TestRegisterHashesPasswordAndRejectsDuplicate(t *testing.T) {
	service, database, _ := newTestService(t, nil)
	parent := registerTestParent(t, service)

	var storedHash string
	if err := database.QueryRow("SELECT password_hash FROM parent_accounts WHERE id = ?", parent.ID).Scan(&storedHash); err != nil {
		t.Fatalf("load stored password hash: %v", err)
	}
	if strings.Contains(storedHash, testPassword) || !strings.HasPrefix(storedHash, "$argon2id$v=19$") {
		t.Fatalf("password was not stored as an Argon2id hash")
	}
	if _, err := service.Register(context.Background(), "parent@example.com", testPassword); !errors.Is(err, ErrEmailExists) {
		t.Fatalf("expected duplicate email error, got %v", err)
	}
}

func TestLoginAcceptsCorrectPasswordAndRejectsIncorrectPassword(t *testing.T) {
	service, _, _ := newTestService(t, nil)
	parent := registerTestParent(t, service)

	if _, _, err := service.Login(context.Background(), parent.Email, "wrong-password-value"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected invalid credentials, got %v", err)
	}
	authenticated, pair, err := service.Login(context.Background(), strings.ToUpper(parent.Email), testPassword)
	if err != nil {
		t.Fatalf("login with correct password: %v", err)
	}
	if authenticated.ID != parent.ID || !validTokenShape(pair.AccessToken, "at_") || !validTokenShape(pair.RefreshToken, "rt_") {
		t.Fatalf("login returned invalid identity or tokens")
	}
	resolved, _, err := service.Authenticate(context.Background(), pair.AccessToken)
	if err != nil || resolved.ID != parent.ID {
		t.Fatalf("authenticate access token: parent=%+v err=%v", resolved, err)
	}
}

func TestLoginRateLimitBlocksEvenCorrectPasswordUntilExpiry(t *testing.T) {
	service, _, clock := newTestService(t, nil)
	parent := registerTestParent(t, service)

	for attempt := 0; attempt < 3; attempt++ {
		if _, _, err := service.Login(context.Background(), parent.Email, "wrong-password-value"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d: expected invalid credentials, got %v", attempt+1, err)
		}
	}
	if _, _, err := service.Login(context.Background(), parent.Email, testPassword); !errors.Is(err, ErrLoginRateLimited) {
		t.Fatalf("expected rate limiting, got %v", err)
	}
	clock.Advance(2*time.Minute + time.Second)
	if _, _, err := service.Login(context.Background(), parent.Email, testPassword); err != nil {
		t.Fatalf("expected login after block expires, got %v", err)
	}
}

func TestRefreshRotatesTokenAndReuseRevokesFamily(t *testing.T) {
	service, _, _ := newTestService(t, nil)
	parent := registerTestParent(t, service)
	_, original, err := service.Login(context.Background(), parent.Email, testPassword)
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	rotated, err := service.Refresh(context.Background(), original.RefreshToken)
	if err != nil {
		t.Fatalf("rotate refresh token: %v", err)
	}
	if rotated.RefreshToken == original.RefreshToken || rotated.AccessToken == original.AccessToken {
		t.Fatalf("refresh did not issue new tokens")
	}
	if _, _, err := service.Authenticate(context.Background(), rotated.AccessToken); err != nil {
		t.Fatalf("rotated access token should initially work: %v", err)
	}
	if _, err := service.Refresh(context.Background(), original.RefreshToken); !errors.Is(err, ErrRefreshReuse) {
		t.Fatalf("expected refresh reuse detection, got %v", err)
	}
	if _, _, err := service.Authenticate(context.Background(), rotated.AccessToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("reuse should revoke the entire family, got %v", err)
	}
	if _, err := service.Refresh(context.Background(), rotated.RefreshToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("family refresh token should be revoked, got %v", err)
	}
}

func TestLogoutRevokesAccessAndRefreshTokens(t *testing.T) {
	service, _, _ := newTestService(t, nil)
	parent := registerTestParent(t, service)
	_, pair, err := service.Login(context.Background(), parent.Email, testPassword)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if err := service.Logout(context.Background(), pair.AccessToken); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, _, err := service.Authenticate(context.Background(), pair.AccessToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("logged-out access token should fail, got %v", err)
	}
	if _, err := service.Refresh(context.Background(), pair.RefreshToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("logged-out refresh token should fail, got %v", err)
	}
}

func TestTokensAreStoredOnlyAsHashes(t *testing.T) {
	service, database, _ := newTestService(t, nil)
	parent := registerTestParent(t, service)
	_, pair, err := service.Login(context.Background(), parent.Email, testPassword)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	for _, table := range []string{"auth_access_tokens", "auth_refresh_tokens"} {
		var hash []byte
		if err := database.QueryRow("SELECT token_hash FROM " + table + " LIMIT 1").Scan(&hash); err != nil {
			t.Fatalf("load %s hash: %v", table, err)
		}
		if len(hash) != sha256Size || string(hash) == pair.AccessToken || string(hash) == pair.RefreshToken {
			t.Fatalf("%s did not store a fixed-size token hash", table)
		}
	}
}

const sha256Size = 32
