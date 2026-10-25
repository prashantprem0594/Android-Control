package main

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
)

// This derives a public key fingerprint only. It does not establish certificate
// trust, validate a hostname or accept any private-key input.
func certificatePin(path string) (string, error) {
	certificates, err := readCertificateChain(path)
	if err != nil {
		return "", err
	}
	if len(certificates) != 1 {
		return "", errors.New("single PEM certificate required")
	}
	digest := sha256.Sum256(certificates[0].RawSubjectPublicKeyInfo)
	return base64.StdEncoding.EncodeToString(digest[:]), nil
}
