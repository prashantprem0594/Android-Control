package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

func main() {
	mode := flag.String("mode", "", "")
	dir := flag.String("dir", "", "")
	listen := flag.String("listen", "", "")
	upstream := flag.String("upstream", "", "")
	readyFile := flag.String("ready-file", "", "")
	flag.Parse()
	var err error
	switch *mode {
	case "generate":
		err = generate(*dir)
	case "serve":
		err = serve(*listen, *upstream, *dir, *readyFile)
	default:
		err = fmt.Errorf("invalid mode")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "acceptance helper failed")
		os.Exit(1)
	}
}

func generate(dir string) error {
	if dir == "" {
		return fmt.Errorf("missing directory")
	}
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	now := time.Now()
	rootTemplate := &x509.Certificate{
		SerialNumber:          randomSerial(),
		Subject:               pkix.Name{CommonName: "App Control Acceptance Fixture Root"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	if err != nil {
		return err
	}
	root, err := x509.ParseCertificate(rootDER)
	if err != nil {
		return err
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: randomSerial(),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("10.0.2.2")},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(12 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, root, &leafKey.PublicKey, rootKey)
	if err != nil {
		return err
	}
	leaf, err := x509.ParseCertificate(leafDER)
	if err != nil {
		return err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(leafKey)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0700); err != nil {
		return err
	}
	if err := writePEM(filepath.Join(dir, "assets", "acceptance-root-ca.pem"), "CERTIFICATE", rootDER, 0600); err != nil {
		return err
	}
	if err := writePEM(filepath.Join(dir, "server-cert.pem"), "CERTIFICATE", leafDER, 0600); err != nil {
		return err
	}
	if err := writePEM(filepath.Join(dir, "server-key.pem"), "PRIVATE KEY", keyDER, 0600); err != nil {
		return err
	}
	parentRoutingKey := make([]byte, 32)
	defer clear(parentRoutingKey)
	if _, err := rand.Read(parentRoutingKey); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "parent-routing-key.bin"), parentRoutingKey, 0600); err != nil {
		return err
	}
	digest := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	if err := os.WriteFile(filepath.Join(dir, "assets", "acceptance-pin.txt"),
		[]byte("sha256/"+base64.StdEncoding.EncodeToString(digest[:])+"\n"), 0600); err != nil {
		return err
	}
	generatedSource := filepath.Join(dir, "generated", "com", "appcontrol", "transport")
	if err := os.MkdirAll(generatedSource, 0700); err != nil {
		return err
	}
	source := fmt.Sprintf("package com.appcontrol.transport;\n\npublic final class AcceptanceTrustFixtureGenerated {\n"+
		"    public static final String ROOT_CERTIFICATE_BASE64 = %s;\n"+
		"    public static final String LEAF_SPKI_PIN = %s;\n"+
		"    private AcceptanceTrustFixtureGenerated() {}\n}\n",
		strconv.Quote(base64.StdEncoding.EncodeToString(rootDER)),
		strconv.Quote("sha256/"+base64.StdEncoding.EncodeToString(digest[:])))
	if err := os.WriteFile(filepath.Join(generatedSource, "AcceptanceTrustFixtureGenerated.java"), []byte(source), 0600); err != nil {
		return err
	}
	fmt.Printf("fixture TLS material created; leaf SPKI pin sha256/%s\n", base64.StdEncoding.EncodeToString(digest[:]))
	return nil
}

func serve(address, upstreamAddress, dir, readyFile string) error {
	if !strings.HasPrefix(address, "127.0.0.1:") || !strings.HasPrefix(upstreamAddress, "127.0.0.1:") || readyFile == "" {
		return fmt.Errorf("invalid local listener configuration")
	}
	target, err := url.Parse("http://" + upstreamAddress)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	defer listener.Close()
	certificate, err := tls.LoadX509KeyPair(filepath.Join(dir, "server-cert.pem"), filepath.Join(dir, "server-key.pem"))
	if err != nil {
		return err
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorLog = log.New(io.Discard, "", 0)
	proxy.Transport = &http.Transport{
		Proxy: nil,
		DialContext: (&net.Dialer{
			Timeout: 3 * time.Second,
		}).DialContext,
		ResponseHeaderTimeout: 12 * time.Second,
		IdleConnTimeout:       15 * time.Second,
	}
	server := &http.Server{
		Handler:           contactFailureFixture(proxy),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       15 * time.Second,
		ErrorLog:          log.New(io.Discard, "", 0),
	}
	if err := os.WriteFile(readyFile, []byte("ready\n"), 0600); err != nil {
		return err
	}
	tlsListener := tls.NewListener(listener, &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12})
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	done := make(chan error, 1)
	go func() { done <- server.Serve(tlsListener) }()
	select {
	case <-signals:
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			return err
		}
		err := <-done
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	case err := <-done:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	}
}

// This handler exists only in the disposable loopback acceptance proxy.
// It never changes the production server or interrupts command/policy routes.
func contactFailureFixture(upstream http.Handler) http.Handler {
	var unavailable atomic.Bool
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/__acceptance/contacts-unavailable", "/__acceptance/contacts-available":
			if r.Method != http.MethodPost {
				w.Header().Set("Allow", http.MethodPost)
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			unavailable.Store(r.URL.Path == "/__acceptance/contacts-unavailable")
			w.WriteHeader(http.StatusNoContent)
			return
		case "/v1/device/call-contacts":
			if unavailable.Load() {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = io.WriteString(w, `{"error":"acceptance_contacts_unavailable"}`)
				return
			}
		}
		upstream.ServeHTTP(w, r)
	})
}

func randomSerial() *big.Int {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		panic("random certificate serial unavailable")
	}
	return serial
}

func writePEM(path, kind string, bytes []byte, mode os.FileMode) error {
	return os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: bytes}), mode)
}
