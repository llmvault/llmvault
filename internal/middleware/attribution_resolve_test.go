package middleware

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func closeUnderlyingDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close sql db: %v", err)
	}
}

func TestExtractAttributionCachesAndSkipsDBOnSecondCall(t *testing.T) {
	db := generationTestDB(t)
	sandboxID := uuid.New()
	sessionID := uuid.New()
	if err := db.Exec(`INSERT INTO tokens (id, jti, meta) VALUES (?, ?, ?)`,
		uuid.NewString(), "jti-cache", `{"type":"agent_proxy","user":"u9","sandbox_id":"`+sandboxID.String()+`"}`).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}
	if err := db.Exec(`INSERT INTO sessions (id, sandbox_id) VALUES (?, ?)`, sessionID.String(), sandboxID.String()).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}

	cache := NewAttributionCache(10, time.Minute)

	var first model.Generation
	extractAttribution(db, cache, "jti-cache", &first)
	if first.SessionID == nil || *first.SessionID != sessionID {
		t.Fatalf("first call session_id = %v, want %s", first.SessionID, sessionID)
	}
	if first.UserID != "u9" {
		t.Fatalf("first call user_id = %q, want u9", first.UserID)
	}

	closeUnderlyingDB(t, db)

	var second model.Generation
	extractAttribution(db, cache, "jti-cache", &second)
	if second.SessionID == nil || *second.SessionID != sessionID {
		t.Fatalf("second call resolved from DB, not cache: session_id = %v", second.SessionID)
	}
	if second.UserID != "u9" {
		t.Fatalf("second call user_id = %q, want u9 from cache", second.UserID)
	}
}

func TestExtractAttributionNegativeCacheSkipsDB(t *testing.T) {
	db := generationTestDB(t)
	if err := db.Exec(`INSERT INTO tokens (id, jti, meta) VALUES (?, ?, ?)`,
		uuid.NewString(), "jti-plain", `{"type":"agent_proxy"}`).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}

	cache := NewAttributionCache(10, time.Minute)

	var first model.Generation
	extractAttribution(db, cache, "jti-plain", &first)
	if first.SessionID != nil {
		t.Fatalf("expected no session, got %v", first.SessionID)
	}
	if cache.Len() != 1 {
		t.Fatalf("negative result not cached, len = %d", cache.Len())
	}

	closeUnderlyingDB(t, db)

	var second model.Generation
	extractAttribution(db, cache, "jti-plain", &second)
	if second.SessionID != nil {
		t.Fatalf("second call should stay negative from cache, got %v", second.SessionID)
	}
}
