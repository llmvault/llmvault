package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/usehivy/hivy/internal/goroutine"
	obsmetrics "github.com/usehivy/hivy/internal/observability/metrics"
	sentryobs "github.com/usehivy/hivy/internal/observability/sentry"
)

func startServeMetricsServer(ctx context.Context, port int) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", obsmetrics.Handler())

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           mux,
		ErrorLog:          sentryobs.NewStdlogBridge("api_metrics_server"),
		ReadHeaderTimeout: 5 * time.Second,
	}
	goroutine.Go(ctx, func(context.Context) {
		slog.Info("api metrics server starting", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("api metrics server error", "error", err)
		}
	})
	return srv
}
