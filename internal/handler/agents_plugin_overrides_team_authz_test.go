package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/pluginresolve"
)

func TestAgentUpdate_MemberDisablesInheritedPluginForOnlyThatAgent(t *testing.T) {
	fx := newAgentAuthzHarness(t)
	plugin := model.Plugin{
		ID:          uuid.New(),
		OrgID:       &fx.org.ID,
		Slug:        "agent-override-" + uuid.NewString()[:8],
		Name:        "Agent Override",
		Status:      model.PluginStatusActive,
		Description: "test plugin",
	}
	if err := fx.db.Create(&plugin).Error; err != nil {
		t.Fatalf("create plugin: %v", err)
	}
	if err := fx.db.Create(&model.OrgPluginInstall{ID: uuid.New(), OrgID: fx.org.ID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("install plugin: %v", err)
	}
	if err := fx.db.Create(&model.TeamPlugin{OrgID: fx.org.ID, TeamID: fx.teamA.ID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("grant plugin to team: %v", err)
	}
	t.Cleanup(func() { fx.db.Where("id = ?", plugin.ID).Delete(&model.Plugin{}) })

	disabledAgent := seedTeamAgent(t, fx.db, fx.org.ID, fx.teamA.ID)
	enabledAgent := seedTeamAgent(t, fx.db, fx.org.ID, fx.teamA.ID)
	rr := fx.doAgentReq(t, http.MethodPatch, "/v1/agents/"+disabledAgent.ID.String(), &fx.memberA,
		map[string]any{"disabled_plugin_ids": []string{plugin.ID.String()}})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	var override model.AgentPluginOverride
	if err := fx.db.Where("agent_id = ? AND plugin_id = ?", disabledAgent.ID, plugin.ID).First(&override).Error; err != nil {
		t.Fatalf("load override: %v", err)
	}
	if override.OrgID != fx.org.ID || override.DisabledBy == nil || *override.DisabledBy != fx.memberA.ID {
		t.Fatalf("override = %#v, want org and disabling member", override)
	}

	disabledIDs, err := pluginresolve.EffectivePluginIDs(t.Context(), fx.db, disabledAgent)
	if err != nil {
		t.Fatalf("resolve disabled agent: %v", err)
	}
	if containsPluginID(disabledIDs, plugin.ID) {
		t.Fatal("disabled agent still has inherited plugin")
	}
	enabledIDs, err := pluginresolve.EffectivePluginIDs(t.Context(), fx.db, enabledAgent)
	if err != nil {
		t.Fatalf("resolve enabled agent: %v", err)
	}
	if !containsPluginID(enabledIDs, plugin.ID) {
		t.Fatal("agent override leaked to another team agent")
	}

	var response struct {
		Agent struct {
			DisabledPluginIDs []string `json:"disabled_plugin_ids"`
		} `json:"agent"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Agent.DisabledPluginIDs) != 1 || response.Agent.DisabledPluginIDs[0] != plugin.ID.String() {
		t.Fatalf("disabled_plugin_ids = %v, want [%s]", response.Agent.DisabledPluginIDs, plugin.ID)
	}
}

func TestAgentUpdate_CannotDisableCatalogRequiredPlugin(t *testing.T) {
	fx := newAgentAuthzHarness(t)
	plugin := model.Plugin{
		ID:          uuid.New(),
		OrgID:       &fx.org.ID,
		Slug:        "required-override-" + uuid.NewString()[:8],
		Name:        "Required Override",
		Status:      model.PluginStatusActive,
		Description: "test plugin",
	}
	if err := fx.db.Create(&plugin).Error; err != nil {
		t.Fatalf("create plugin: %v", err)
	}
	if err := fx.db.Create(&model.OrgPluginInstall{ID: uuid.New(), OrgID: fx.org.ID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("install plugin: %v", err)
	}
	if err := fx.db.Create(&model.TeamPlugin{OrgID: fx.org.ID, TeamID: fx.teamA.ID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("grant plugin to team: %v", err)
	}
	catalog := model.AgentCatalog{
		ID:              uuid.New(),
		Slug:            "required-catalog-" + uuid.NewString()[:8],
		Name:            "Required Catalog",
		Status:          model.AgentCatalogStatusActive,
		Manifest:        model.RawJSON(`{}`),
		RequiredPlugins: pq.StringArray{plugin.Slug},
	}
	if err := fx.db.Create(&catalog).Error; err != nil {
		t.Fatalf("create catalog: %v", err)
	}
	t.Cleanup(func() { fx.db.Where("id = ?", plugin.ID).Delete(&model.Plugin{}) })
	t.Cleanup(func() { fx.db.Where("id = ?", catalog.ID).Delete(&model.AgentCatalog{}) })

	agent := seedTeamAgent(t, fx.db, fx.org.ID, fx.teamA.ID)
	if err := fx.db.Model(&model.Agent{}).Where("id = ?", agent.ID).Update("agent_catalog_id", catalog.ID).Error; err != nil {
		t.Fatalf("attach catalog to agent: %v", err)
	}
	rr := fx.doAgentReq(t, http.MethodPatch, "/v1/agents/"+agent.ID.String(), &fx.memberA,
		map[string]any{"disabled_plugin_ids": []string{plugin.ID.String()}})
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d, want 422; body=%s", rr.Code, rr.Body.String())
	}
	var count int64
	if err := fx.db.Model(&model.AgentPluginOverride{}).
		Where("agent_id = ? AND plugin_id = ?", agent.ID, plugin.ID).
		Count(&count).Error; err != nil {
		t.Fatalf("count overrides: %v", err)
	}
	if count != 0 {
		t.Fatalf("stored %d overrides for required plugin", count)
	}
}

func containsPluginID(ids []uuid.UUID, want uuid.UUID) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
