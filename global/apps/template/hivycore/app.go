// DO NOT EDIT — hivycore is the Hivy platform core. See doc.go.

package hivycore

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// App is the assembled platform core. Agent code gets one from MustNew (in
// main), registers handlers with HandleAPI, and calls Run.
type App struct {
	cfg    *Config
	log    *slog.Logger
	codec  *sessionCodec
	jti    *jtiGuard
	sheets *SheetsClient
	live   *liveManager
	api    *http.ServeMux
	// frameAncestors is the Content-Security-Policy for every HTML response,
	// derived once from the launch URL (see frameAncestorsFrom).
	frameAncestors string
}

// New loads configuration from the environment and assembles the core.
func New() (*App, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	return newApp(cfg)
}

// MustNew is New with fail-fast startup: configuration errors are logged as
// JSON and the process exits non-zero so systemd surfaces the failure.
func MustNew() *App {
	app, err := New()
	if err != nil {
		newLogger().Error("hivycore startup failed", "error", err.Error())
		os.Exit(1)
	}
	return app
}

func newApp(cfg *Config) (*App, error) {
	codec, err := newSessionCodec(cfg.SessionSecret)
	if err != nil {
		return nil, err
	}
	log := newLogger()
	app := &App{
		cfg:            cfg,
		log:            log,
		codec:          codec,
		jti:            newJTIGuard(jtiRetention, jtiCapacity),
		sheets:         newSheetsClient(cfg.APIBaseURL, cfg.AppSecret),
		live:           newLiveManager(cfg.APIBaseURL, cfg.AppSecret, log),
		api:            http.NewServeMux(),
		frameAncestors: frameAncestorsFrom(cfg.LaunchURL),
	}
	// The realtime relay is mounted inside the authed /api router, so the
	// session cookie gates it just like every other app endpoint. Agent code
	// never sees it — the SPA's realtime engine is the only client.
	app.api.HandleFunc("GET /_live", app.handleLive)
	return app, nil
}

// HandleAPI registers an agent handler under /api with authentication
// pre-applied. Patterns are Go 1.22 ServeMux patterns relative to /api:
// HandleAPI("GET /me", h) serves GET /api/me. Handlers can rely on
// UserFrom(r.Context()) returning the session.
func (a *App) HandleAPI(pattern string, handler http.HandlerFunc) {
	a.api.HandleFunc(pattern, handler)
}

// Sheets is the typed client for the app's one bound sheet.
func (a *App) Sheets() *SheetsClient { return a.sheets }

// SheetID is the bound sheet's ID (informational; the API is already scoped).
func (a *App) SheetID() string { return a.cfg.SheetID }

// LaunchURL is the Hivy frontend launch endpoint for this app.
func (a *App) LaunchURL() string { return a.cfg.LaunchURL }

// Log is the app's structured JSON logger (stdout → platform log capture).
func (a *App) Log() *slog.Logger { return a.log }

// Handler assembles the full HTTP surface: health, auth callback, the authed
// /api router, and the session-gated static SPA.
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.handleHealthz)
	mux.HandleFunc("GET /auth/callback", a.handleAuthCallback)
	mux.Handle("/api/", a.requireAPISession(http.StripPrefix("/api", a.api)))
	mux.Handle("/", a.pageGate(a.staticHandler()))
	return a.requestLogger(mux)
}

// Run serves until SIGTERM/SIGINT, then shuts down gracefully. It logs its
// own errors; main only needs to exit non-zero when it returns one.
func (a *App) Run() error {
	server := &http.Server{
		Addr:              ":" + a.cfg.Port,
		Handler:           a.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServe() }()

	a.log.Info("hivy app listening",
		"port", a.cfg.Port,
		"app_id", a.cfg.AppID,
		"template_version", TemplateVersion,
	)

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.log.Error("server failed", "error", err.Error())
			return err
		}
		return nil
	case <-ctx.Done():
		// Tear the live relay down first so in-flight SSE writers return and
		// their long-lived connections don't stall graceful shutdown.
		a.live.Close()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			a.log.Error("shutdown failed", "error", err.Error())
			return err
		}
		a.log.Info("shutdown complete")
		return nil
	}
}

// handleHealthz is unauthenticated liveness for the platform's health probe.
func (a *App) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]string{
		"status":           "ok",
		"template_version": TemplateVersion,
	})
}

// WriteJSON writes a JSON response with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError maps a handler error onto an HTTP response: an *APIError from
// the sheets client relays the platform's status and message (so the SPA
// sees validation failures verbatim); anything else logs and returns an
// opaque 500.
func (a *App) WriteError(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		status := apiErr.Status
		if status < 400 || status > 599 {
			status = http.StatusBadGateway
		}
		WriteJSON(w, status, map[string]string{"error": apiErr.Message})
		return
	}
	a.log.Error("handler error",
		"method", r.Method,
		"path", r.URL.Path,
		"error", err.Error(),
	)
	WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
}
