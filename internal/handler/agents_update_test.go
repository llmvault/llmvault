package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

func TestIntegration_AgentUpdate_PersistsDefinitionFields(t *testing.T) {
	h := newAgentHarness(t)
	m := h.createOrg(t)
	agent := h.seedAgentAgent(t, m)

	rr := h.patchAgent(t, m, agent.ID, map[string]any{
		"name":             "incident-agent-" + agent.ID.String()[:8],
		"description":      "Handles incident response.",
		"instructions":     "Confirm the blast radius before recommending mitigation.",
		"icon":             "shield",
		"placeholder":      "Ask about an incident",
		"sandbox_strategy": "per_session",
		"sandbox_tools":    []string{"chrome"},
		"permissions":      map[string]any{"bash": "allow"},
		"tools":            map[string]any{"bash": map[string]any{"enabled": true}},
		"mcp_servers":      []map[string]any{{"name": "incident_docs", "transport": "streamable_http", "url": "https://mcp.example.test"}},
	}, "admin")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}

	var out struct {
		Agent struct {
			Name            string           `json:"name"`
			Description     string           `json:"description"`
			Instructions    string           `json:"instructions"`
			Icon            string           `json:"icon"`
			Placeholder     string           `json:"placeholder"`
			SandboxStrategy string           `json:"sandbox_strategy"`
			SandboxTools    []string         `json:"sandbox_tools"`
			Permissions     map[string]any   `json:"permissions"`
			Tools           map[string]any   `json:"tools"`
			McpServers      []map[string]any `json:"mcp_servers"`
		} `json:"agent"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode update response: %v\n%s", err, rr.Body.String())
	}
	if out.Agent.Instructions != "Confirm the blast radius before recommending mitigation." {
		t.Fatalf("instructions = %q", out.Agent.Instructions)
	}
	if out.Agent.Icon != "shield" || out.Agent.Placeholder != "Ask about an incident" {
		t.Fatalf("display fields = %#v", out.Agent)
	}
	if out.Agent.SandboxStrategy != "per_session" {
		t.Fatalf("sandbox_strategy = %q", out.Agent.SandboxStrategy)
	}
	if len(out.Agent.SandboxTools) != 1 || out.Agent.SandboxTools[0] != "chrome" {
		t.Fatalf("sandbox_tools = %#v", out.Agent.SandboxTools)
	}
	if _, ok := out.Agent.Permissions["bash"]; !ok {
		t.Fatalf("permissions missing bash: %#v", out.Agent.Permissions)
	}
	if len(out.Agent.McpServers) != 1 || out.Agent.McpServers[0]["name"] != "incident_docs" {
		t.Fatalf("mcp_servers = %#v", out.Agent.McpServers)
	}

	var persisted model.Agent
	if err := h.db.Where("id = ? AND org_id = ?", agent.ID, m.org.ID).First(&persisted).Error; err != nil {
		t.Fatalf("load persisted agent: %v", err)
	}
	if persisted.Instructions == nil || *persisted.Instructions != out.Agent.Instructions {
		t.Fatalf("persisted instructions = %#v", persisted.Instructions)
	}
	if persisted.Icon != out.Agent.Icon || persisted.Placeholder != out.Agent.Placeholder {
		t.Fatalf("persisted display fields = icon:%q placeholder:%q", persisted.Icon, persisted.Placeholder)
	}
	if len(persisted.SandboxTools) != 1 || persisted.SandboxTools[0] != "chrome" {
		t.Fatalf("persisted sandbox_tools = %#v", persisted.SandboxTools)
	}
}

func TestIntegration_AgentUpdate_RequiresAdmin(t *testing.T) {
	h := newAgentHarness(t)
	m := h.createOrgWithRole(t, "member")
	agent := h.seedAgentAgent(t, m)

	rr := h.patchAgent(t, m, agent.ID, map[string]any{"name": "member-update"}, "member")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_AgentArchive_GuardrailsAndSuccess(t *testing.T) {
	h := newAgentHarness(t)
	m := h.createOrg(t)

	defaultAgent := h.seedAgentAgent(t, m)
	if err := h.db.Model(&model.Agent{}).
		Where("id = ?", defaultAgent.ID).
		Updates(map[string]any{"is_default": true, "sandbox_strategy": "always_on"}).Error; err != nil {
		t.Fatalf("mark default agent: %v", err)
	}
	rr := h.deleteAgent(t, m, defaultAgent.ID, "admin")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("default archive status = %d, want 400: %s", rr.Code, rr.Body.String())
	}

	agent := h.seedAgentAgent(t, m)
	sandbox := h.seedSandbox(t, m, agent.ID)
	session := seedAgentSession(t, h, m.org.ID, agent.ID, sandbox.ID, "slack", "Incident thread", sandbox.CreatedAt)
	rr = h.deleteAgent(t, m, agent.ID, "admin")
	if rr.Code != http.StatusConflict {
		t.Fatalf("active session archive status = %d, want 409: %s", rr.Code, rr.Body.String())
	}

	if err := h.db.Model(&model.AgentSession{}).
		Where("id = ?", session.ID).
		Update("status", "ended").Error; err != nil {
		t.Fatalf("end active session: %v", err)
	}
	rr = h.deleteAgent(t, m, agent.ID, "admin")
	if rr.Code != http.StatusOK {
		t.Fatalf("archive status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var persisted model.Agent
	if err := h.db.Where("id = ?", agent.ID).First(&persisted).Error; err != nil {
		t.Fatalf("load archived agent: %v", err)
	}
	if persisted.Status != "archived" {
		t.Fatalf("status = %q, want archived", persisted.Status)
	}
}
