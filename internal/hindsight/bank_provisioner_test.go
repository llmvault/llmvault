package hindsight

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

func TestBankProvisioner_EnsuresOrgBankConfigMentalModelAndTracker(t *testing.T) {
	db := openHindsightBankTestDB(t)
	orgID := uuid.New()
	if err := db.Create(&model.Org{ID: orgID, Name: "bank-org-" + uuid.NewString()[:8], Active: true}).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}

	var configCalls, mentalModelCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch {
		case r.Method == http.MethodPatch && strings.HasSuffix(r.URL.Path, "/config"):
			configCalls++
			var req BankConfigUpdate
			if err := json.Unmarshal(body, &req); err != nil {
				t.Fatalf("decode config: %v", err)
			}
			if req.Updates["retain_mission"] == "" {
				t.Fatalf("missing retain mission in config payload: %#v", req.Updates)
			}
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/mental-models"):
			mentalModelCalls++
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected hindsight request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)

	provisioner := NewBankProvisioner(db, NewClient(srv.URL))
	if err := provisioner.EnsureOrgBank(context.Background(), orgID); err != nil {
		t.Fatalf("ensure org bank: %v", err)
	}
	if configCalls != 1 || mentalModelCalls != 1 {
		t.Fatalf("calls config=%d mental_model=%d, want 1/1", configCalls, mentalModelCalls)
	}
	var bank model.HindsightBank
	if err := db.First(&bank, "bank_id = ?", OrgBankID(orgID)).Error; err != nil {
		t.Fatalf("load bank tracker: %v", err)
	}
	if bank.ConfigHash != OrgBankConfigHash(orgID, DefaultMemoryConfig()) {
		t.Fatalf("config hash = %q", bank.ConfigHash)
	}

	if err := provisioner.EnsureOrgBank(context.Background(), orgID); err != nil {
		t.Fatalf("second ensure org bank: %v", err)
	}
	if configCalls != 1 || mentalModelCalls != 1 {
		t.Fatalf("second ensure should be idempotent, calls config=%d mental_model=%d", configCalls, mentalModelCalls)
	}
}

func openHindsightBankTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := testdb.DatabaseURL("DATABASE_URL", "HIVY_DATABASE_URL", "TEST_DATABASE_URL")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	testdb.ApplyMigrations(t, db)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}
