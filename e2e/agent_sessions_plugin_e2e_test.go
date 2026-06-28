package e2e

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

type agentSessionsPluginFixture struct {
	PluginID     uuid.UUID
	PluginSlug   string
	ConnectionID uuid.UUID
	InstallID    uuid.UUID
}

type agentSessionsPluginResponse struct {
	ID              string   `json:"id"`
	Slug            string   `json:"slug"`
	Installed       bool     `json:"installed"`
	EnabledAgentIDs []string `json:"enabled_agent_ids"`
}

func agentSessionsSeedPluginFixture(t *testing.T, orgIDRaw, userIDRaw, runID string) agentSessionsPluginFixture {
	t.Helper()
	db := agentSessionsOpenDB(t)
	orgID := uuid.MustParse(orgIDRaw)
	userID := uuid.MustParse(userIDRaw)
	provider := "linear"

	var integration model.Integration
	createdIntegration := false
	result := db.Where("provider = ? AND custom_app = false AND deleted_at IS NULL", provider).Limit(1).Find(&integration)
	if result.Error != nil {
		t.Fatalf("load linear integration: %v", result.Error)
	}
	if result.RowsAffected == 0 {
		integration = model.Integration{
			ID:          uuid.New(),
			UniqueKey:   "linear-e2e-" + runID,
			Provider:    provider,
			DisplayName: "Linear",
		}
		if err := db.Create(&integration).Error; err != nil {
			t.Fatalf("create linear integration: %v", err)
		}
		createdIntegration = true
	}

	conn := model.Connection{
		ID:                uuid.New(),
		OrgID:             orgID,
		UserID:            userID,
		IntegrationID:     integration.ID,
		NangoConnectionID: "agent-sessions-linear-" + runID,
		Meta:              model.JSON{},
	}
	if err := db.Create(&conn).Error; err != nil {
		t.Fatalf("create linear connection fixture: %v", err)
	}

	pluginID := uuid.New()
	slug := "agent-sessions-linear-" + runID
	plugin := model.Plugin{
		ID:          pluginID,
		Slug:        slug,
		Name:        "Agent Sessions Linear " + runID,
		Description: "E2E plugin for install sync",
		Category:    "productivity",
		Status:      model.PluginStatusActive,
		Manifest:    model.RawJSON(`{}`),
	}
	if err := db.Create(&plugin).Error; err != nil {
		t.Fatalf("create plugin fixture: %v", err)
	}
	if err := db.Create(&model.PluginIntegration{
		PluginID: pluginID,
		Provider: provider,
		Kind:     model.PluginIntegrationKindIntegration,
		Required: true,
	}).Error; err != nil {
		t.Fatalf("create plugin integration fixture: %v", err)
	}
	desc := "E2E skill installed through the plugin flow"
	skill := model.Skill{
		ID:          uuid.New(),
		PluginID:    &pluginID,
		Slug:        "agent-sessions-linear-skill-" + runID,
		Name:        "Agent Sessions Linear Skill " + runID,
		Description: &desc,
		Category:    "productivity",
		SourceType:  model.SkillSourceInline,
		RepoRef:     "main",
		Bundle: model.RawJSON(`{
			"description":"E2E plugin skill",
			"content":"Use this skill only as evidence that plugin-installed skills reached the runtime config."
		}`),
		Status: model.SkillStatusPublished,
	}
	if err := db.Create(&skill).Error; err != nil {
		t.Fatalf("create plugin skill fixture: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ? AND plugin_id = ?", orgID, pluginID).Delete(&model.AgentPluginInstall{})
		db.Where("org_id = ? AND plugin_id = ?", orgID, pluginID).Delete(&model.OrgPluginInstall{})
		db.Where("plugin_id = ?", pluginID).Delete(&model.Skill{})
		db.Where("plugin_id = ?", pluginID).Delete(&model.PluginIntegration{})
		db.Where("id = ?", pluginID).Delete(&model.Plugin{})
		db.Where("id = ?", conn.ID).Delete(&model.Connection{})
		if createdIntegration {
			db.Where("id = ?", integration.ID).Delete(&model.Integration{})
		}
	})
	return agentSessionsPluginFixture{PluginID: pluginID, PluginSlug: slug, ConnectionID: conn.ID}
}

func agentSessionsInstallPlugin(t *testing.T, ctx context.Context, baseURL, token, orgID, slug string) agentSessionsPluginResponse {
	t.Helper()
	var out agentSessionsPluginResponse
	agentSessionsJSON(t, ctx, "POST", baseURL+"/v1/plugins/"+slug+"/install", token, orgID, nil, 201, &out)
	if !out.Installed {
		t.Fatalf("plugin install response installed=false: %+v", out)
	}
	return out
}
