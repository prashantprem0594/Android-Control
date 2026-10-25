package serve

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"

	"appcontrol/server/internal/config"
)

func Run(
	ctx context.Context,
	listener net.Listener,
	handler http.Handler,
	logger *slog.Logger,
	settings config.Config,
) error {
	if ctx == nil || listener == nil || handler == nil {
		return fmt.Errorf("context, listener, and handler are required")
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: settings.ReadHeaderTimeout,
		ReadTimeout:       settings.ReadTimeout,
		WriteTimeout:      settings.WriteTimeout,
		IdleTimeout:       settings.IdleTimeout,
		MaxHeaderBytes:    settings.MaxHeaderBytes,
	}

	serveResult := make(chan error, 1)
	go func() {
		logger.Info("control server listening", "address", listener.Addr().String())
		serveResult <- server.Serve(listener)
	}()

	select {
	case err := <-serveResult:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
	}

	logger.Info("control server shutting down")
	shutdownContext, cancel := context.WithTimeout(context.Background(), settings.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownContext); err != nil {
		_ = server.Close()
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	err := <-serveResult
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve HTTP during shutdown: %w", err)
	}
	logger.Info("control server stopped")
	return nil
}
