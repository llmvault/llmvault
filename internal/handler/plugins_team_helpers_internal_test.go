package handler

import (
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func firstTeamID(t *testing.T, db *gorm.DB, orgID uuid.UUID) uuid.UUID {
	t.Helper()
	var team model.Team
	err := db.Where("org_id = ?", orgID).Order("created_at ASC").First(&team).Error
	if err == nil {
		return team.ID
	}
	team = model.Team{ID: uuid.New(), OrgID: orgID, Name: "seed-team-" + uuid.NewString()[:8]}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create seed team: %v", err)
	}
	return team.ID
}

func grantPluginToAgentTeam(t *testing.T, db *gorm.DB, orgID, agentID, pluginID uuid.UUID) {
	t.Helper()
	var agent model.Agent
	if err := db.First(&agent, "id = ?", agentID).Error; err != nil {
		t.Fatalf("load agent for grant: %v", err)
	}
	if err := db.Create(&model.TeamPlugin{OrgID: orgID, TeamID: agent.TeamID, PluginID: pluginID}).Error; err != nil {
		t.Fatalf("grant plugin to team: %v", err)
	}
}
