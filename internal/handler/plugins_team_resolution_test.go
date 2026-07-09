package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func seedRequiredPluginCatalogAgent(t *testing.T, db *gorm.DB, orgID, teamID uuid.UUID, requiredSlug string) model.Agent {
	t.Helper()
	catalog := model.AgentCatalog{
		ID:              uuid.New(),
		Slug:            "req-cat-" + uuid.NewString()[:8],
		Name:            "Req Cat " + uuid.NewString()[:8],
		Model:           agentruntime.DefaultAgentModel,
		SandboxImage:    model.SandboxImageDefault,
		RequiredPlugins: pq.StringArray{requiredSlug},
		Manifest:        model.RawJSON(`{}`),
		Status:          model.AgentCatalogStatusActive,
	}
	if err := db.Create(&catalog).Error; err != nil {
		t.Fatalf("create catalog: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", catalog.ID).Delete(&model.AgentCatalog{}) })
	agent := model.Agent{
		ID:             uuid.New(),
		OrgID:          &orgID,
		TeamID:         teamID,
		AgentCatalogID: &catalog.ID,
		Name:           "req-agent-" + uuid.NewString()[:8],
		Model:          agentruntime.DefaultAgentModel,
		Status:         "active",
		Tools:          model.JSON{},
		McpServers:     model.RawJSON("[]"),
		Skills:         model.JSON{},
		RuntimeConfig:  model.JSON{},
		Permissions:    model.JSON{},
		Resources:      model.JSON{},
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create catalog agent: %v", err)
	}
	return agent
}

// D2: a team plugin disable is refused with 409 while a non-archived catalog
// agent on the team requires the plugin, and succeeds once the agent is
// archived.
func TestTeamPluginDisable_BlockedWhileCatalogAgentRequires(t *testing.T) {
	h := newTeamProvHarness(t)
	fx := (&channelHarness{db: h.db}).seed(t)
	team := tpSeedTeam(t, h.db, fx.org.ID)
	pluginID := tpSeedInstalledPlugin(t, h.db, fx.org.ID)
	var plugin model.Plugin
	if err := h.db.First(&plugin, "id = ?", pluginID).Error; err != nil {
		t.Fatalf("load plugin: %v", err)
	}
	base := "/v1/orgs/current/teams/" + team.ID.String() + "/plugins"

	if rr := h.doJSON(t, http.MethodPost, base, fx, fx.owner, map[string]any{"plugin_id": pluginID.String()}); rr.Code != http.StatusCreated {
		t.Fatalf("enable: code=%d body=%s", rr.Code, rr.Body.String())
	}
	agent := seedRequiredPluginCatalogAgent(t, h.db, fx.org.ID, team.ID, plugin.Slug)

	rr := h.doJSON(t, http.MethodDelete, base+"/"+pluginID.String(), fx, fx.owner, nil)
	if rr.Code != http.StatusConflict {
		t.Fatalf("disable while required: code=%d, want 409; body=%s", rr.Code, rr.Body.String())
	}
	var grantCount int64
	h.db.Model(&model.TeamPlugin{}).Where("team_id = ? AND plugin_id = ?", team.ID, pluginID).Count(&grantCount)
	if grantCount != 1 {
		t.Fatalf("grant removed despite 409: count=%d", grantCount)
	}

	if err := h.db.Model(&model.Agent{}).Where("id = ?", agent.ID).Update("status", "archived").Error; err != nil {
		t.Fatalf("archive agent: %v", err)
	}
	rr = h.doJSON(t, http.MethodDelete, base+"/"+pluginID.String(), fx, fx.owner, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("disable after archive: code=%d body=%s", rr.Code, rr.Body.String())
	}
}

// D2 at the org level: uninstalling an org plugin is refused with 409 while any
// active catalog agent in the org requires it, and succeeds after archiving.
// D7: the uninstall also deletes the plugin's team grants for the org.
func TestOrgPluginUninstall_BlockedWhileCatalogAgentRequires(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	team := seedTeam(t, db, org.ID, "org-uninstall")
	plugin := model.Plugin{ID: uuid.New(), Slug: "org-req-" + uuid.NewString()[:8], Name: "Org Req", Status: model.PluginStatusActive}
	if err := db.Create(&plugin).Error; err != nil {
		t.Fatalf("create plugin: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.TeamPlugin{})
		db.Where("plugin_id = ?", plugin.ID).Delete(&model.OrgPluginInstall{})
		db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		db.Where("id = ?", team.ID).Delete(&model.Team{})
		db.Where("id = ?", plugin.ID).Delete(&model.Plugin{})
	})
	if err := db.Create(&model.OrgPluginInstall{ID: uuid.New(), OrgID: org.ID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("org install: %v", err)
	}
	grantPluginToTeam(t, db, org.ID, team.ID, plugin.ID)
	agent := seedRequiredPluginCatalogAgent(t, db, org.ID, team.ID, plugin.Slug)

	h := handler.NewPluginHandler(db)
	r := chi.NewRouter()
	r.Delete("/v1/plugins/{slug}/install", h.Uninstall)
	uninstall := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodDelete, "/v1/plugins/"+plugin.Slug+"/install", nil)
		req = middleware.WithOrg(req, &org)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr
	}

	if rr := uninstall(); rr.Code != http.StatusConflict {
		t.Fatalf("uninstall while required: code=%d, want 409; body=%s", rr.Code, rr.Body.String())
	}

	if err := db.Model(&model.Agent{}).Where("id = ?", agent.ID).Update("status", "archived").Error; err != nil {
		t.Fatalf("archive agent: %v", err)
	}
	if rr := uninstall(); rr.Code != http.StatusOK {
		t.Fatalf("uninstall after archive: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var grantCount int64
	db.Model(&model.TeamPlugin{}).Where("org_id = ? AND plugin_id = ?", org.ID, plugin.ID).Count(&grantCount)
	if grantCount != 0 {
		t.Fatalf("team grants not cleaned on org uninstall: count=%d", grantCount)
	}
}

// D1 through the handler layer: a team granted BOTH GitHub identity plugins
// yields exactly one identity per agent in enabled_agent_ids — the
// catalog-required agent resolves the reviews plugin, the plain agent resolves
// the primary github plugin, and neither appears under both.
func TestPluginsList_EnabledAgentIDs_GitHubPairRule(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	team := seedTeam(t, db, org.ID, "pair-rule")

	mkGitHubPlugin := func(provider string) model.Plugin {
		plugin := model.Plugin{ID: uuid.New(), Slug: provider + "-" + uuid.NewString()[:8], Name: provider, Status: model.PluginStatusActive}
		if err := db.Create(&plugin).Error; err != nil {
			t.Fatalf("create plugin: %v", err)
		}
		if err := db.Create(&model.PluginIntegration{
			PluginID: plugin.ID, Provider: provider, Kind: model.PluginIntegrationKindIntegration, Required: true,
		}).Error; err != nil {
			t.Fatalf("create integration: %v", err)
		}
		if err := db.Create(&model.OrgPluginInstall{ID: uuid.New(), OrgID: org.ID, PluginID: plugin.ID}).Error; err != nil {
			t.Fatalf("org install: %v", err)
		}
		grantPluginToTeam(t, db, org.ID, team.ID, plugin.ID)
		t.Cleanup(func() {
			db.Where("plugin_id = ?", plugin.ID).Delete(&model.TeamPlugin{})
			db.Where("plugin_id = ?", plugin.ID).Delete(&model.OrgPluginInstall{})
			db.Where("plugin_id = ?", plugin.ID).Delete(&model.PluginIntegration{})
			db.Where("id = ?", plugin.ID).Delete(&model.Plugin{})
		})
		return plugin
	}
	primary := mkGitHubPlugin("github-app")
	reviews := mkGitHubPlugin("github-app-code-reviews")

	reviewsAgent := seedRequiredPluginCatalogAgent(t, db, org.ID, team.ID, reviews.Slug)
	plainAgent := seedTeamAgent(t, db, org.ID, team.ID)

	h := handler.NewPluginHandler(db)
	r := chi.NewRouter()
	r.Get("/v1/plugins/{slug}", h.Get)
	enabledFor := func(slug string) map[string]bool {
		req := httptest.NewRequest(http.MethodGet, "/v1/plugins/"+slug, nil)
		req = middleware.WithOrg(req, &org)
		req = middleware.WithAPIKeyClaims(req, &middleware.APIKeyClaims{OrgID: org.ID.String()})
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("get plugin %s: code=%d body=%s", slug, rr.Code, rr.Body.String())
		}
		var resp struct {
			EnabledAgentIDs []string `json:"enabled_agent_ids"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		out := map[string]bool{}
		for _, id := range resp.EnabledAgentIDs {
			out[id] = true
		}
		return out
	}

	primaryEnabled := enabledFor(primary.Slug)
	reviewsEnabled := enabledFor(reviews.Slug)

	if !primaryEnabled[plainAgent.ID.String()] {
		t.Fatalf("plain agent missing from primary github plugin: %v", primaryEnabled)
	}
	if primaryEnabled[reviewsAgent.ID.String()] {
		t.Fatalf("reviews-required agent must not resolve primary github plugin: %v", primaryEnabled)
	}
	if !reviewsEnabled[reviewsAgent.ID.String()] {
		t.Fatalf("reviews-required agent missing from reviews plugin: %v", reviewsEnabled)
	}
	if reviewsEnabled[plainAgent.ID.String()] {
		t.Fatalf("plain agent must not resolve reviews plugin: %v", reviewsEnabled)
	}
}
