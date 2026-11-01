package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
<<<<<<< HEAD
	"time"
=======
>>>>>>> origin/main

	"appcontrol/server/internal/audit"
	"appcontrol/server/internal/auth"
	"appcontrol/server/internal/config"
	"appcontrol/server/internal/control"
	"appcontrol/server/internal/httpapi"
	"appcontrol/server/internal/pairing"
<<<<<<< HEAD
	"appcontrol/server/internal/parentpush"
	"appcontrol/server/internal/serve"
	"appcontrol/server/internal/storage"
	"sync"
=======
	"appcontrol/server/internal/serve"
	"appcontrol/server/internal/storage"
>>>>>>> origin/main
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
<<<<<<< HEAD
		logServerFailure(logger, err)
=======
		logger.Error("control server stopped with error", "error", err)
>>>>>>> origin/main
		os.Exit(1)
	}
}

<<<<<<< HEAD
func logServerFailure(logger *slog.Logger, _ error) {
	logger.Error("control server stopped with error", "failure", "internal")
}

=======
>>>>>>> origin/main
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

<<<<<<< HEAD
	recoveryKey, err := loadRecoveryHMACKey(settings.RecoveryHMACKeyPath)
	if err != nil {
		_ = listener.Close()
		return fmt.Errorf("initialize recovery protection: %w", err)
	}
	defer clear(recoveryKey)
	pairingService, err := pairing.NewService(database, pairing.Options{RecoveryHMACKey: recoveryKey})
=======
	pairingService, err := pairing.NewService(database, pairing.Options{})
>>>>>>> origin/main
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
<<<<<<< HEAD
	parentPushStore, err := configureParentPush(database, settings)
	if err != nil {
		_ = listener.Close()
		return fmt.Errorf("initialize parent push registry: %w", err)
	}
	parentPushSender, err := configureParentPushSender(settings)
	if err != nil {
		_ = listener.Close()
		return fmt.Errorf("initialize parent push sender: %w", err)
	}
	var parentPushWorker *parentpush.Worker
	if parentPushSender != nil {
		parentPushWorker, err = parentpush.NewWorker(parentPushStore, parentPushSender, logger)
		if err != nil {
			_ = listener.Close()
			return fmt.Errorf("initialize parent push worker: %w", err)
		}
	}
=======
>>>>>>> origin/main
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
<<<<<<< HEAD
		Release:            releaseDocument,
		WakeupTokens:       tokenStore,
		ParentWakeupTokens: parentPushStore,
		Guard:              guard,
		Control:            controlService,
=======
		Release:      releaseDocument,
		WakeupTokens: tokenStore,
		Guard:        guard,
		Control:      controlService,
>>>>>>> origin/main
		Audit: func(ctx context.Context, parentID string, after int64, limit int) ([]audit.Event, error) {
			return audit.List(ctx, database, parentID, after, limit)
		},
		Logger:  logger,
		Ready:   database.PingContext,
		Auth:    authService,
		Pairing: pairingService,
	})
<<<<<<< HEAD
	workerContext, cancelWorkers := context.WithCancel(shutdownContext)
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		_ = parentpush.RunMaintenance(workerContext, database, logger)
	}()
	workers.Add(1)
	go func() {
		defer workers.Done()
		// Retention is enforced on reads/writes as well, but this worker keeps the
		// database clear even while no device or parent is connected.
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			if err := controlService.PruneVisibleLocations(workerContext); err != nil && workerContext.Err() == nil {
				logger.Error("visible location retention failed", "failure", "internal")
			}
			select {
			case <-workerContext.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	if parentPushWorker != nil {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if err := parentPushWorker.Run(workerContext); err != nil && workerContext.Err() == nil {
				logger.Error("parent push worker stopped", "failure", "internal")
			}
		}()
	}
	serveErr := serve.Run(shutdownContext, listener, handler, logger, settings)
	cancelWorkers()
	workers.Wait()
	return serveErr
}

func loadRecoveryHMACKey(path string) ([]byte, error) {
	if path == "" {
		return nil, nil
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("recovery key is unavailable")
	}
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) != 32 {
		return nil, fmt.Errorf("recovery key is unavailable")
	}
	return raw, nil
=======
	return serve.Run(shutdownContext, listener, handler, logger, settings)
>>>>>>> origin/main
}
