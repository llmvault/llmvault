package handler_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/pluginresolve"
)

// firstTeamID returns the org's oldest team, creating one when the org has none.
// Agents require a team (agents.team_id is NOT NULL), so seeds route teamless
// agents through this helper.
func firstTeamID(t *testing.T, db *gorm.DB, orgID uuid.UUID) uuid.UUID {
	t.Helper()
	var team model.Team
	err := db.Where("org_id = ?", orgID).Order("created_at ASC").First(&team).Error
	if err == nil {
		return team.ID
	}
	created := seedTeam(t, db, orgID, "seed-team")
	return created.ID
}

// grantPluginToTeam adds a team_plugins grant so the team's agents resolve the
// plugin. Replaces the removed per-agent plugin install seed.
func grantPluginToTeam(t *testing.T, db *gorm.DB, orgID, teamID, pluginID uuid.UUID) {
	t.Helper()
	if err := db.Create(&model.TeamPlugin{OrgID: orgID, TeamID: teamID, PluginID: pluginID}).Error; err != nil {
		t.Fatalf("grant plugin to team: %v", err)
	}
}

// grantPluginToAgentTeam resolves the agent's team and grants the plugin to it,
// so the agent's effective set contains the plugin. Replaces the removed
// per-agent plugin install seed keyed by agent id.
func grantPluginToAgentTeam(t *testing.T, db *gorm.DB, orgID, agentID, pluginID uuid.UUID) {
	t.Helper()
	var agent model.Agent
	if err := db.First(&agent, "id = ?", agentID).Error; err != nil {
		t.Fatalf("load agent for grant: %v", err)
	}
	grantPluginToTeam(t, db, orgID, agent.TeamID, pluginID)
}

// agentEffectiveHasPlugin reports whether the agent's resolved effective set
// contains the plugin.
func agentEffectiveHasPlugin(t *testing.T, db *gorm.DB, agentID, pluginID uuid.UUID) bool {
	t.Helper()
	var agent model.Agent
	if err := db.First(&agent, "id = ?", agentID).Error; err != nil {
		t.Fatalf("load agent: %v", err)
	}
	ids, err := pluginresolve.EffectivePluginIDs(context.Background(), db, agent)
	if err != nil {
		t.Fatalf("resolve effective plugins: %v", err)
	}
	for _, id := range ids {
		if id == pluginID {
			return true
		}
	}
	return false
}

// assertAgentEffectivePlugin fails unless the plugin is in the agent's effective
// set. Replaces the removed agent_plugin_installs row assertion.
func assertAgentEffectivePlugin(t *testing.T, db *gorm.DB, agentID, pluginID uuid.UUID) {
	t.Helper()
	if !agentEffectiveHasPlugin(t, db, agentID, pluginID) {
		t.Fatalf("agent %s effective set missing plugin %s", agentID, pluginID)
	}
}
