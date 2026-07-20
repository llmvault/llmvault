package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/usehivy/hivy/internal/bootstrap"
	"github.com/usehivy/hivy/internal/goroutine"
	sentryobs "github.com/usehivy/hivy/internal/observability/sentry"
)

func startWorkerHealthServer(ctx context.Context, deps *bootstrap.Deps) (*http.Server, error) {
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"worker"}`))
	})
	healthMux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		sqlDB, err := deps.DB.DB()
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"error","detail":"db connection failed"}`))
			return
		}
		if err := sqlDB.Ping(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"error","detail":"db ping failed"}`))
			return
		}
		if err := deps.Redis.Ping(r.Context()).Err(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"error","detail":"redis ping failed"}`))
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"worker"}`))
	})

	healthPort := deps.Config.WorkerHealthPort
	if port := os.Getenv("PORT"); port != "" {
		parsed, err := strconv.Atoi(port)
		if err != nil {
			return nil, fmt.Errorf("parsing PORT for worker health server: %w", err)
		}
		healthPort = parsed
	}

	healthSrv := &http.Server{
		Addr:              fmt.Sprintf(":%d", healthPort),
		Handler:           healthMux,
		ErrorLog:          sentryobs.NewStdlogBridge("worker_health_server"),
		ReadHeaderTimeout: 5 * time.Second,
	}
	goroutine.Go(ctx, func(context.Context) {
		slog.Info("worker health server starting", "port", healthPort)
		if err := healthSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("worker health server error", "error", err)
		}
	})
	return healthSrv, nil
}
