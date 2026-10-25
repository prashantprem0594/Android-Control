package auth

import (
	"strings"
	"testing"
)

func TestPasswordHashRoundTripAndValidation(t *testing.T) {
	parameters := PasswordParameters{MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1, SaltBytes: 16, KeyBytes: 32}
	encoded, err := hashPassword(testPassword, parameters, nil)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	valid, err := verifyPassword(encoded, testPassword)
	if err != nil || !valid {
		t.Fatalf("verify correct password: valid=%t err=%v", valid, err)
	}
	valid, err = verifyPassword(encoded, "incorrect-password-value")
	if err != nil || valid {
		t.Fatalf("verify incorrect password: valid=%t err=%v", valid, err)
	}
	if _, err := hashPassword("too-short", parameters, nil); err == nil {
		t.Fatal("expected short password rejection")
	}
	malicious := "$argon2id$v=19$m=4294967295,t=1,p=1$" + strings.Repeat("A", 22) + "$" + strings.Repeat("A", 43)
	if _, err := verifyPassword(malicious, testPassword); err == nil {
		t.Fatal("expected excessive memory parameter rejection")
	}
}
