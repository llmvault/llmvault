package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/getsentry/sentry-go"

	"github.com/usehivy/hivy/internal/goroutine"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/microsandbox/config"
	"github.com/usehivy/hivy/internal/microsandbox/control"
	"github.com/usehivy/hivy/internal/microsandbox/db"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cmd := "control"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	if cmd == "version" {
		fmt.Printf("msb %s (%s)\n", version, commit)
		return
	}

	cfg := config.Load()
	logging.Init(cfg.LogLevel, cfg.LogFormat)
	slog.Info("starting msb", "version", version, "commit", commit, "command", cmd)

	if cfg.SentryDSN != "" {
		if err := sentry.Init(sentry.ClientOptions{Dsn: cfg.SentryDSN, Environment: cfg.Environment}); err != nil {
			slog.Error("sentry init failed", "error", err)
			os.Exit(1)
		}
		defer sentry.Flush(2 * time.Second)
	}

	var err error
	switch cmd {
	case "control":
		err = runControl(ctx, cfg)
	case "migrate":
		err = runMigrate(ctx, cfg, os.Args[2:])
	default:
		err = fmt.Errorf("unsupported msb command %q (use: control, migrate, version)", cmd)
	}
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("service stopped with error", "error", err)
		os.Exit(1)
	}
}

func runControl(ctx context.Context, cfg config.Config) error {
	if cfg.PreviewPasswordKey == "" {
		return fmt.Errorf("HIVY_MICROSANDBOX_PREVIEW_PASSWORD_KEY is required")
	}
	if cfg.PreviewJWTSecret == "" {
		return fmt.Errorf("HIVY_MICROSANDBOX_PREVIEW_JWT_SECRET is required")
	}
	database, err := db.Open(ctx, cfg.DatabaseDSN, db.PoolConfig{
		MaxOpenConnections: cfg.DatabaseMaxOpenConns,
		MaxIdleConnections: cfg.DatabaseMaxIdleConns,
		ConnectionLifetime: cfg.DatabaseConnMaxLifetime,
	})
	if err != nil {
		return err
	}
	app := control.NewServer(ctx, database, cfg)
	return serve(ctx, cfg.Addr, app.Routes())
}

func serve(ctx context.Context, addr string, h http.Handler) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
	}
	errCh := make(chan error, 1)
	goroutine.Go(ctx, func(context.Context) {
		slog.Info("server listening", "addr", addr)
		errCh <- srv.ListenAndServe()
	})
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
