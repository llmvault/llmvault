package memory

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

func TestAgentMissionLoadsCatalogCategoryWithoutAmbiguousColumns(t *testing.T) {
	db, err := gorm.Open(postgres.Open(testdb.DatabaseURL()), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	testdb.ApplyMigrations(t, db)
	ctx := t.Context()

	org := model.Org{ID: uuid.New(), Name: "memory-mission-" + uuid.NewString(), Active: true}
	team := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "Engineering"}
	catalog := model.AgentCatalog{
		ID:       uuid.New(),
		Slug:     "memory-mission-" + uuid.NewString(),
		Name:     "Engineering agent",
		Category: AgentCategoryEngineering,
		Status:   model.AgentCatalogStatusActive,
	}
	agent := model.Agent{
		ID:             uuid.New(),
		OrgID:          &org.ID,
		TeamID:         team.ID,
		AgentCatalogID: &catalog.ID,
		Name:           "Engineering agent",
		Model:          "test-model",
		Status:         "active",
	}
	for _, row := range []any{&org, &team, &catalog, &agent} {
		if err := db.WithContext(ctx).Create(row).Error; err != nil {
			t.Fatalf("seed %T: %v", row, err)
		}
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		for _, cleanup := range []struct {
			name   string
			id     uuid.UUID
			target any
		}{
			{name: "agent", id: agent.ID, target: &model.Agent{}},
			{name: "catalog agent", id: catalog.ID, target: &model.AgentCatalog{}},
			{name: "team", id: team.ID, target: &model.Team{}},
			{name: "org", id: org.ID, target: &model.Org{}},
		} {
			if err := db.WithContext(cleanupCtx).Where("id = ?", cleanup.id).Delete(cleanup.target).Error; err != nil {
				t.Errorf("clean up %s: %v", cleanup.name, err)
			}
		}
	})

	mission, err := AgentMission(ctx, db, org.ID, agent.ID)
	if err != nil {
		t.Fatalf("load agent mission: %v", err)
	}
	if want := MissionTemplate(AgentCategoryEngineering); mission != want {
		t.Fatalf("agent mission = %q, want %q", mission, want)
	}
}
