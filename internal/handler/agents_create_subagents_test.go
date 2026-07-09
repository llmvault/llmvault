package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

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
	rr := postCreateAgent(t, db, h, &org, body)
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
	rr := postCreateAgent(t, db, h, &org, body)
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
	rr := postCreateAgent(t, db, h, &org, body)
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

	rr := postCreateAgent(t, db, h, &org, `{"name":"Blank Sub Parent","sub_agents":[{"name":"   "}]}`)
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
	rr := postCreateAgent(t, db, h, &org, body)
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

	create := postCreateAgent(t, db, h, &org, `{"name":"Team Lead","sub_agents":[{"name":"Aide"}]}`)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", create.Code, create.Body.String())
	}
	parentID := decodeCreateAgent(t, create).Agent.ID

	// List: only the parent, never the sub-agent. An org-scoped (API-key)
	// caller sees the whole org, isolating this assertion from actor-scoped
	// visibility.
	listReq := httptest.NewRequest(http.MethodGet, "/v1/agents?status=active&limit=100", nil)
	listReq = middleware.WithOrg(listReq, &org)
	listReq = middleware.WithAPIKeyClaims(listReq, &middleware.APIKeyClaims{OrgID: org.ID.String()})
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
	getReq = middleware.WithAPIKeyClaims(getReq, &middleware.APIKeyClaims{OrgID: org.ID.String()})
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
	rr := postCreateAgent(t, db, h, &org, body)
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
