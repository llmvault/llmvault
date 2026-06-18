package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	"github.com/usehivy/hivy/internal/registry"
)

func TestAgentHandlerCreateAutoInstallsRuntimePlugin(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	plugin := seedAutoInstallPlugin(t, db, "runtime-create")
	seedDefaultModelCredential(t, db)

	h := handler.NewAgentHandler(db, nil, agentruntime.CompileDeps{}, registry.Global())
	r := chi.NewRouter()
	r.Post("/v1/agents", h.Create)

	body := bytes.NewReader([]byte(`{"name":"Runtime Create Agent"}`))
	req := httptest.NewRequest(http.MethodPost, "/v1/agents", body)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Agent struct {
			ID string `json:"id"`
		} `json:"agent"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	agentID, err := uuid.Parse(resp.Agent.ID)
	if err != nil {
		t.Fatalf("parse agent id: %v", err)
	}
	var agent model.Agent
	if err := db.First(&agent, "id = ?", agentID).Error; err != nil {
		t.Fatalf("load created agent: %v", err)
	}
	if got, want := agent.SandboxImage, model.SandboxImageDefault; got != want {
		t.Fatalf("sandbox_image = %q, want %q", got, want)
	}
	assertAgentPluginInstalled(t, db, org.ID, agentID, plugin.ID)
	assertOrgPluginInstalled(t, db, org.ID, plugin.ID)
}

func TestAgentCatalogInstallAutoInstallsRuntimePlugin(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	plugin := seedAutoInstallPlugin(t, db, "runtime-catalog")
	seedDefaultModelCredential(t, db)

	catalog := model.AgentCatalog{
		ID:              uuid.New(),
		Slug:            "runtime-catalog-agent-" + uuid.NewString()[:8],
		Name:            "Runtime Catalog Agent " + uuid.NewString()[:8],
		Model:           agentruntime.DefaultAgentModel,
		AvailableModels: pq.StringArray{agentruntime.DefaultAgentModel},
		SandboxStrategy: "per_session",
		SandboxImage:    model.SandboxImageDeveloper,
		Manifest:        model.RawJSON(`{}`),
		Status:          model.AgentCatalogStatusActive,
	}
	if err := db.Create(&catalog).Error; err != nil {
		t.Fatalf("create catalog agent: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", catalog.ID).Delete(&model.AgentCatalog{}) })

	h := handler.NewAgentHandler(db, nil, agentruntime.CompileDeps{}, registry.Global())
	r := chi.NewRouter()
	r.Post("/v1/agents/catalog/{slug}/install", h.InstallCatalog)

	req := httptest.NewRequest(http.MethodPost, "/v1/agents/catalog/"+catalog.Slug+"/install", nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}

	var agent model.Agent
	if err := db.Where("org_id = ? AND agent_catalog_id = ?", org.ID, catalog.ID).First(&agent).Error; err != nil {
		t.Fatalf("load catalog-created agent: %v", err)
	}
	if got, want := agent.SandboxImage, model.SandboxImageDeveloper; got != want {
		t.Fatalf("sandbox_image = %q, want %q", got, want)
	}
	assertAgentPluginInstalled(t, db, org.ID, agent.ID, plugin.ID)
	assertOrgPluginInstalled(t, db, org.ID, plugin.ID)
}

func TestPluginHandlerRejectsRemovingLockedRuntimePlugin(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	agent := createAutoInstallHandlerTestAgent(t, db, org.ID)
	plugin := seedAutoInstallPlugin(t, db, "runtime-locked")
	if err := db.Create(&model.OrgPluginInstall{ID: uuid.New(), OrgID: org.ID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("create org install: %v", err)
	}
	if err := db.Create(&model.AgentPluginInstall{OrgID: org.ID, AgentID: agent.ID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("create agent install: %v", err)
	}

	h := handler.NewPluginHandler(db, &recordingEnqueuer{})
	r := chi.NewRouter()
	r.Delete("/v1/plugins/{slug}/install", h.Uninstall)
	r.Delete("/v1/agents/{id}/plugins/{slug}", h.DisableForAgent)

	orgReq := httptest.NewRequest(http.MethodDelete, "/v1/plugins/"+plugin.Slug+"/install", nil)
	orgReq = middleware.WithOrg(orgReq, &org)
	orgRR := httptest.NewRecorder()
	r.ServeHTTP(orgRR, orgReq)
	if orgRR.Code != http.StatusConflict {
		t.Fatalf("org uninstall status = %d, body = %s", orgRR.Code, orgRR.Body.String())
	}

	agentReq := httptest.NewRequest(http.MethodDelete, "/v1/agents/"+agent.ID.String()+"/plugins/"+plugin.Slug, nil)
	agentReq = middleware.WithOrg(agentReq, &org)
	agentRR := httptest.NewRecorder()
	r.ServeHTTP(agentRR, agentReq)
	if agentRR.Code != http.StatusConflict {
		t.Fatalf("agent disable status = %d, body = %s", agentRR.Code, agentRR.Body.String())
	}

	assertAgentPluginInstalled(t, db, org.ID, agent.ID, plugin.ID)
	assertOrgPluginInstalled(t, db, org.ID, plugin.ID)
}

func seedAutoInstallPlugin(t *testing.T, db *gorm.DB, slugPrefix string) model.Plugin {
	t.Helper()
	plugin := model.Plugin{
		ID:       uuid.New(),
		Slug:     slugPrefix + "-" + uuid.NewString()[:8],
		Name:     "Runtime",
		Status:   model.PluginStatusActive,
		Manifest: model.RawJSON(`{"auto_install":true,"locked":true}`),
	}
	if err := db.Create(&plugin).Error; err != nil {
		t.Fatalf("create auto-install plugin: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", plugin.ID).Delete(&model.Plugin{}) })
	return plugin
}

func seedDefaultModelCredential(t *testing.T, db *gorm.DB) {
	t.Helper()
	cred := model.Credential{
		ID:           uuid.New(),
		Label:        "runtime-autoinstall-" + uuid.NewString()[:8],
		BaseURL:      "https://openrouter.ai/api/v1",
		AuthScheme:   "bearer",
		ProviderID:   "openrouter",
		EncryptedKey: []byte("enc"),
		WrappedDEK:   []byte("dek"),
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("create default model credential: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", cred.ID).Delete(&model.Credential{}) })
}

func createAutoInstallHandlerTestAgent(t *testing.T, db *gorm.DB, orgID uuid.UUID) model.Agent {
	t.Helper()
	agent := model.Agent{
		ID:              uuid.New(),
		OrgID:           &orgID,
		Name:            fmt.Sprintf("runtime-locked-agent-%s", uuid.NewString()[:8]),
		SandboxStrategy: "per_session",
		SandboxSize:     model.DefaultAgentSandboxSize,
		Model:           agentruntime.DefaultAgentModel,
		AvailableModels: []string{agentruntime.DefaultAgentModel},
		Status:          "active",
		Tools:           model.JSON{},
		McpServers:      model.RawJSON("[]"),
		Skills:          model.JSON{},
		RuntimeConfig:   model.JSON{},
		Permissions:     model.JSON{},
		Resources:       model.JSON{},
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return agent
}

func assertAgentPluginInstalled(t *testing.T, db *gorm.DB, orgID, agentID, pluginID uuid.UUID) {
	t.Helper()
	var count int64
	if err := db.Model(&model.AgentPluginInstall{}).
		Where("org_id = ? AND agent_id = ? AND plugin_id = ?", orgID, agentID, pluginID).
		Count(&count).Error; err != nil {
		t.Fatalf("count agent plugin install: %v", err)
	}
	if count != 1 {
		t.Fatalf("agent plugin install count = %d, want 1", count)
	}
}

func assertOrgPluginInstalled(t *testing.T, db *gorm.DB, orgID, pluginID uuid.UUID) {
	t.Helper()
	var count int64
	if err := db.Model(&model.OrgPluginInstall{}).
		Where("org_id = ? AND plugin_id = ? AND revoked_at IS NULL", orgID, pluginID).
		Count(&count).Error; err != nil {
		t.Fatalf("count org plugin install: %v", err)
	}
	if count != 1 {
		t.Fatalf("org plugin install count = %d, want 1", count)
	}
}
