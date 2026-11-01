package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
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
	"syscall"
	"time"
)

const scope = "laptop-development-bootstrap"
const metadataName = "laptop-server.json"
const shutdownPath = "/__appcontrol_laptop/shutdown"

var invalid = errors.New("invalid or incomplete laptop state")

type metadata struct {
	SchemaVersion           int    `json:"schemaVersion"`
	Scope                   string `json:"scope"`
	HTTPSOrigin             string `json:"httpsOrigin"`
	RootCertificateFile     string `json:"rootCertificateFile"`
	RootCertificateSHA256   string `json:"rootCertificateSHA256"`
	CurrentPin              string `json:"currentPin"`
	BackupPin               string `json:"backupPin"`
	BackendAddress          string `json:"backendAddress"`
	CertificatesExpireAtUTC string `json:"certificatesExpireAtUtc"`
}
type state struct {
	metadata
	root          *x509.Certificate
	pair          tls.Certificate
	shutdownToken []byte
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "Laptop server operation failed. Check state, ownership, certificate lifetime and local ports.")
		os.Exit(1)
	}
}
func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		return invalid
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	directory := flags.String("state-dir", "", "")
	httpsPort := flags.Int("https-port", 8443, "")
	backendPort := flags.Int("backend-port", 18080, "")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || *directory == "" {
		return invalid
	}
	switch args[0] {
	case "init":
		if err := initialize(*directory, *httpsPort, *backendPort, time.Now().UTC()); err != nil {
			return err
		}
		fmt.Fprintln(out, "New isolated laptop identities created. Public metadata is laptop-server.json; private material was not printed.")
	case "verify-state":
		if _, err := loadState(*directory, time.Now()); err != nil {
			return err
		}
		fmt.Fprintln(out, "Existing laptop identities verified; no files changed.")
	case "check", "shutdown":
		current, err := loadState(*directory, time.Now())
		if err != nil {
			return err
		}
		if err := request(current, args[0] == "shutdown"); err != nil {
			return err
		}
		fmt.Fprintln(out, "Pinned localhost HTTPS operation passed.")
	case "serve":
		current, err := loadState(*directory, time.Now())
		if err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return serve(ctx, current, out)
	default:
		return invalid
	}
	return nil
}
func safePath(path string) error {
	full, err := filepath.Abs(path)
	if err != nil {
		return invalid
	}
	for current := full; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return invalid
		}
		if filepath.Dir(current) == current {
			break
		}
	}
	return nil
}
func writeNew(path string, bytes []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err = file.Write(bytes); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}
func key() (*ecdsa.PrivateKey, error) { return ecdsa.GenerateKey(elliptic.P256(), rand.Reader) }
func serial() (*big.Int, error)       { return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128)) }
func keyPEM(value *ecdsa.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(value)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}
func certificatePEM(der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
func pin(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return base64.StdEncoding.EncodeToString(sum[:])
}
func fingerprint(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func validPort(value int) bool      { return value >= 1 && value <= 65535 }
func originForPort(port int) string {
	if port == 443 {
		return "https://localhost"
	}
	return "https://localhost:" + strconv.Itoa(port)
}
func initialize(directory string, httpsPort, backendPort int, now time.Time) error {
	if !validPort(httpsPort) || !validPort(backendPort) || httpsPort == backendPort {
		return invalid
	}
	if _, err := os.Stat(directory); errors.Is(err, os.ErrNotExist) {
		if safePath(filepath.Dir(directory)) != nil {
			return invalid
		}
		if err := os.Mkdir(directory, 0700); err != nil {
			return err
		}
	}
	if safePath(directory) != nil {
		return invalid
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 0 {
		return invalid
	}
	if err := writeNew(filepath.Join(directory, ".initializing"), []byte("incomplete until metadata is published\n"), 0600); err != nil {
		return err
	}
	private := filepath.Join(directory, "private")
	if err := os.Mkdir(private, 0700); err != nil {
		return err
	}
	if err := os.Mkdir(filepath.Join(directory, "data"), 0700); err != nil {
		return err
	}
	rootKey, err := key()
	if err != nil {
		return err
	}
	rootSerial, err := serial()
	if err != nil {
		return err
	}
	rootTemplate := &x509.Certificate{SerialNumber: rootSerial, Subject: pkix.Name{CommonName: "App Control laptop development CA"}, NotBefore: now.Add(-5 * time.Minute), NotAfter: now.Add(365 * 24 * time.Hour), IsCA: true, BasicConstraintsValid: true, MaxPathLen: 0, MaxPathLenZero: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	if err != nil {
		return err
	}
	rootCert, err := x509.ParseCertificate(rootDER)
	if err != nil {
		return err
	}
	rootPrivate, err := keyPEM(rootKey)
	if err != nil {
		return err
	}
	defer clear(rootPrivate)
	if err := writeNew(filepath.Join(private, "root-ca.key"), rootPrivate, 0600); err != nil {
		return err
	}
	rootPEM := certificatePEM(rootDER)
	if err := writeNew(filepath.Join(directory, "root-ca.pem"), rootPEM, 0644); err != nil {
		return err
	}
	pins := make([]string, 0, 2)
	expiry := now.Add(90 * 24 * time.Hour)
	for _, name := range []string{"leaf", "backup-leaf"} {
		leafKey, err := key()
		if err != nil {
			return err
		}
		leafSerial, err := serial()
		if err != nil {
			return err
		}
		template := &x509.Certificate{SerialNumber: leafSerial, Subject: pkix.Name{CommonName: "localhost"}, DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}, NotBefore: now.Add(-5 * time.Minute), NotAfter: expiry, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true}
		der, err := x509.CreateCertificate(rand.Reader, template, rootCert, &leafKey.PublicKey, rootKey)
		if err != nil {
			return err
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return err
		}
		pins = append(pins, pin(cert))
		encoded, err := keyPEM(leafKey)
		if err != nil {
			return err
		}
		err = writeNew(filepath.Join(private, name+".key"), encoded, 0600)
		clear(encoded)
		if err != nil {
			return err
		}
		if err := writeNew(filepath.Join(directory, name+".pem"), certificatePEM(der), 0644); err != nil {
			return err
		}
	}
	for _, name := range []string{"backup-key.bin", "parent-routing-key.bin", "child-routing-key.bin", "shutdown-token.bin"} {
		bytes := make([]byte, 32)
		if _, err := rand.Read(bytes); err != nil {
			return err
		}
		err := writeNew(filepath.Join(private, name), bytes, 0600)
		clear(bytes)
		if err != nil {
			return err
		}
	}
	public := metadata{SchemaVersion: 1, Scope: scope, HTTPSOrigin: originForPort(httpsPort), RootCertificateFile: "root-ca.pem", RootCertificateSHA256: fingerprint(rootPEM), CurrentPin: pins[0], BackupPin: pins[1], BackendAddress: net.JoinHostPort("127.0.0.1", strconv.Itoa(backendPort)), CertificatesExpireAtUTC: expiry.Format(time.RFC3339)}
	raw, err := json.MarshalIndent(public, "", "  ")
	if err != nil {
		return err
	}
	if err := writeNew(filepath.Join(directory, metadataName), append(raw, '\n'), 0644); err != nil {
		return err
	}
	return os.Remove(filepath.Join(directory, ".initializing"))
}
func readBounded(path string, maximum int) ([]byte, error) {
	if safePath(path) != nil {
		return nil, invalid
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > int64(maximum) {
		return nil, invalid
	}
	raw, err := io.ReadAll(io.LimitReader(file, int64(maximum+1)))
	if err != nil || len(raw) > maximum {
		return nil, invalid
	}
	return raw, nil
}
func parseCertificate(raw []byte) (*x509.Certificate, error) {
	block, rest := pem.Decode(raw)
	if block == nil || block.Type != "CERTIFICATE" || len(strings.TrimSpace(string(rest))) != 0 {
		return nil, invalid
	}
	return x509.ParseCertificate(block.Bytes)
}
func loadState(directory string, now time.Time) (*state, error) {
	if safePath(directory) != nil {
		return nil, invalid
	}
	if _, err := os.Lstat(filepath.Join(directory, ".initializing")); err == nil || !errors.Is(err, os.ErrNotExist) {
		return nil, invalid
	}
	raw, err := readBounded(filepath.Join(directory, metadataName), 16*1024)
	if err != nil {
		return nil, invalid
	}
	var value metadata
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&value) != nil || decoder.Decode(new(any)) != io.EOF || value.SchemaVersion != 1 || value.Scope != scope || value.RootCertificateFile != "root-ca.pem" {
		return nil, invalid
	}
	origin, err := url.Parse(value.HTTPSOrigin)
	if err != nil || origin.Scheme != "https" || origin.Hostname() != "localhost" || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
		return nil, invalid
	}
	originPort := origin.Port()
	if originPort == "" {
		originPort = "443"
	}
	httpsPort, err := strconv.Atoi(originPort)
	if err != nil || !validPort(httpsPort) || value.HTTPSOrigin != originForPort(httpsPort) {
		return nil, invalid
	}
	host, port, err := net.SplitHostPort(value.BackendAddress)
	if err != nil || host != "127.0.0.1" {
		return nil, invalid
	}
	backendPort, err := strconv.Atoi(port)
	if err != nil || !validPort(backendPort) || backendPort == httpsPort || port != strconv.Itoa(backendPort) {
		return nil, invalid
	}
	rootRaw, err := readBounded(filepath.Join(directory, "root-ca.pem"), 64*1024)
	if err != nil || fingerprint(rootRaw) != value.RootCertificateSHA256 {
		return nil, invalid
	}
	root, err := parseCertificate(rootRaw)
	if err != nil || !root.IsCA || root.CheckSignatureFrom(root) != nil || now.Before(root.NotBefore) || !now.Before(root.NotAfter) {
		return nil, invalid
	}
	roots := x509.NewCertPool()
	roots.AddCert(root)
	var active tls.Certificate
	for index, name := range []string{"leaf", "backup-leaf"} {
		certRaw, err := readBounded(filepath.Join(directory, name+".pem"), 64*1024)
		if err != nil {
			return nil, invalid
		}
		cert, err := parseCertificate(certRaw)
		if err != nil {
			return nil, invalid
		}
		if _, err := cert.Verify(x509.VerifyOptions{Roots: roots, DNSName: "localhost", CurrentTime: now}); err != nil {
			return nil, invalid
		}
		expected := value.CurrentPin
		if index == 1 {
			expected = value.BackupPin
		}
		if pin(cert) != expected || cert.NotAfter.Format(time.RFC3339) != value.CertificatesExpireAtUTC {
			return nil, invalid
		}
		private, err := readBounded(filepath.Join(directory, "private", name+".key"), 16*1024)
		if err != nil {
			return nil, invalid
		}
		pair, err := tls.X509KeyPair(certRaw, private)
		clear(private)
		if err != nil {
			return nil, invalid
		}
		if index == 0 {
			active = pair
		}
	}
	if value.CurrentPin == value.BackupPin {
		return nil, invalid
	}
	for _, name := range []string{"backup-key.bin", "parent-routing-key.bin", "child-routing-key.bin"} {
		raw, err := readBounded(filepath.Join(directory, "private", name), 32)
		if err != nil || len(raw) != 32 {
			return nil, invalid
		}
		clear(raw)
	}
	rootPrivate, err := readBounded(filepath.Join(directory, "private", "root-ca.key"), 16*1024)
	if err != nil {
		return nil, invalid
	}
	rootPair, err := tls.X509KeyPair(rootRaw, rootPrivate)
	clear(rootPrivate)
	if err != nil || rootPair.PrivateKey == nil {
		return nil, invalid
	}
	token, err := readBounded(filepath.Join(directory, "private", "shutdown-token.bin"), 32)
	if err != nil || len(token) != 32 {
		return nil, invalid
	}
	return &state{metadata: value, root: root, pair: active, shutdownToken: token}, nil
}
func client(current *state) *http.Client {
	pool := x509.NewCertPool()
	pool.AddCert(current.root)
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, ServerName: "localhost", MinVersion: tls.VersionTLS12, VerifyConnection: func(connection tls.ConnectionState) error {
		if len(connection.VerifiedChains) == 0 || len(connection.PeerCertificates) == 0 || pin(connection.PeerCertificates[0]) != current.CurrentPin {
			return invalid
		}
		return nil
	}}}
	return &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func request(current *state, shutdown bool) error {
	method, path := http.MethodGet, "/readyz"
	if shutdown {
		method, path = http.MethodPost, shutdownPath
	}
	call, err := http.NewRequest(method, current.HTTPSOrigin+path, nil)
	if err != nil {
		return invalid
	}
	if shutdown {
		call.Header.Set("X-App-Control-Laptop-Token", base64.RawURLEncoding.EncodeToString(current.shutdownToken))
	}
	c := client(current)
	defer c.CloseIdleConnections()
	response, err := c.Do(call)
	if err != nil {
		return invalid
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return invalid
	}
	_, err = io.Copy(io.Discard, io.LimitReader(response.Body, 16385))
	return err
}
func handler(current *state, stop func()) (http.Handler, error) {
	upstream, err := url.Parse("http://" + current.BackendAddress)
	if err != nil {
		return nil, invalid
	}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	original := proxy.Director
	proxy.Director = func(call *http.Request) {
		original(call)
		call.Header.Del("Forwarded")
		call.Header.Del("X-Real-IP")
		call.Header.Del("X-Forwarded-For")
		call.Header.Set("X-Forwarded-For", "127.0.0.1")
		call.Header.Set("X-Forwarded-Proto", "https")
	}
	proxy.ErrorLog = log.New(io.Discard, "", 0)
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		http.Error(w, "Local backend unavailable", http.StatusBadGateway)
	}
	expectedHost := strings.TrimPrefix(current.HTTPSOrigin, "https://")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != expectedHost {
			http.Error(w, "Host rejected", http.StatusMisdirectedRequest)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/__appcontrol_laptop/") {
			expected := base64.RawURLEncoding.EncodeToString(current.shutdownToken)
			if r.URL.Path != shutdownPath || r.Method != http.MethodPost || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-App-Control-Laptop-Token")), []byte(expected)) != 1 {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, "{\"stopping\":true}\n")
			go stop()
			return
		}
		if r.Method == http.MethodConnect {
			http.Error(w, "Method rejected", http.StatusMethodNotAllowed)
			return
		}
		proxy.ServeHTTP(w, r)
	}), nil
}
func serve(ctx context.Context, current *state, out io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	router, err := handler(current, cancel)
	if err != nil {
		return err
	}
	parsed, _ := url.Parse(current.HTTPSOrigin)
	port := parsed.Port()
	if port == "" {
		port = "443"
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", port))
	if err != nil {
		return err
	}
	server := &http.Server{Handler: router, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 * 1024, ErrorLog: log.New(io.Discard, "", 0), TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{current.pair}}}
	completed := make(chan error, 1)
	go func() { completed <- server.Serve(tls.NewListener(listener, server.TLSConfig)) }()
	fmt.Fprintln(out, "Laptop HTTPS proxy listening on loopback only. Wireless/cloud delivery is not configured.")
	select {
	case err := <-completed:
		return err
	case <-ctx.Done():
	}
	deadline, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	if err := server.Shutdown(deadline); err != nil {
		server.Close()
		return err
	}
	err = <-completed
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
