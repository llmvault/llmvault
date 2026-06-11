package sentry

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"sync/atomic"
	"time"

	sentrygo "github.com/getsentry/sentry-go"

	"github.com/usehivy/hivy/internal/config"
)

// sensitiveQueryParams lists URL query parameter names whose values must be scrubbed from Sentry
// events to prevent tokens from appearing in breadcrumbs or request data.
var sensitiveQueryParams = []string{"token", "key"}

// scrubQueryString replaces sensitive token values in a query string with "[Filtered]".
func scrubQueryString(qs string) string {
	if qs == "" {
		return qs
	}
	q, err := url.ParseQuery(qs)
	if err != nil {
		return qs
	}
	changed := false
	for _, param := range sensitiveQueryParams {
		if q.Has(param) {
			q.Set(param, "[Filtered]")
			changed = true
		}
	}
	if !changed {
		return qs
	}
	return q.Encode()
}

// scrubSensitiveQueryParams replaces sensitive token values in a URL with [Filtered].
func scrubSensitiveQueryParams(rawURL string) string {
	if rawURL == "" {
		return rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.RawQuery == "" {
		return rawURL
	}
	scrubbed := scrubQueryString(u.RawQuery)
	if scrubbed == u.RawQuery {
		return rawURL
	}
	u.RawQuery = scrubbed
	return u.String()
}

// beforeSend scrubs sensitive query params from request URLs before transmission.
func beforeSend(event *sentrygo.Event, _ *sentrygo.EventHint) *sentrygo.Event {
	if event == nil {
		return event
	}
	if event.Request != nil {
		// URL is a full URL string.
		event.Request.URL = scrubSensitiveQueryParams(event.Request.URL)
		// QueryString is a bare query string (no leading "?").
		event.Request.QueryString = scrubQueryString(event.Request.QueryString)
	}
	return event
}

type ClientOptions struct {
	ServiceName string
	Environment string
	Hostname    string
	Release     string
}

var initialized atomic.Bool

func Init(cfg *config.Config, opts ClientOptions) error {
	if cfg == nil {
		return nil
	}
	if cfg.SentryDSN != "" && !cfg.SentryEnabled {
		slog.Warn("HIVY_SENTRY_DSN is set but HIVY_SENTRY_ENABLED is false — Sentry will not capture events; set HIVY_SENTRY_ENABLED=true to enable")
	}
	if !cfg.SentryEnabled || cfg.SentryDSN == "" {
		return nil
	}

	hostname := opts.Hostname
	if hostname == "" {
		if h, err := os.Hostname(); err == nil {
			hostname = h
		}
	}

	environment := opts.Environment
	if environment == "" {
		environment = cfg.Environment
	}

	release := opts.Release
	if release == "" {
		release = cfg.SentryRelease
	}

	options := sentryClientOptions(cfg, environment, release, hostname)
	if err := sentrygo.Init(options); err != nil {
		return fmt.Errorf("sentry: initialize client: %w", err)
	}

	sentrygo.ConfigureScope(func(scope *sentrygo.Scope) {
		if opts.ServiceName != "" {
			scope.SetTag("service", opts.ServiceName)
		}
		if hostname != "" {
			scope.SetTag("hostname", hostname)
		}
	})

	initialized.Store(true)
	slog.Info("sentry initialized",
		"environment", environment,
		"release", release,
		"tracing_enabled", options.EnableTracing,
		"traces_sample_rate", options.TracesSampleRate,
		"service", opts.ServiceName,
	)
	return nil
}

func sentryClientOptions(cfg *config.Config, environment, release, hostname string) sentrygo.ClientOptions {
	enableTracing := cfg.SentryTracesSampleRate > 0
	return sentrygo.ClientOptions{
		Dsn:                  cfg.SentryDSN,
		Environment:          environment,
		Release:              release,
		EnableTracing:        enableTracing,
		TracesSampleRate:     cfg.SentryTracesSampleRate,
		AttachStacktrace:     true,
		ServerName:           hostname,
		DisableClientReports: true,
		BeforeSend:           beforeSend,
	}
}

func Enabled() bool { return initialized.Load() }

func Flush(timeout time.Duration) bool {
	if !Enabled() {
		return true
	}
	return sentrygo.Flush(timeout)
}

func Close() {
	if !Enabled() {
		return
	}
	if !sentrygo.Flush(5 * time.Second) {
		slog.Warn("sentry: flush timed out before shutdown")
	}
}

func CaptureException(ctx context.Context, err error) {
	if !Enabled() || err == nil {
		return
	}
	hubFromContext(ctx).CaptureException(err)
}

func CaptureExceptionWithFields(ctx context.Context, err error, fields map[string]any) {
	if !Enabled() || err == nil {
		return
	}
	hub := hubFromContext(ctx)
	hub.WithScope(func(scope *sentrygo.Scope) {
		for key, value := range fields {
			switch typed := value.(type) {
			case string:
				if typed != "" {
					scope.SetTag(key, typed)
				}
			case fmt.Stringer:
				scope.SetTag(key, typed.String())
			default:
				scope.SetContext(key, sentrygo.Context{"value": value})
			}
		}
		hub.CaptureException(err)
	})
}

func CaptureMessage(ctx context.Context, message string) {
	if !Enabled() || message == "" {
		return
	}
	hubFromContext(ctx).CaptureMessage(message)
}

func hubFromContext(ctx context.Context) *sentrygo.Hub {
	if ctx != nil {
		if hub := sentrygo.GetHubFromContext(ctx); hub != nil {
			return hub
		}
	}
	return sentrygo.CurrentHub()
}
