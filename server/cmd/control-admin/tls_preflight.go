package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"time"
)

type rotationPreflight struct {
	Hostname        string    `json:"hostname"`
	CurrentPin      string    `json:"currentPin"`
	BackupPin       string    `json:"backupPin"`
	CurrentNotAfter time.Time `json:"currentNotAfter"`
	BackupNotAfter  time.Time `json:"backupNotAfter"`
	CheckedAt       time.Time `json:"checkedAt"`
}

func certificatePreflight(ctx context.Context, host, currentPath, backupPath string, out io.Writer) error {
	current, err := readCertificateChain(currentPath)
	if err != nil {
		return err
	}
	backup, err := readCertificateChain(backupPath)
	if err != nil {
		return err
	}
	// nil roots deliberately uses the operator host's system trust store. A root
	// supplied inside a chain is never promoted to a trusted root.
	result, err := validateRotationCertificates(host, current, backup, nil, time.Now().UTC())
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(result)
}

func readCertificateChain(path string) ([]*x509.Certificate, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("certificate chain unavailable")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, errors.New("regular certificate chain file required")
	}
	data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return nil, errors.New("certificate chain read failed")
	}
	var certificates []*x509.Certificate
	for len(bytes.TrimSpace(data)) > 0 {
		data = bytes.TrimSpace(data)
		if len(certificates) >= 16 || !bytes.HasPrefix(data, []byte("-----BEGIN CERTIFICATE-----")) {
			return nil, errors.New("certificate-only PEM chain required")
		}
		block, rest := pem.Decode(data)
		if block == nil || block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			return nil, errors.New("certificate-only PEM chain required")
		}
		// pem.Decode can skip malformed PEM before a later valid block. Reject
		// skipped material rather than silently accepting an ambiguous bundle.
		if bytes.Count(data[:len(data)-len(rest)], []byte("-----BEGIN")) != 1 {
			return nil, errors.New("malformed certificate chain prefix")
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, errors.New("invalid certificate chain")
		}
		certificates = append(certificates, certificate)
		data = rest
	}
	if len(certificates) == 0 {
		return nil, errors.New("empty certificate chain")
	}
	return certificates, nil
}

func validateRotationCertificates(host string, current, backup []*x509.Certificate, roots *x509.CertPool, now time.Time) (rotationPreflight, error) {
	result := rotationPreflight{}
	host = strings.ToLower(host)
	if len(host) == 0 || len(host) > 253 || net.ParseIP(host) != nil || !strings.ContainsAny(host, "abcdefghijklmnopqrstuvwxyz") {
		return result, errors.New("deployment DNS hostname required")
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return result, errors.New("deployment DNS hostname required")
		}
		for _, character := range label {
			if !(character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-') {
				return result, errors.New("deployment DNS hostname required")
			}
		}
	}
	pins := make([]string, 0, 2)
	for _, chain := range [][]*x509.Certificate{current, backup} {
		if len(chain) == 0 || chain[0].IsCA {
			return result, errors.New("leaf-first server certificate chain required")
		}
		intermediates := x509.NewCertPool()
		for _, certificate := range chain[1:] {
			intermediates.AddCert(certificate)
		}
		if _, err := chain[0].Verify(x509.VerifyOptions{DNSName: host, Roots: roots, Intermediates: intermediates, CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err != nil {
			return result, errors.New("server certificate trust, hostname or validity check failed")
		}
		digest := sha256.Sum256(chain[0].RawSubjectPublicKeyInfo)
		pins = append(pins, base64.StdEncoding.EncodeToString(digest[:]))
	}
	if pins[0] == pins[1] {
		return result, errors.New("rotation requires distinct current and backup keys")
	}
	return rotationPreflight{host, pins[0], pins[1], current[0].NotAfter, backup[0].NotAfter, now}, nil
}
