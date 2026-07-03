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
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

func TestAgentHandlerUpdateStoresSandboxSize(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	agent := createSandboxSizeTestAgent(t, db, org.ID, "small")
	h := handler.NewAgentHandler(db, nil, agentruntime.CompileDeps{}, registry.Global())

	rr := patchAgentSandboxSize(t, h, &org, agent.ID, "large")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}

	var stored model.Agent
	if err := db.First(&stored, "id = ?", agent.ID).Error; err != nil {
		t.Fatalf("load stored agent: %v", err)
	}
	if stored.SandboxSize != "large" {
		t.Fatalf("stored sandbox_size = %q, want large", stored.SandboxSize)
	}

	var payload map[string]map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := payload["agent"]["sandbox_size"]; got != "large" {
		t.Fatalf("response sandbox_size = %v, want large", got)
	}
}

func TestAgentHandlerUpdateStoresNanoSandboxSize(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	agent := createSandboxSizeTestAgent(t, db, org.ID, "small")
	h := handler.NewAgentHandler(db, nil, agentruntime.CompileDeps{}, registry.Global())

	rr := patchAgentSandboxSize(t, h, &org, agent.ID, "nano")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}

	var stored model.Agent
	if err := db.First(&stored, "id = ?", agent.ID).Error; err != nil {
		t.Fatalf("load stored agent: %v", err)
	}
	if stored.SandboxSize != "nano" {
		t.Fatalf("stored sandbox_size = %q, want nano", stored.SandboxSize)
	}
}

func TestAgentHandlerUpdateRejectsInvalidSandboxSize(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	agent := createSandboxSizeTestAgent(t, db, org.ID, "small")
	h := handler.NewAgentHandler(db, nil, agentruntime.CompileDeps{}, registry.Global())

	rr := patchAgentSandboxSize(t, h, &org, agent.ID, "jumbo")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rr.Code, rr.Body.String())
	}

	var stored model.Agent
	if err := db.First(&stored, "id = ?", agent.ID).Error; err != nil {
		t.Fatalf("load stored agent: %v", err)
	}
	if stored.SandboxSize != "small" {
		t.Fatalf("stored sandbox_size = %q, want small", stored.SandboxSize)
	}
}

func TestAgentHandlerUpdateStoresSandboxImage(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	agent := createSandboxConfigTestAgent(t, db, org.ID, model.SandboxImageDefault, "small")
	h := handler.NewAgentHandler(db, nil, agentruntime.CompileDeps{}, registry.Global())

	rr := patchAgentSandboxImage(t, h, &org, agent.ID, model.SandboxImageDeveloper)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}

	var stored model.Agent
	if err := db.First(&stored, "id = ?", agent.ID).Error; err != nil {
		t.Fatalf("load stored agent: %v", err)
	}
	if stored.SandboxImage != model.SandboxImageDeveloper {
		t.Fatalf("stored sandbox_image = %q, want developer", stored.SandboxImage)
	}

	var payload map[string]map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := payload["agent"]["sandbox_image"]; got != model.SandboxImageDeveloper {
		t.Fatalf("response sandbox_image = %v, want developer", got)
	}
}

func TestAgentHandlerUpdateRejectsInvalidSandboxImage(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	agent := createSandboxConfigTestAgent(t, db, org.ID, model.SandboxImageDefault, "small")
	h := handler.NewAgentHandler(db, nil, agentruntime.CompileDeps{}, registry.Global())

	rr := patchAgentSandboxImage(t, h, &org, agent.ID, "gpu")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rr.Code, rr.Body.String())
	}

	var stored model.Agent
	if err := db.First(&stored, "id = ?", agent.ID).Error; err != nil {
		t.Fatalf("load stored agent: %v", err)
	}
	if stored.SandboxImage != model.SandboxImageDefault {
		t.Fatalf("stored sandbox_image = %q, want default", stored.SandboxImage)
	}
}

func createSandboxSizeTestAgent(t *testing.T, db *gorm.DB, orgID uuid.UUID, size string) model.Agent {
	t.Helper()
	return createSandboxConfigTestAgent(t, db, orgID, model.SandboxImageDefault, size)
}

func createSandboxConfigTestAgent(t *testing.T, db *gorm.DB, orgID uuid.UUID, image, size string) model.Agent {
	t.Helper()
	agent := model.Agent{
		ID:              uuid.New(),
		OrgID:           &orgID,
		Name:            "sandbox-config-" + randSuffix(),
		SandboxImage:    image,
		SandboxSize:     size,
		Model:           agentruntime.DefaultAgentModel,
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
	t.Cleanup(func() { db.Where("id = ?", agent.ID).Delete(&model.Agent{}) })
	return agent
}

func patchAgentSandboxSize(t *testing.T, h *handler.AgentHandler, org *model.Org, agentID uuid.UUID, size string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{"sandbox_size": size})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPatch, "/v1/agents/"+agentID.String(), bytes.NewReader(body))
	req = withChiURLParam(req, "id", agentID.String())
	req = middleware.WithOrg(req, org)
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	return rr
}

func patchAgentSandboxImage(t *testing.T, h *handler.AgentHandler, org *model.Org, agentID uuid.UUID, image string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{"sandbox_image": image})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPatch, "/v1/agents/"+agentID.String(), bytes.NewReader(body))
	req = withChiURLParam(req, "id", agentID.String())
	req = middleware.WithOrg(req, org)
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	return rr
}
