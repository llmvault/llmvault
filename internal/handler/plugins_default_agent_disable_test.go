package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// A default_agent_install plugin (agent-builder, skill-manager, service-discovery)
// must not be removable from the org's default Hivy agent through the UI disable
// endpoint — but it stays removable from any other agent it was opted onto. This
// mirrors the MCP update_agent protection; both go through
// pluginstore.PluginDetachLock.
func TestDisableForAgent_DefaultAgentInstallLockedOnDefaultAgent(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)

	plugin := model.Plugin{
		ID:       uuid.New(),
		Slug:     "default-agent-plugin-" + uuid.NewString()[:8],
		Name:     "Default Agent Plugin",
		Status:   model.PluginStatusActive,
		Manifest: model.RawJSON(`{"default_agent_install":true}`),
	}
	if err := db.Create(&plugin).Error; err != nil {
		t.Fatalf("create plugin: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", plugin.ID).Delete(&model.Plugin{}) })
	if err := db.Create(&model.OrgPluginInstall{ID: uuid.New(), OrgID: org.ID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("create org install: %v", err)
	}

	h := handler.NewPluginHandler(db)
	r := chi.NewRouter()
	r.Delete("/v1/agents/{id}/plugins/{slug}", h.DisableForAgent)

	disable := func(agentID uuid.UUID) int {
		req := httptest.NewRequest(http.MethodDelete, "/v1/agents/"+agentID.String()+"/plugins/"+plugin.Slug, nil)
		req = middleware.WithOrg(req, &org)
		req = middleware.WithAPIKeyClaims(req, &middleware.APIKeyClaims{OrgID: org.ID.String()})
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr.Code
	}

	// Default agent: removal is blocked (409) and the plugin stays installed.
	defAgent := createAutoInstallHandlerTestAgent(t, db, org.ID)
	if err := db.Model(&model.Agent{}).Where("id = ?", defAgent.ID).Update("is_default", true).Error; err != nil {
		t.Fatalf("mark default: %v", err)
	}
	if err := db.Create(&model.AgentPluginInstall{OrgID: org.ID, AgentID: defAgent.ID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("attach to default agent: %v", err)
	}
	if code := disable(defAgent.ID); code != http.StatusConflict {
		t.Fatalf("default agent disable status = %d, want 409", code)
	}
	assertAgentPluginInstalled(t, db, org.ID, defAgent.ID, plugin.ID)

	// Non-default agent: removal is allowed (200) and the plugin is gone.
	sideAgent := createAutoInstallHandlerTestAgent(t, db, org.ID)
	if err := db.Create(&model.AgentPluginInstall{OrgID: org.ID, AgentID: sideAgent.ID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("attach to side agent: %v", err)
	}
	if code := disable(sideAgent.ID); code != http.StatusOK {
		t.Fatalf("non-default agent disable status = %d, want 200", code)
	}
	var count int64
	db.Model(&model.AgentPluginInstall{}).
		Where("agent_id = ? AND plugin_id = ?", sideAgent.ID, plugin.ID).
		Count(&count)
	if count != 0 {
		t.Fatalf("plugin should be removed from the non-default agent, count = %d", count)
	}
}
