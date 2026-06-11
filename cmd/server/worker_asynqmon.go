package main

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/hibiken/asynqmon"

	"github.com/usehivy/hivy/internal/config"
	sentryobs "github.com/usehivy/hivy/internal/observability/sentry"
)

// buildAsynqmonServer returns an http.Server hosting the asynqmon dashboard
// behind HTTP basic auth on its own port, or nil if the dashboard is disabled
// or misconfigured. The dashboard is fail-closed: it requires both
// HIVY_ASYNQMON_ENABLED=true and basic-auth credentials, and it must not share
// the public health port.
func buildAsynqmonServer(ctx context.Context, cfg *config.Config, redisOpt asynq.RedisConnOpt) *http.Server {
	if !cfg.AsynqmonEnabled {
		slog.Info("asynqmon dashboard disabled (set HIVY_ASYNQMON_ENABLED=true with basic-auth credentials to enable)")
		return nil
	}
	user := strings.TrimSpace(cfg.AsynqmonUser)
	password := cfg.AsynqmonPassword
	if user == "" || password == "" {
		slog.Error("asynqmon dashboard enabled but HIVY_ASYNQMON_USER/HIVY_ASYNQMON_PASSWORD not set — refusing to expose it unauthenticated")
		return nil
	}
	dashboardPort := cfg.AsynqmonPort
	if dashboardPort == cfg.WorkerHealthPort {
		slog.Error("asynqmon dashboard port must differ from the worker health port — refusing to mount on the public health port",
			"asynqmon_port", dashboardPort, "health_port", cfg.WorkerHealthPort)
		return nil
	}
	if port := os.Getenv("PORT"); port != "" {
		if parsed, err := strconv.Atoi(port); err == nil && parsed == dashboardPort {
			slog.Error("asynqmon dashboard port collides with the platform-published PORT — refusing to expose it publicly",
				"asynqmon_port", dashboardPort, "platform_port", parsed)
			return nil
		}
	}

	dashboard := asynqmon.New(asynqmon.Options{
		RootPath:     "/asynq",
		RedisConnOpt: redisOpt,
		ReadOnly:     true,
	})
	mux := http.NewServeMux()
	mux.Handle("/asynq/", basicAuth(user, password, dashboard))
	mux.Handle("/asynq", basicAuth(user, password, dashboard))

	slog.Info("asynqmon dashboard enabled (basic-auth)", "port", dashboardPort, "path", "/asynq")
	return &http.Server{
		Addr:              fmt.Sprintf(":%d", dashboardPort),
		Handler:           mux,
		ErrorLog:          sentryobs.NewStdlogBridge("asynqmon_dashboard"),
		ReadHeaderTimeout: 5 * time.Second,
	}
}

// basicAuth wraps a handler with constant-time HTTP basic-auth verification.
func basicAuth(user, password string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		userOK := subtle.ConstantTimeCompare([]byte(u), []byte(user)) == 1
		passOK := subtle.ConstantTimeCompare([]byte(p), []byte(password)) == 1
		if !ok || !userOK || !passOK {
			w.Header().Set("WWW-Authenticate", `Basic realm="asynqmon"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// asynqLogger adapts slog to asynq's Logger interface.
type asynqLogger struct{}

func newAsynqLogger() *asynqLogger { return &asynqLogger{} }

func (l *asynqLogger) Debug(args ...any) {
	slog.Debug(fmt.Sprint(args...))
}

func (l *asynqLogger) Info(args ...any) {
	slog.Info(fmt.Sprint(args...))
}

func (l *asynqLogger) Warn(args ...any) {
	slog.Warn(fmt.Sprint(args...))
}

func (l *asynqLogger) Error(args ...any) {
	slog.Error(fmt.Sprint(args...))
}

func (l *asynqLogger) Fatal(args ...any) {
	slog.Error(fmt.Sprint(args...))
}
