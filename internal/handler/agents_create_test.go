package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func TestIntegration_AgentsCreate_PersistsDefinitionWithoutSandbox(t *testing.T) {
	h := newAgentHarness(t)
	m := h.createOrg(t)
	h.seedSystemCred(t, "openrouter", false)

	body := validAgentBody()
	body["instructions"] = "Review incidents with production evidence before suggesting fixes."
	body["icon"] = "wrench"
	body["placeholder"] = "Ask the incident agent"
	body["sandbox_strategy"] = "per_session"
	body["sandbox_tools"] = []string{"chrome"}
	body["permissions"] = map[string]any{"bash": "allow"}
	body["tools"] = map[string]any{"bash": map[string]any{"enabled": true}}
	body["mcp_servers"] = []map[string]any{{"name": "docs", "transport": "streamable_http", "url": "https://mcp.example.test"}}

	rr := h.post(t, m, body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rr.Code, rr.Body.String())
	}
	var out struct {
		Agent struct {
			ID              string           `json:"id"`
			Name            string           `json:"name"`
			Instructions    string           `json:"instructions"`
			Icon            string           `json:"icon"`
			Placeholder     string           `json:"placeholder"`
			SandboxStrategy string           `json:"sandbox_strategy"`
			SandboxTools    []string         `json:"sandbox_tools"`
			Permissions     map[string]any   `json:"permissions"`
			Tools           map[string]any   `json:"tools"`
			McpServers      []map[string]any `json:"mcp_servers"`
			IsDefault       bool             `json:"is_default"`
			Sandbox         any              `json:"sandbox,omitempty"`
		} `json:"agent"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode create response: %v\n%s", err, rr.Body.String())
	}
	if out.Agent.ID == "" || out.Agent.Name != body["name"] {
		t.Fatalf("unexpected created agent response: %+v", out.Agent)
	}
	if out.Agent.Instructions != body["instructions"] {
		t.Fatalf("instructions = %q", out.Agent.Instructions)
	}
	if out.Agent.Icon != "wrench" || out.Agent.Placeholder != "Ask the incident agent" {
		t.Fatalf("display fields not persisted in response: %+v", out.Agent)
	}
	if out.Agent.SandboxStrategy != "per_session" || out.Agent.IsDefault {
		t.Fatalf("unexpected strategy/default: %+v", out.Agent)
	}
	if len(out.Agent.SandboxTools) != 1 || out.Agent.SandboxTools[0] != "chrome" {
		t.Fatalf("sandbox_tools = %#v", out.Agent.SandboxTools)
	}
	if _, ok := out.Agent.Permissions["bash"]; !ok {
		t.Fatalf("permissions missing bash: %#v", out.Agent.Permissions)
	}
	if len(out.Agent.McpServers) != 1 || out.Agent.McpServers[0]["name"] != "docs" {
		t.Fatalf("mcp_servers = %#v", out.Agent.McpServers)
	}

	agentID := uuid.MustParse(out.Agent.ID)
	var persisted model.Agent
	if err := h.db.Where("id = ? AND org_id = ?", agentID, m.org.ID).First(&persisted).Error; err != nil {
		t.Fatalf("load persisted agent: %v", err)
	}
	if persisted.Instructions == nil || *persisted.Instructions != body["instructions"] {
		t.Fatalf("persisted instructions = %#v", persisted.Instructions)
	}
	var sandboxCount int64
	if err := h.db.Model(&model.Sandbox{}).Where("agent_id = ?", agentID).Count(&sandboxCount).Error; err != nil {
		t.Fatalf("count sandboxes: %v", err)
	}
	if sandboxCount != 0 {
		t.Fatalf("create should not create sandbox, count=%d", sandboxCount)
	}
}

func TestIntegration_AgentsCreate_RequiresAdmin(t *testing.T) {
	h := newAgentHarness(t)
	m := h.createOrgWithRole(t, "member")
	h.seedSystemCred(t, "openrouter", false)

	rr := h.postAs(t, m, validAgentBody(), "member")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", rr.Code, rr.Body.String())
	}
}

func skillIDsFor(t *testing.T, db *gorm.DB, agentID uuid.UUID) map[uuid.UUID]bool {
	t.Helper()
	var rows []model.AgentSkill
	if err := db.Where("agent_id = ?", agentID).Find(&rows).Error; err != nil {
		t.Fatalf("load agent_skills for %v: %v", agentID, err)
	}
	out := make(map[uuid.UUID]bool, len(rows))
	for _, r := range rows {
		out[r.SkillID] = true
	}
	return out
}
