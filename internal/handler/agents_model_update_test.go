package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
	sandboxpkg "github.com/usehivy/hivy/internal/sandbox"
)

const (
	agentModelUpdateOriginalModel = "deepseek-v4-flash"
	agentModelUpdateNewModel      = "qwen3.7-plus"
)

func TestAgentHandlerUpdateModelStoresWithoutRuntimePush(t *testing.T) {
	h, runtime, provider, db := newAgentModelUpdateRuntimeHarness(t)
	org := createTestOrg(t, db)
	seedSessionModelCredential(t, db, org.ID)
	agent := createAgentModelUpdateTestAgent(t, db, org.ID)

	rr := patchAgentModel(t, h, &org, agent.ID, agentModelUpdateNewModel)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}

	assertAgentModelStored(t, db, agent.ID, agentModelUpdateNewModel)
	if len(provider.created) != 0 {
		t.Fatalf("provider created %d sandboxes, want 0", len(provider.created))
	}
	if runtime.configCalls != 0 {
		t.Fatalf("runtime config calls = %d, want 0", runtime.configCalls)
	}
	if count := countAgentModelUpdateSandboxes(t, db, org.ID, agent.ID); count != 0 {
		t.Fatalf("sandbox rows = %d, want 0", count)
	}
	assertAgentModelResponse(t, rr, agentModelUpdateNewModel)
}

func newAgentModelUpdateRuntimeHarness(t *testing.T) (*handler.AgentHandler, *sessionRuntimeStub, *sessionSandboxProviderStub, *gorm.DB) {
	t.Helper()
	db := connectTestDB(t)
	runtime := newSessionRuntimeStub(t, http.StatusOK)
	encKey := sessionTestEncKey(t)
	cfg := &config.Config{
		SandboxProviderID:        sandboxpkg.ProviderMicrosandbox,
		SandboxesRuntimeImageTag: "model-update-test-amd64",
		APIWebhookBaseURL:        "https://api.example.test",
		ProxyHost:                "https://proxy.example.test",
		MCPBaseURL:               "https://mcp.example.test",
	}
	provider := &sessionSandboxProviderStub{endpoint: runtime.server.URL}
	orchestrator := sandboxpkg.NewOrchestrator(db, provider, encKey, cfg)
	compileDeps := agentruntime.CompileDeps{
		DB:         db,
		EncKey:     encKey,
		SigningKey: []byte("agent-model-update-test-signing-key"),
		Cfg:        cfg,
	}
	h := handler.NewAgentHandler(db, orchestrator, compileDeps, registry.Global())
	return h, runtime, provider, db
}

func createAgentModelUpdateTestAgent(t *testing.T, db *gorm.DB, orgID uuid.UUID) model.Agent {
	t.Helper()
	agent := model.Agent{
		ID:              uuid.New(),
		OrgID:           &orgID,
		TeamID:          firstTeamID(t, db, orgID),
		Name:            "model-update-" + uuid.NewString()[:8],
		Description:     ptrString("model update test agent"),
		SandboxSize:     model.DefaultAgentSandboxSize,
		Model:           agentModelUpdateOriginalModel,
		Tools:           model.JSON{},
		McpServers:      model.RawJSON("[]"),
		Skills:          model.JSON{},
		RuntimeConfig:   model.JSON{},
		Permissions:     model.JSON{},
		Resources:       model.JSON{},
		Status:          "active",
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", agent.ID).Delete(&model.Agent{}) })
	return agent
}

func patchAgentModel(t *testing.T, h *handler.AgentHandler, org *model.Org, agentID uuid.UUID, modelID string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{"model": modelID})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPatch, "/v1/agents/"+agentID.String()+"/model", bytes.NewReader(body))
	req = withChiURLParam(req, "id", agentID.String())
	req = middleware.WithOrg(req, org)
	rr := httptest.NewRecorder()
	h.UpdateModel(rr, req)
	return rr
}

func assertAgentModelStored(t *testing.T, db *gorm.DB, agentID uuid.UUID, want string) {
	t.Helper()
	var stored model.Agent
	if err := db.First(&stored, "id = ?", agentID).Error; err != nil {
		t.Fatalf("load stored agent: %v", err)
	}
	if stored.Model != want {
		t.Fatalf("stored model = %q, want %q", stored.Model, want)
	}
}

func assertAgentModelResponse(t *testing.T, rr *httptest.ResponseRecorder, want string) {
	t.Helper()
	var payload map[string]map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := payload["agent"]["model"]; got != want {
		t.Fatalf("response model = %v, want %q", got, want)
	}
}

func countAgentModelUpdateSandboxes(t *testing.T, db *gorm.DB, orgID, agentID uuid.UUID) int64 {
	t.Helper()
	var count int64
	if err := db.Model(&model.Sandbox{}).
		Where("org_id = ? AND agent_id = ?", orgID, agentID).
		Count(&count).Error; err != nil {
		t.Fatalf("count sandboxes: %v", err)
	}
	return count
}
