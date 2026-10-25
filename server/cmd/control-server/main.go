package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"appcontrol/server/internal/audit"
	"appcontrol/server/internal/auth"
	"appcontrol/server/internal/config"
	"appcontrol/server/internal/control"
	"appcontrol/server/internal/httpapi"
	"appcontrol/server/internal/pairing"
	"appcontrol/server/internal/serve"
	"appcontrol/server/internal/storage"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("control server stopped with error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	settings, err := config.Load(os.LookupEnv)
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	listener, err := net.Listen("tcp", settings.HTTPAddress)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", settings.HTTPAddress, err)
	}
	database, err := storage.Open(context.Background(), settings.DatabasePath)
	if err != nil {
		_ = listener.Close()
		return fmt.Errorf("open storage: %w", err)
	}
	defer database.Close()
	authService, err := auth.NewService(database, auth.Options{})
	if err != nil {
		_ = listener.Close()
		return fmt.Errorf("initialize authentication: %w", err)
	}

	pairingService, err := pairing.NewService(database, pairing.Options{})
	if err != nil {
		_ = listener.Close()
		return fmt.Errorf("initialize pairing: %w", err)
	}

	shutdownContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	provider, tokenStore, err := configureWakeup(database, settings)
	if err != nil {
		_ = listener.Close()
		return fmt.Errorf("initialize wake-up provider: %w", err)
	}
	releaseDocument, err := configureRelease(settings)
	if err != nil {
		_ = listener.Close()
		return fmt.Errorf("initialize release metadata: %w", err)
	}
	guard, err := httpapi.NewRequestGuard(httpapi.GuardConfig{TrustedProxies: settings.TrustedProxies,
		RequestTimeout: settings.RequestTimeout, MaxInFlight: settings.MaxInFlight, MaxPasswordInFlight: settings.MaxPasswordInFlight,
		MaxPeers: settings.MaxRatePeers, RateScale: settings.RateScale})
	if err != nil {
		_ = listener.Close()
		return fmt.Errorf("initialize request protection: %w", err)
	}

	controlService := control.NewServiceWithWakeup(database, provider)
	controlService.SetRemoteCallsEnabled(settings.RemoteCallsEnabled)
	handler := httpapi.NewHandler(httpapi.Options{
		Release:      releaseDocument,
		WakeupTokens: tokenStore,
		Guard:        guard,
		Control:      controlService,
		Audit: func(ctx context.Context, parentID string, after int64, limit int) ([]audit.Event, error) {
			return audit.List(ctx, database, parentID, after, limit)
		},
		Logger:  logger,
		Ready:   database.PingContext,
		Auth:    authService,
		Pairing: pairingService,
	})
	return serve.Run(shutdownContext, listener, handler, logger, settings)
}
