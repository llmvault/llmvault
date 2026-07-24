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
	"github.com/usehivy/hivy/internal/microsandbox/config"
	"github.com/usehivy/hivy/internal/microsandbox/control"
	"github.com/usehivy/hivy/internal/microsandbox/db"
	"github.com/usehivy/hivy/internal/microsandbox/runner"
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
		fmt.Printf("microsandbox %s (%s)\n", version, commit)
		return
	}

	cfg := config.Load()
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
	case "runner":
		err = runRunner(ctx, cfg)
	default:
		slog.Error("unsupported microsandbox command", "command", cmd)
		os.Exit(1)
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

func runRunner(ctx context.Context, cfg config.Config) error {
	if cfg.RunnerLogIngestAddr == "" || cfg.RunnerLogIngestPublicURL == "" || cfg.RunnerLogIngestSigningKey == "" {
		return fmt.Errorf("HIVY_MICROSANDBOX_RUNNER_LOG_INGEST_ADDR, HIVY_MICROSANDBOX_RUNNER_LOG_INGEST_PUBLIC_URL, and HIVY_MICROSANDBOX_LOG_INGEST_SIGNING_KEY are required")
	}
	app, err := runner.NewServer(ctx, cfg)
	if err != nil {
		return err
	}
	goroutine.Go(ctx, func(ctx context.Context) {
		app.RegisterAndHeartbeat(ctx)
	})
	return serveMany(ctx,
		httpEndpoint{name: "runner API", addr: cfg.Addr, handler: app.Routes()},
		httpEndpoint{name: "sandbox log ingestion", addr: cfg.RunnerLogIngestAddr, handler: app.LogRoutes()},
	)
}

func serve(ctx context.Context, addr string, h http.Handler) error {
	return serveMany(ctx, httpEndpoint{name: "HTTP", addr: addr, handler: h})
}

type httpEndpoint struct {
	name    string
	addr    string
	handler http.Handler
}

func serveMany(ctx context.Context, endpoints ...httpEndpoint) error {
	if len(endpoints) == 0 {
		return fmt.Errorf("at least one HTTP endpoint is required")
	}
	servers := make([]*http.Server, 0, len(endpoints))
	errCh := make(chan error, len(endpoints))
	for _, endpoint := range endpoints {
		srv := &http.Server{
			Addr:              endpoint.addr,
			Handler:           endpoint.handler,
			ReadHeaderTimeout: 10 * time.Second,
		}
		servers = append(servers, srv)
		name := endpoint.name
		goroutine.Go(ctx, func(context.Context) {
			slog.Info("server listening", "service", name, "addr", srv.Addr)
			errCh <- srv.ListenAndServe()
		})
	}
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		var shutdownErr error
		for _, srv := range servers {
			if err := srv.Shutdown(shutdownCtx); err != nil {
				shutdownErr = errors.Join(shutdownErr, err)
			}
		}
		return shutdownErr
	case err := <-errCh:
		return err
	}
}
