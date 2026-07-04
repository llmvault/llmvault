package main

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

type server struct {
	cfg     config
	log     *slog.Logger
	proc    processManager
	started time.Time

	// deployMu serializes deploy/rollback/env mutations so concurrent calls
	// cannot interleave symlink swaps and restarts.
	deployMu sync.Mutex
}

func newServer(cfg config, logger *slog.Logger, proc processManager) *server {
	return &server{cfg: cfg, log: logger, proc: proc, started: time.Now()}
}

func (s *server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /deploy", s.handleDeploy)
	mux.HandleFunc("POST /rollback", s.handleRollback)
	mux.HandleFunc("GET /logs", s.handleLogs)
	mux.HandleFunc("GET /health", s.handleHealth)
	// Alias: a stray /healthz probe (e.g. an agent-runtime style health check
	// pointed at appd) resolves to the same liveness handler as /health.
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("POST /env", s.handleEnv)
	return s.requireAuth(mux)
}

// requireAuth enforces bearer auth with the app secret on every endpoint,
// using a constant-time comparison (same idiom as the platform's authAgent).
func (s *server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bearer := bearerFromHeader(r.Header.Get("Authorization"))
		if bearer == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization"})
			return
		}
		if subtle.ConstantTimeCompare([]byte(bearer), []byte(s.cfg.Secret)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid app secret"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bearerFromHeader(header string) string {
	const prefix = "Bearer "
	if len(header) > len(prefix) && strings.EqualFold(header[:len(prefix)], prefix) {
		return strings.TrimSpace(header[len(prefix):])
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return false
	}
	return true
}
