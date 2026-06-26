package agentcatalog

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

func connectAgentCatalogTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := testdb.DatabaseURL("DATABASE_URL", "HIVY_DATABASE_URL", "TEST_DATABASE_URL")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect Postgres: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	testdb.ApplyMigrations(t, db)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func TestValidatePluginReferencesRejectsUnknownPlugin(t *testing.T) {
	db := connectAgentCatalogTestDB(t)
	err := validatePluginReferences(context.Background(), db, []Manifest{{
		Slug:    "hakaree",
		Plugins: PluginManifest{Required: []string{"missing-plugin"}},
	}})
	if err == nil || !strings.Contains(err.Error(), "unknown or inactive plugin") {
		t.Fatalf("validatePluginReferences error = %v, want unknown plugin error", err)
	}
}

func TestValidatePluginReferencesAcceptsActivePlugin(t *testing.T) {
	db := connectAgentCatalogTestDB(t)
	plugin := model.Plugin{
		ID:       uuid.New(),
		Slug:     "known-plugin-" + uuid.NewString()[:8],
		Name:     "Known Plugin",
		Status:   model.PluginStatusActive,
		Manifest: model.RawJSON(`{}`),
	}
	if err := db.Create(&plugin).Error; err != nil {
		t.Fatalf("create plugin: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", plugin.ID).Delete(&model.Plugin{}) })

	err := validatePluginReferences(context.Background(), db, []Manifest{{
		Slug:    "hakaree",
		Plugins: PluginManifest{Required: []string{plugin.Slug}},
	}})
	if err != nil {
		t.Fatalf("validatePluginReferences: %v", err)
	}
}
