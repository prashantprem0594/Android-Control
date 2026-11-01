package config

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

const defaultMaxHeaderBytes = 1 << 20

type LookupEnv func(string) (string, bool)

type Config struct {
	ReleaseManifestPath            string
	ReleasePublicKeyPath           string
	ReleaseOrigin                  string
	ReleaseCertificate             string
	ReleaseMinimumSequence         int
	RemoteCallsEnabled             bool
	WakeupServiceAccountPath       string
	WakeupTokenKeyPath             string
	ParentWakeupTokenKeyPath       string
	ParentWakeupServiceAccountPath string
	ParentWakeupSenderEnabled      bool
	RecoveryHMACKeyPath            string
	HTTPAddress                    string
	DatabasePath                   string
	ReadHeaderTimeout              time.Duration
	ReadTimeout                    time.Duration
	WriteTimeout                   time.Duration
	IdleTimeout                    time.Duration
	ShutdownTimeout                time.Duration
	MaxHeaderBytes                 int
	RequestTimeout                 time.Duration
	MaxInFlight                    int
	MaxPasswordInFlight            int
	MaxRatePeers                   int
	RateScale                      int
	TrustedProxies                 []netip.Prefix
}

func Load(lookup LookupEnv) (Config, error) {
	if lookup == nil {
		return Config{}, fmt.Errorf("environment lookup is required")
	}

	result := Config{
		ReleaseManifestPath:            strings.TrimSpace(valueOrDefault(lookup, "APP_CONTROL_RELEASE_MANIFEST_FILE", "")),
		ReleasePublicKeyPath:           strings.TrimSpace(valueOrDefault(lookup, "APP_CONTROL_RELEASE_PUBLIC_KEY_FILE", "")),
		ReleaseOrigin:                  strings.TrimSpace(valueOrDefault(lookup, "APP_CONTROL_RELEASE_ORIGIN", "")),
		ReleaseCertificate:             strings.TrimSpace(valueOrDefault(lookup, "APP_CONTROL_RELEASE_CERTIFICATE_SHA256", "")),
		WakeupServiceAccountPath:       strings.TrimSpace(valueOrDefault(lookup, "APP_CONTROL_FCM_SERVICE_ACCOUNT_FILE", "")),
		WakeupTokenKeyPath:             strings.TrimSpace(valueOrDefault(lookup, "APP_CONTROL_FCM_TOKEN_KEY_FILE", "")),
		ParentWakeupTokenKeyPath:       strings.TrimSpace(valueOrDefault(lookup, "APP_CONTROL_PARENT_FCM_TOKEN_KEY_FILE", "")),
		ParentWakeupServiceAccountPath: strings.TrimSpace(valueOrDefault(lookup, "APP_CONTROL_PARENT_FCM_SERVICE_ACCOUNT_FILE", "")),
		RecoveryHMACKeyPath:            strings.TrimSpace(valueOrDefault(lookup, "APP_CONTROL_RECOVERY_HMAC_KEY_FILE", "")),
		HTTPAddress:                    valueOrDefault(lookup, "APP_CONTROL_HTTP_ADDRESS", ":8080"),
		DatabasePath:                   valueOrDefault(lookup, "APP_CONTROL_DATABASE_PATH", "data/app-control.db"),
		ReadHeaderTimeout:              5 * time.Second,
		ReadTimeout:                    15 * time.Second,
		WriteTimeout:                   15 * time.Second,
		IdleTimeout:                    60 * time.Second,
		ShutdownTimeout:                10 * time.Second,
		MaxHeaderBytes:                 defaultMaxHeaderBytes,
		RequestTimeout:                 12 * time.Second,
		MaxInFlight:                    128,
		MaxPasswordInFlight:            2,
		MaxRatePeers:                   4096,
		RateScale:                      1,
	}

	var err error
	remoteCalls := valueOrDefault(lookup, "APP_CONTROL_REMOTE_CALLS_ENABLED", "false")
	if remoteCalls != "true" && remoteCalls != "false" {
		return Config{}, fmt.Errorf("APP_CONTROL_REMOTE_CALLS_ENABLED must be true or false")
	}
	result.RemoteCallsEnabled = remoteCalls == "true"
	parentSender := valueOrDefault(lookup, "APP_CONTROL_PARENT_FCM_SENDER_ENABLED", "false")
	if parentSender != "true" && parentSender != "false" {
		return Config{}, fmt.Errorf("APP_CONTROL_PARENT_FCM_SENDER_ENABLED must be true or false")
	}
	result.ParentWakeupSenderEnabled = parentSender == "true"
	if result.ParentWakeupSenderEnabled &&
		(result.ParentWakeupTokenKeyPath == "" || result.ParentWakeupServiceAccountPath == "") {
		return Config{}, fmt.Errorf("parent FCM sender requires parent token key and parent service account files")
	}
	if result.ReleaseMinimumSequence, err = strconv.Atoi(valueOrDefault(lookup, "APP_CONTROL_RELEASE_MIN_SEQUENCE", "0")); err != nil || result.ReleaseMinimumSequence < 0 || result.ReleaseMinimumSequence > 2100000000 {
		return Config{}, fmt.Errorf("invalid release minimum sequence")
	}
	releaseFields := 0
	for _, value := range []string{result.ReleaseManifestPath, result.ReleasePublicKeyPath, result.ReleaseOrigin, result.ReleaseCertificate} {
		if value != "" {
			releaseFields++
		}
	}
	if releaseFields != 0 && releaseFields != 4 || releaseFields == 0 && result.ReleaseMinimumSequence != 0 {
		return Config{}, fmt.Errorf("release hosting requires manifest, public key, origin and APK certificate settings together")
	}
	if (result.WakeupServiceAccountPath == "") != (result.WakeupTokenKeyPath == "") {
		return Config{}, fmt.Errorf("FCM requires both service account and routing-token key files")
	}
	if result.ReadHeaderTimeout, err = duration(lookup, "APP_CONTROL_READ_HEADER_TIMEOUT", result.ReadHeaderTimeout); err != nil {
		return Config{}, err
	}
	if result.ReadTimeout, err = duration(lookup, "APP_CONTROL_READ_TIMEOUT", result.ReadTimeout); err != nil {
		return Config{}, err
	}
	if result.WriteTimeout, err = duration(lookup, "APP_CONTROL_WRITE_TIMEOUT", result.WriteTimeout); err != nil {
		return Config{}, err
	}
	if result.IdleTimeout, err = duration(lookup, "APP_CONTROL_IDLE_TIMEOUT", result.IdleTimeout); err != nil {
		return Config{}, err
	}
	if result.ShutdownTimeout, err = duration(lookup, "APP_CONTROL_SHUTDOWN_TIMEOUT", result.ShutdownTimeout); err != nil {
		return Config{}, err
	}
	if result.MaxHeaderBytes, err = positiveInt(lookup, "APP_CONTROL_MAX_HEADER_BYTES", result.MaxHeaderBytes); err != nil {
		return Config{}, err
	}
	if result.RequestTimeout, err = duration(lookup, "APP_CONTROL_REQUEST_TIMEOUT", result.RequestTimeout); err != nil {
		return Config{}, err
	}
	for _, setting := range []struct {
		name   string
		target *int
	}{{"APP_CONTROL_MAX_IN_FLIGHT", &result.MaxInFlight}, {"APP_CONTROL_MAX_PASSWORD_IN_FLIGHT", &result.MaxPasswordInFlight}, {"APP_CONTROL_MAX_RATE_PEERS", &result.MaxRatePeers}, {"APP_CONTROL_RATE_SCALE", &result.RateScale}} {
		if *setting.target, err = positiveInt(lookup, setting.name, *setting.target); err != nil {
			return Config{}, err
		}
	}
	if result.RequestTimeout > 2*time.Minute || result.MaxInFlight > 1024 || result.MaxPasswordInFlight > 16 || result.MaxPasswordInFlight > result.MaxInFlight || result.MaxRatePeers > 65536 || result.RateScale > 100 {
		return Config{}, fmt.Errorf("request protection setting exceeds supported bounds")
	}
	proxyText := strings.TrimSpace(valueOrDefault(lookup, "APP_CONTROL_TRUSTED_PROXY_CIDRS", ""))
	if proxyText != "" {
		parts := strings.Split(proxyText, ",")
		if len(parts) > 32 {
			return Config{}, fmt.Errorf("APP_CONTROL_TRUSTED_PROXY_CIDRS accepts at most 32 prefixes")
		}
		for _, part := range parts {
			prefix, err := netip.ParsePrefix(strings.TrimSpace(part))
			if err != nil || prefix.Bits() == 0 {
				return Config{}, fmt.Errorf("APP_CONTROL_TRUSTED_PROXY_CIDRS requires specific IP prefixes")
			}
			result.TrustedProxies = append(result.TrustedProxies, prefix.Masked())
		}
	}

	if _, _, err := net.SplitHostPort(result.HTTPAddress); err != nil {
		return Config{}, fmt.Errorf("APP_CONTROL_HTTP_ADDRESS: %w", err)
	}
	if strings.TrimSpace(result.DatabasePath) == "" {
		return Config{}, fmt.Errorf("APP_CONTROL_DATABASE_PATH must not be empty")
	}

	return result, nil
}

func valueOrDefault(lookup LookupEnv, name, fallback string) string {
	if value, ok := lookup(name); ok && value != "" {
		return value
	}
	return fallback
}

func duration(lookup LookupEnv, name string, fallback time.Duration) (time.Duration, error) {
	value := valueOrDefault(lookup, name, fallback.String())
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration: %w", name, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", name)
	}
	return parsed, nil
}

func positiveInt(lookup LookupEnv, name string, fallback int) (int, error) {
	value := valueOrDefault(lookup, name, strconv.Itoa(fallback))
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsed, nil
}
