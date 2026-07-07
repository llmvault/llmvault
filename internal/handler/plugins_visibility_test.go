package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/model"
)

func TestPluginEnabledAgentIDs_ActorScopedVisibility(t *testing.T) {
	db := connectTestDB(t)
	fx := seedVisFixture(t, db)
	h := handler.NewPluginHandler(db)

	plugin := model.Plugin{ID: uuid.New(), Slug: "vis-plugin-" + uuid.NewString()[:8], Name: "Vis Plugin", Status: model.PluginStatusActive}
	rows := []any{
		&plugin,
		&model.OrgPluginInstall{ID: uuid.New(), OrgID: fx.org.ID, PluginID: plugin.ID},
		&model.AgentPluginInstall{OrgID: fx.org.ID, AgentID: fx.visibleAgent.ID, PluginID: plugin.ID},
		&model.AgentPluginInstall{OrgID: fx.org.ID, AgentID: fx.hiddenAgent.ID, PluginID: plugin.ID},
	}
	for _, r := range rows {
		if err := db.Create(r).Error; err != nil {
			t.Fatalf("seed plugin: %v", err)
		}
	}
	t.Cleanup(func() {
		db.Where("plugin_id = ?", plugin.ID).Delete(&model.AgentPluginInstall{})
		db.Where("plugin_id = ?", plugin.ID).Delete(&model.OrgPluginInstall{})
		db.Where("id = ?", plugin.ID).Delete(&model.Plugin{})
	})

	getEnabled := func(c caller) map[string]bool {
		req := httptest.NewRequest(http.MethodGet, "/v1/plugins/"+plugin.Slug, nil)
		req = c.apply(req, fx.org)
		req = withURLParam(req, "slug", plugin.Slug)
		rr := httptest.NewRecorder()
		h.Get(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("get plugin status=%d body=%s", rr.Code, rr.Body.String())
		}
		var resp struct {
			EnabledAgentIDs []string `json:"enabled_agent_ids"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		ids := map[string]bool{}
		for _, id := range resp.EnabledAgentIDs {
			ids[id] = true
		}
		return ids
	}

	member := getEnabled(memberCaller(fx))
	if !member[fx.visibleAgent.ID.String()] {
		t.Fatalf("member enabled_agent_ids missing visible agent: %v", member)
	}
	if member[fx.hiddenAgent.ID.String()] {
		t.Fatalf("member enabled_agent_ids must exclude hidden agent: %v", member)
	}
	admin := getEnabled(adminCaller(fx))
	if !admin[fx.visibleAgent.ID.String()] || !admin[fx.hiddenAgent.ID.String()] {
		t.Fatalf("admin enabled_agent_ids must include both: %v", admin)
	}
	apiKey := getEnabled(apiKeyCaller())
	if !apiKey[fx.visibleAgent.ID.String()] || !apiKey[fx.hiddenAgent.ID.String()] {
		t.Fatalf("api-key enabled_agent_ids must include both: %v", apiKey)
	}
}

func TestListAgentPlugins_HiddenAgentIs404(t *testing.T) {
	db := connectTestDB(t)
	fx := seedVisFixture(t, db)
	h := handler.NewPluginHandler(db)

	status := func(c caller, agentID uuid.UUID) int {
		req := httptest.NewRequest(http.MethodGet, "/v1/agents/"+agentID.String()+"/plugins", nil)
		req = c.apply(req, fx.org)
		req = withURLParam(req, "id", agentID.String())
		rr := httptest.NewRecorder()
		h.ListAgentPlugins(rr, req)
		return rr.Code
	}

	if code := status(memberCaller(fx), fx.visibleAgent.ID); code != http.StatusOK {
		t.Fatalf("member list plugins for visible agent = %d, want 200", code)
	}
	if code := status(memberCaller(fx), fx.hiddenAgent.ID); code != http.StatusNotFound {
		t.Fatalf("member list plugins for hidden agent = %d, want 404", code)
	}
	if code := status(adminCaller(fx), fx.hiddenAgent.ID); code != http.StatusOK {
		t.Fatalf("admin list plugins for hidden agent = %d, want 200", code)
	}
}
