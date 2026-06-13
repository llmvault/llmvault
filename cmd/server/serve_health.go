package main

import (
	"net/http"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// orchestratorMissing reports whether sandbox orchestration was expected (a
// provider is configured) but failed to initialize. /readyz surfaces it so an
// instance can't report healthy while the agent runtime subsystem is missing.
func readyz(database *gorm.DB, rc *redis.Client, orchestratorMissing bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		sqlDB, err := database.DB()
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

		if err := rc.Ping(r.Context()).Err(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"error","detail":"redis ping failed"}`))
			return
		}

		if orchestratorMissing {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"error","detail":"sandbox orchestrator unavailable"}`))
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}
}
