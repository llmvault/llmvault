package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

// --- response decoding helpers ------------------------------------------------

type createAgentResponse struct {
	Agent struct {
		ID        string             `json:"id"`
		Name      string             `json:"name"`
		Model     string             `json:"model"`
		Tools     map[string]any     `json:"tools"`
		SubAgents []subAgentRespJSON `json:"sub_agents"`
	} `json:"agent"`
}

type subAgentRespJSON struct {
	ID            string         `json:"id"`
	ParentAgentID string         `json:"parent_agent_id"`
	Name          string         `json:"name"`
	Model         string         `json:"model"`
	Instructions  string         `json:"instructions"`
	Tools         map[string]any `json:"tools"`
	Status        string         `json:"status"`
}

func newAgentHandlerForTest(db *gorm.DB) *handler.AgentHandler {
	return handler.NewAgentHandler(db, nil, agentruntime.CompileDeps{}, registry.Global())
}

func postCreateAgent(t *testing.T, h *handler.AgentHandler, org *model.Org, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/agents", strings.NewReader(body))
	req = middleware.WithOrg(req, org)
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	return rr
}

func decodeCreateAgent(t *testing.T, rr *httptest.ResponseRecorder) createAgentResponse {
	t.Helper()
	var resp createAgentResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, rr.Body.String())
	}
	return resp
}

func cleanupAgents(t *testing.T, db *gorm.DB, orgID uuid.UUID) {
	t.Helper()
	t.Cleanup(func() { db.Where("org_id = ?", orgID).Delete(&model.Agent{}) })
}

// --- tests --------------------------------------------------------------------

// A minimal payload defaults to the full runtime toolset so the agent is usable.
func TestCreateAgent_MinimalDefaultsFullToolset(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	cleanupAgents(t, db, org.ID)
	seedDefaultModelCredential(t, db)
	h := newAgentHandlerForTest(db)

	rr := postCreateAgent(t, h, &org, `{"name":"Minimal Agent"}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	resp := decodeCreateAgent(t, rr)

	if len(resp.Agent.Tools) != len(model.RuntimeBuiltInToolIDs) {
		t.Fatalf("tools = %d keys, want full runtime toolset (%d)", len(resp.Agent.Tools), len(model.RuntimeBuiltInToolIDs))
	}
	if resp.Agent.Model != agentruntime.DefaultAgentModel {
		t.Fatalf("model = %q, want default", resp.Agent.Model)
	}
	if resp.Agent.SubAgents == nil {
		t.Fatalf("sub_agents should be an empty array, got null")
	}
	if len(resp.Agent.SubAgents) != 0 {
		t.Fatalf("sub_agents = %d, want 0", len(resp.Agent.SubAgents))
	}

	var stored model.Agent
	if err := db.First(&stored, "id = ?", resp.Agent.ID).Error; err != nil {
		t.Fatalf("load stored agent: %v", err)
	}
	if stored.Type != model.AgentTypeAgent {
		t.Fatalf("type = %q, want agent", stored.Type)
	}
	if stored.ParentAgentID != nil {
		t.Fatalf("parent_agent_id = %v, want nil", stored.ParentAgentID)
	}
}

// An explicit tools map restricts the agent to exactly those runtime tools.
func TestCreateAgent_RestrictsToolsToRequested(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	cleanupAgents(t, db, org.ID)
	seedDefaultModelCredential(t, db)
	h := newAgentHandlerForTest(db)

	rr := postCreateAgent(t, h, &org, `{"name":"Scoped Agent","tools":{"read_file":true,"grep":true}}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	resp := decodeCreateAgent(t, rr)

	if len(resp.Agent.Tools) != 2 || resp.Agent.Tools["read_file"] != true || resp.Agent.Tools["grep"] != true {
		t.Fatalf("tools = %#v, want exactly read_file+grep", resp.Agent.Tools)
	}

	var stored model.Agent
	if err := db.First(&stored, "id = ?", resp.Agent.ID).Error; err != nil {
		t.Fatalf("load stored agent: %v", err)
	}
	if len(stored.Tools) != 2 {
		t.Fatalf("stored tools = %#v, want 2 keys", stored.Tools)
	}
}

func TestCreateAgent_RejectsInvalidToolKey(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	cleanupAgents(t, db, org.ID)
	seedDefaultModelCredential(t, db)
	h := newAgentHandlerForTest(db)

	rr := postCreateAgent(t, h, &org, `{"name":"Bad Tool Agent","tools":{"not_a_real_tool":true}}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rr.Code, rr.Body.String())
	}
}

// Duplicate top-level agent names are allowed within an org (each team's Hivy
// shares the name), so two creates with the same name both succeed.
func TestCreateAgent_DuplicateNameAllowed(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	cleanupAgents(t, db, org.ID)
	seedDefaultModelCredential(t, db)
	h := newAgentHandlerForTest(db)

	first := postCreateAgent(t, h, &org, `{"name":"Shared Name"}`)
	if first.Code != http.StatusCreated {
		t.Fatalf("first create status = %d, body = %s", first.Code, first.Body.String())
	}
	second := postCreateAgent(t, h, &org, `{"name":"Shared Name"}`)
	if second.Code != http.StatusCreated {
		t.Fatalf("second create status = %d, want 201; body = %s", second.Code, second.Body.String())
	}
	firstResp := decodeCreateAgent(t, first)
	secondResp := decodeCreateAgent(t, second)
	if firstResp.Agent.ID == secondResp.Agent.ID {
		t.Fatalf("expected distinct agent IDs, got %s twice", firstResp.Agent.ID)
	}
	if firstResp.Agent.Name != "Shared Name" || secondResp.Agent.Name != "Shared Name" {
		t.Fatalf("names = %q, %q; want both %q (no suffix)", firstResp.Agent.Name, secondResp.Agent.Name, "Shared Name")
	}
}
