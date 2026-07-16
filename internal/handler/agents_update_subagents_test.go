package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func patchAgent(t *testing.T, h interface {
	Update(http.ResponseWriter, *http.Request)
}, org *model.Org, id, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/v1/agents/"+id, strings.NewReader(body))
	req = withChiURLParam(req, "id", id)
	req = middleware.WithOrg(req, org)
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	return rr
}

// Editing a user agent replaces its whole sub-agent set and re-persists the MCP
// tool filter (verifying the jsonb serializer works through a map-based Update).
func TestUpdateAgent_ReplacesSubAgentsAndMcpFilter(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	cleanupAgents(t, db, org.ID)
	seedDefaultModelCredential(t, db)
	h := newAgentHandlerForTest(db)

	create := postCreateAgent(t, db, h, &org, `{
		"name": "Editable",
		"mcp_tool_filter": { "deny": ["generate_image"] },
		"sub_agents": [{ "name": "Old", "tools": { "read_file": true } }]
	}`)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", create.Code, create.Body.String())
	}
	id := decodeCreateAgent(t, create).Agent.ID
	pluginID := uuid.New()

	patch := patchAgent(t, h, &org, id, `{
		"name": "Edited",
		"mcp_tool_filter": { "deny": ["generate_vector_image", "web_search"] },
		"plugin_mcp_tool_deny": { "`+pluginID.String()+`": [" chat_delete ", "chat_delete", "reactions_remove"] },
		"sub_agents": [
			{ "name": "NewA", "tools": { "bash": true } },
			{ "name": "NewB", "tools": { "write_file": true } }
		]
	}`)
	if patch.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", patch.Code, patch.Body.String())
	}

	var agent model.Agent
	if err := db.First(&agent, "id = ?", id).Error; err != nil {
		t.Fatalf("load agent: %v", err)
	}
	if agent.Name != "Edited" {
		t.Fatalf("name = %q, want Edited", agent.Name)
	}
	if agent.McpToolFilter == nil || len(agent.McpToolFilter.Deny) != 2 ||
		agent.McpToolFilter.Deny[0] != "generate_vector_image" {
		t.Fatalf("mcp_tool_filter = %#v, want deny vector+web_search", agent.McpToolFilter)
	}
	pluginDeny := agent.PluginMCPToolDeny[pluginID.String()]
	if len(pluginDeny) != 2 || pluginDeny[0] != "chat_delete" || pluginDeny[1] != "reactions_remove" {
		t.Fatalf("plugin_mcp_tool_deny = %#v, want normalized Slack denies", agent.PluginMCPToolDeny)
	}

	var subs []model.Agent
	if err := db.Where("parent_agent_id = ? AND type = ?", agent.ID, model.AgentTypeSubAgent).
		Order("name ASC").Find(&subs).Error; err != nil {
		t.Fatalf("load sub-agents: %v", err)
	}
	if len(subs) != 2 || subs[0].Name != "NewA" || subs[1].Name != "NewB" {
		t.Fatalf("sub-agents = %#v, want NewA+NewB (Old removed)", subs)
	}
}

func TestUpdateAgentRejectsNonUUIDPluginMCPToolDenyKey(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	cleanupAgents(t, db, org.ID)
	seedDefaultModelCredential(t, db)
	h := newAgentHandlerForTest(db)

	create := postCreateAgent(t, db, h, &org, `{"name":"Plugin policy"}`)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", create.Code, create.Body.String())
	}
	id := decodeCreateAgent(t, create).Agent.ID
	patch := patchAgent(t, h, &org, id, `{"plugin_mcp_tool_deny":{"slack":["chat_delete"]}}`)
	if patch.Code != http.StatusBadRequest {
		t.Fatalf("update status = %d, want 400; body = %s", patch.Code, patch.Body.String())
	}
}
