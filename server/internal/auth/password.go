package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/argon2"
)

type PasswordParameters struct {
	MemoryKiB   uint32
	Iterations  uint32
	Parallelism uint8
	SaltBytes   uint32
	KeyBytes    uint32
}

var DefaultPasswordParameters = PasswordParameters{
	MemoryKiB: 64 * 1024, Iterations: 3, Parallelism: 2, SaltBytes: 16, KeyBytes: 32,
}

func hashPassword(password string, parameters PasswordParameters, random io.Reader) (string, error) {
	if err := validatePassword(password); err != nil {
		return "", err
	}
	if err := validatePasswordParameters(parameters); err != nil {
		return "", err
	}
	if random == nil {
		random = rand.Reader
	}
	salt := make([]byte, parameters.SaltBytes)
	if _, err := io.ReadFull(random, salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, parameters.Iterations, parameters.MemoryKiB, parameters.Parallelism, parameters.KeyBytes)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		parameters.MemoryKiB,
		parameters.Iterations,
		parameters.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

func verifyPassword(encoded, password string) (bool, error) {
	parameters, salt, expected, err := parsePasswordHash(encoded)
	if err != nil {
		return false, err
	}
	actual := argon2.IDKey([]byte(password), salt, parameters.Iterations, parameters.MemoryKiB, parameters.Parallelism, parameters.KeyBytes)
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

func parsePasswordHash(encoded string) (PasswordParameters, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" {
		return PasswordParameters{}, nil, nil, fmt.Errorf("invalid password hash format")
	}
	var parameters PasswordParameters
	var parallelism uint64
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &parameters.MemoryKiB, &parameters.Iterations, &parallelism); err != nil || parallelism > 255 {
		return PasswordParameters{}, nil, nil, fmt.Errorf("invalid password hash parameters")
	}
	parameters.Parallelism = uint8(parallelism)
	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil {
		return PasswordParameters{}, nil, nil, fmt.Errorf("decode password salt: %w", err)
	}
	key, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil {
		return PasswordParameters{}, nil, nil, fmt.Errorf("decode password key: %w", err)
	}
	parameters.SaltBytes = uint32(len(salt))
	parameters.KeyBytes = uint32(len(key))
	if err := validatePasswordParameters(parameters); err != nil {
		return PasswordParameters{}, nil, nil, err
	}
	return parameters, salt, key, nil
}

func validatePassword(password string) error {
	if len(password) < 12 || len(password) > 1024 {
		return fmt.Errorf("password must be between 12 and 1024 bytes")
	}
	return nil
}

func validatePasswordParameters(parameters PasswordParameters) error {
	if parameters.MemoryKiB < 8*1024 || parameters.MemoryKiB > 1024*1024 ||
		parameters.Iterations < 1 || parameters.Iterations > 20 ||
		parameters.Parallelism < 1 || parameters.Parallelism > 16 ||
		parameters.SaltBytes < 16 || parameters.SaltBytes > 64 ||
		parameters.KeyBytes < 16 || parameters.KeyBytes > 64 {
		return fmt.Errorf("password parameters are outside supported limits")
	}
	return nil
}
