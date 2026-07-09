package memory

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func TestChannelCategoriesAndTemplates(t *testing.T) {
	for _, category := range ChannelCategories {
		if !ValidChannelCategory(category) {
			t.Fatalf("category %q should be valid", category)
		}
	}
	if ValidChannelCategory("random") || ValidChannelCategory("") {
		t.Fatal("unknown categories should be invalid")
	}
	if MissionTemplate(ChannelCategoryGeneral) != "" {
		t.Fatal("general must have no mission template")
	}
	if MissionTemplate("random") != "" {
		t.Fatal("unknown category must have no mission template")
	}
	for _, category := range ChannelCategories {
		if category == ChannelCategoryGeneral {
			continue
		}
		if MissionTemplate(category) == "" {
			t.Fatalf("category %q must have a mission template", category)
		}
	}
}

func seedMissionChannel(t *testing.T, db *gorm.DB, category string, mission *string) model.Channel {
	t.Helper()
	org := model.Org{ID: uuid.New(), Name: "mission-" + uuid.NewString(), Active: true, RateLimit: 1000}
	team := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "mission-team-" + uuid.NewString()[:8]}
	agent := model.Agent{ID: uuid.New(), OrgID: &org.ID, TeamID: team.ID, Name: "Mission Agent " + uuid.NewString(), Model: "test", Status: "active"}
	channel := model.Channel{
		ID:             uuid.New(),
		OrgID:          org.ID,
		Name:           "mission-" + uuid.NewString(),
		Description:    "ACME tier-1 queue",
		Category:       category,
		MemoryMission:  mission,
		DefaultAgentID: agent.ID,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.Channel{})
		db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		db.Where("org_id = ?", org.ID).Delete(&model.Team{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})
	return channel
}

func TestChannelMissionReturnsStoredMission(t *testing.T) {
	db := connectMemoryToolTestDB(t)
	template := MissionTemplate(ChannelCategoryCustomerSupport)
	channel := seedMissionChannel(t, db, ChannelCategoryCustomerSupport, &template)

	mission, err := ChannelMission(context.Background(), db, channel.OrgID, channel.ID)
	if err != nil {
		t.Fatalf("load mission: %v", err)
	}
	if mission != template {
		t.Fatalf("mission = %q, want stored template %q", mission, template)
	}
}

func TestChannelMissionEmptyWhenUnset(t *testing.T) {
	db := connectMemoryToolTestDB(t)
	channel := seedMissionChannel(t, db, ChannelCategoryGeneral, nil)

	mission, err := ChannelMission(context.Background(), db, channel.OrgID, channel.ID)
	if err != nil {
		t.Fatalf("load mission: %v", err)
	}
	if mission != "" {
		t.Fatalf("mission = %q, want empty for unset mission in a 'general' channel", mission)
	}
}

func TestChannelMissionFallsBackToCategoryTemplate(t *testing.T) {
	db := connectMemoryToolTestDB(t)
	want := MissionTemplate(ChannelCategoryEngineering)

	// Unset mission on a categorized channel: the category default applies.
	unset := seedMissionChannel(t, db, ChannelCategoryEngineering, nil)
	mission, err := ChannelMission(context.Background(), db, unset.OrgID, unset.ID)
	if err != nil {
		t.Fatalf("load mission: %v", err)
	}
	if mission != want {
		t.Fatalf("unset mission = %q, want the engineering template", mission)
	}

	// A cleared (whitespace) mission behaves like unset: reset to default.
	blank := "   "
	cleared := seedMissionChannel(t, db, ChannelCategoryEngineering, &blank)
	mission, err = ChannelMission(context.Background(), db, cleared.OrgID, cleared.ID)
	if err != nil {
		t.Fatalf("load mission: %v", err)
	}
	if mission != want {
		t.Fatalf("cleared mission = %q, want the engineering template", mission)
	}

	// A custom mission always wins over the template.
	custom := "Only retain deploy-pipeline decisions."
	set := seedMissionChannel(t, db, ChannelCategoryEngineering, &custom)
	mission, err = ChannelMission(context.Background(), db, set.OrgID, set.ID)
	if err != nil {
		t.Fatalf("load mission: %v", err)
	}
	if mission != custom {
		t.Fatalf("custom mission = %q, want %q", mission, custom)
	}
}

func TestChannelMissionMissingChannel(t *testing.T) {
	db := connectMemoryToolTestDB(t)
	mission, err := ChannelMission(context.Background(), db, uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("missing channel should not error, got %v", err)
	}
	if mission != "" {
		t.Fatalf("mission = %q, want empty", mission)
	}
}
