package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/testdb"
)

// When a sandbox provider is configured but the orchestrator failed to
// initialize, /readyz must report 503 rather than letting the instance look
// healthy while the entire agent/gateway subsystem is silently missing.
func TestReadyzReportsOrchestratorMissing(t *testing.T) {
	dsn := testdb.DatabaseURL()
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("cannot connect to Postgres: %v", err)
	}
	sqlDB, _ := db.DB()
	if err := sqlDB.Ping(); err != nil {
		t.Skipf("cannot ping Postgres: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	addr := testdb.RedisAddr()
	rc := redis.NewClient(&redis.Options{Addr: addr})
	if err := rc.Ping(context.Background()).Err(); err != nil {
		t.Skipf("redis not reachable at %s: %v", addr, err)
	}
	t.Cleanup(func() { rc.Close() })

	t.Run("orchestrator missing -> 503", func(t *testing.T) {
		rec := httptest.NewRecorder()
		readyz(db, rc, true)(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", rec.Code)
		}
	})

	t.Run("orchestrator present -> 200", func(t *testing.T) {
		rec := httptest.NewRecorder()
		readyz(db, rc, false)(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
	})
}
