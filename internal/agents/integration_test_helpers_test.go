package agents

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.Open(testdb.DatabaseURL()), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	testdb.ApplyMigrations(t, db)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}
func testOrg(t *testing.T, db *gorm.DB) model.Org {
	t.Helper()
	org := model.Org{ID: uuid.New(), Name: "agents-test-" + uuid.NewString()[:8], RateLimit: 1000, Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	return org
}
func testTeam(t *testing.T, db *gorm.DB, orgID uuid.UUID) model.Team {
	t.Helper()
	team := model.Team{ID: uuid.New(), OrgID: orgID, Name: "agents-team-" + uuid.NewString()[:8]}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	return team
}
func noopDeps(db *gorm.DB) Deps {
	return Deps{DB: db, DefaultModel: "deepseek-v4-flash", ValidateModel: func(context.Context, uuid.UUID, string) error { return nil }}
}
