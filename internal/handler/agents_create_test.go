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

// A full "team" payload: a parent with two scoped sub-agents persists three rows
// (one parent + two sub-agents) and returns them nested in the response.
func TestCreateAgent_WithSubAgentsPersistsRowsAndResponse(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	cleanupAgents(t, db, org.ID)
	seedDefaultModelCredential(t, db)
	h := newAgentHandlerForTest(db)

	body := `{
		"name": "Research Lead",
		"instructions": "Coordinate the team.",
		"sub_agents": [
			{"name": "Researcher", "instructions": "Find sources.", "tools": {"grep": true, "read_file": true}},
			{"name": "Writer", "instructions": "Draft output.", "tools": {"write_file": true}}
		]
	}`
	rr := postCreateAgent(t, h, &org, body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	resp := decodeCreateAgent(t, rr)

	if len(resp.Agent.SubAgents) != 2 {
		t.Fatalf("response sub_agents = %d, want 2", len(resp.Agent.SubAgents))
	}
	for _, sub := range resp.Agent.SubAgents {
		if sub.ParentAgentID != resp.Agent.ID {
			t.Fatalf("sub-agent %q parent = %q, want %q", sub.Name, sub.ParentAgentID, resp.Agent.ID)
		}
		if sub.Model != agentruntime.DefaultAgentModel {
			t.Fatalf("sub-agent %q model = %q, want inherited default", sub.Name, sub.Model)
		}
	}

	// The parent must be able to dispatch to its sub-agents.
	if resp.Agent.Tools["subagent_task"] != true {
		t.Fatalf("parent tools = %#v, want subagent_task enabled", resp.Agent.Tools)
	}

	parentID := uuid.MustParse(resp.Agent.ID)
	var subRows []model.Agent
	if err := db.Where("parent_agent_id = ? AND type = ?", parentID, model.AgentTypeSubAgent).
		Order("name ASC").Find(&subRows).Error; err != nil {
		t.Fatalf("load sub-agent rows: %v", err)
	}
	if len(subRows) != 2 {
		t.Fatalf("stored sub-agent rows = %d, want 2", len(subRows))
	}
	if subRows[0].Name != "Researcher" || subRows[1].Name != "Writer" {
		t.Fatalf("sub-agent names = %q,%q", subRows[0].Name, subRows[1].Name)
	}
	if _, ok := subRows[0].Tools["grep"]; !ok {
		t.Fatalf("Researcher tools = %#v, want grep", subRows[0].Tools)
	}
}

// subagent_task is injected even when the caller restricts the parent's tools.
func TestCreateAgent_EnablesSubagentTaskWhenSubAgentsPresent(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	cleanupAgents(t, db, org.ID)
	seedDefaultModelCredential(t, db)
	h := newAgentHandlerForTest(db)

	body := `{"name":"Restricted Parent","tools":{"read_file":true},"sub_agents":[{"name":"Helper"}]}`
	rr := postCreateAgent(t, h, &org, body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	resp := decodeCreateAgent(t, rr)
	if resp.Agent.Tools["read_file"] != true || resp.Agent.Tools["subagent_task"] != true {
		t.Fatalf("parent tools = %#v, want read_file + injected subagent_task", resp.Agent.Tools)
	}
}

func TestCreateAgent_RejectsDuplicateSubAgentNames(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	cleanupAgents(t, db, org.ID)
	seedDefaultModelCredential(t, db)
	h := newAgentHandlerForTest(db)

	body := `{"name":"Dup Parent","sub_agents":[{"name":"Twin"},{"name":"Twin"}]}`
	rr := postCreateAgent(t, h, &org, body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rr.Code, rr.Body.String())
	}
}

func TestCreateAgent_RejectsBlankSubAgentName(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	cleanupAgents(t, db, org.ID)
	seedDefaultModelCredential(t, db)
	h := newAgentHandlerForTest(db)

	rr := postCreateAgent(t, h, &org, `{"name":"Blank Sub Parent","sub_agents":[{"name":"   "}]}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rr.Code, rr.Body.String())
	}
}

func TestCreateAgent_RejectsUnknownSubAgentModel(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	cleanupAgents(t, db, org.ID)
	seedDefaultModelCredential(t, db)
	h := newAgentHandlerForTest(db)

	body := `{"name":"Bad Model Parent","sub_agents":[{"name":"Worker","model":"totally-made-up-model"}]}`
	rr := postCreateAgent(t, h, &org, body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rr.Code, rr.Body.String())
	}
}

// Sub-agent rows must never surface in the top-level agent list, and GET must
// return them nested under their parent.
func TestCreateAgent_SubAgentsExcludedFromListAndReturnedByGet(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	cleanupAgents(t, db, org.ID)
	seedDefaultModelCredential(t, db)
	h := newAgentHandlerForTest(db)

	create := postCreateAgent(t, h, &org, `{"name":"Team Lead","sub_agents":[{"name":"Aide"}]}`)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", create.Code, create.Body.String())
	}
	parentID := decodeCreateAgent(t, create).Agent.ID

	// List: only the parent, never the sub-agent.
	listReq := httptest.NewRequest(http.MethodGet, "/v1/agents?status=active&limit=100", nil)
	listReq = middleware.WithOrg(listReq, &org)
	listRR := httptest.NewRecorder()
	h.List(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", listRR.Code, listRR.Body.String())
	}
	var list struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.NewDecoder(listRR.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	for _, item := range list.Data {
		if item.Name == "Aide" {
			t.Fatalf("sub-agent Aide leaked into agent list: %#v", list.Data)
		}
	}

	// Get: sub-agents nested under the parent.
	getReq := httptest.NewRequest(http.MethodGet, "/v1/agents/"+parentID, nil)
	getReq = withChiURLParam(getReq, "id", parentID)
	getReq = middleware.WithOrg(getReq, &org)
	getRR := httptest.NewRecorder()
	h.Get(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", getRR.Code, getRR.Body.String())
	}
	var got createAgentResponse
	// Get returns the agent object directly (not wrapped in {"agent":...}).
	if err := json.NewDecoder(getRR.Body).Decode(&got.Agent); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if len(got.Agent.SubAgents) != 1 || got.Agent.SubAgents[0].Name != "Aide" {
		t.Fatalf("get sub_agents = %#v, want [Aide]", got.Agent.SubAgents)
	}
}

// An MCP tool filter on the agent and on a sub-agent is persisted (deny on the
// parent, allow on the sub-agent), normalized and de-duped.
func TestCreateAgent_PersistsMcpToolFilter(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	cleanupAgents(t, db, org.ID)
	seedDefaultModelCredential(t, db)
	h := newAgentHandlerForTest(db)

	body := `{
		"name": "Filtered Agent",
		"mcp_tool_filter": { "deny": ["generate_image", "generate_vector_image"] },
		"sub_agents": [
			{ "name": "Imager", "mcp_tool_filter": { "allow": ["generate_image"] } }
		]
	}`
	rr := postCreateAgent(t, h, &org, body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	parentID := decodeCreateAgent(t, rr).Agent.ID

	var parent model.Agent
	if err := db.First(&parent, "id = ?", parentID).Error; err != nil {
		t.Fatalf("load parent: %v", err)
	}
	if parent.McpToolFilter == nil ||
		len(parent.McpToolFilter.Deny) != 2 ||
		parent.McpToolFilter.Deny[0] != "generate_image" {
		t.Fatalf("parent mcp_tool_filter = %#v, want deny generate_image+vector", parent.McpToolFilter)
	}

	var sub model.Agent
	if err := db.Where("parent_agent_id = ? AND type = ?", parent.ID, model.AgentTypeSubAgent).
		First(&sub).Error; err != nil {
		t.Fatalf("load sub-agent: %v", err)
	}
	if sub.McpToolFilter == nil ||
		len(sub.McpToolFilter.Allow) != 1 ||
		sub.McpToolFilter.Allow[0] != "generate_image" {
		t.Fatalf("sub-agent mcp_tool_filter = %#v, want allow generate_image", sub.McpToolFilter)
	}
}

func TestCreateAgent_DuplicateNameConflict(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	cleanupAgents(t, db, org.ID)
	seedDefaultModelCredential(t, db)
	h := newAgentHandlerForTest(db)

	first := postCreateAgent(t, h, &org, `{"name":"Only One"}`)
	if first.Code != http.StatusCreated {
		t.Fatalf("first create status = %d, body = %s", first.Code, first.Body.String())
	}
	second := postCreateAgent(t, h, &org, `{"name":"Only One"}`)
	if second.Code != http.StatusConflict {
		t.Fatalf("second create status = %d, want 409; body = %s", second.Code, second.Body.String())
	}
}
