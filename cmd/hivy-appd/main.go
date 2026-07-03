// Command hivy-appd is the deploy/observe daemon that runs inside every app
// sandbox. It is independent of the app process itself (survives app crashes)
// and exposes deploy, rollback, logs, health, and env endpoints, all
// authenticated with the per-app secret.
//
// It works under both sandbox providers:
//   - microsandbox: systemd is PID1; appd runs as hivy-appd.service and drives
//     the app through systemctl (hivy-app.service).
//   - docker: the image entrypoint runs appd directly (no systemd boot, same
//     pattern as Dockerfile.runtime); appd supervises the app process itself.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/usehivy/hivy/internal/goroutine"
)

func main() {
	if err := run(); err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("hivy-appd failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if err := ensureDirs(cfg); err != nil {
		return err
	}

	appdLog, err := os.OpenFile(cfg.appdLogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open appd log: %w", err)
	}
	defer appdLog.Close()
	logger := slog.New(slog.NewJSONHandler(io.MultiWriter(os.Stdout, appdLog), nil))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	proc := detectProcessManager(cfg, logger)
	logger.InfoContext(ctx, "hivy-appd starting",
		"port", cfg.Port,
		"app_root", cfg.AppRoot,
		"logs_dir", cfg.LogsDir,
		"process_manager", proc.Mode(),
	)
	if err := proc.Start(ctx); err != nil {
		logger.WarnContext(ctx, "initial app start failed", "error", err)
	}

	goroutine.Go(ctx, func(ctx context.Context) {
		rotationLoop(ctx, logger, []string{cfg.appLogPath(), cfg.appdLogPath()})
	})

	srv := newServer(cfg, logger, proc)
	httpServer := &http.Server{
		Addr:              fmt.Sprintf("0.0.0.0:%d", cfg.Port),
		Handler:           srv.handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	goroutine.Go(ctx, func(context.Context) {
		errCh <- httpServer.ListenAndServe()
	})

	select {
	case <-ctx.Done():
		logger.InfoContext(ctx, "hivy-appd shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.WarnContext(ctx, "http shutdown", "error", err)
		}
		proc.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("http server: %w", err)
	}
}

func ensureDirs(cfg config) error {
	for _, dir := range []string{cfg.AppRoot, cfg.releasesDir(), cfg.tmpDir(), cfg.LogsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	return nil
}

// rotationLoop periodically size-rotates the given log files. appd owns
// rotation for both its own log and the app's log (which the app or systemd
// appends to); copy-truncate keeps open file descriptors valid.
func rotationLoop(ctx context.Context, logger *slog.Logger, paths []string) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, path := range paths {
				rotated, err := rotateIfNeeded(path, rotateAtBytes, rotateKeep)
				if err != nil {
					logger.WarnContext(ctx, "log rotation failed", "path", path, "error", err)
				} else if rotated {
					logger.InfoContext(ctx, "log rotated", "path", path)
				}
			}
		}
	}
}
