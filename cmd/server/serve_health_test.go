package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/testdb"
)

// TestReadyzReportsOrchestratorMissing is the P1-20 regression guard: when a
// sandbox provider is configured but the orchestrator failed to initialize,
// /readyz must report 503 rather than letting the instance look healthy while
// the entire employee/gateway subsystem is silently missing.
func TestReadyzReportsOrchestratorMissing(t *testing.T) {
	db, err := gorm.Open(postgres.Open(testdb.DatabaseURL()), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })

	rc := redis.NewClient(&redis.Options{Addr: testdb.RedisAddr()})
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
