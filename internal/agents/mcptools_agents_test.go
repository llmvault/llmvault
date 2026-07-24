package agents

import (
	"context"
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

func TestListAndGetAgent(t *testing.T) {
	db := testDB(t)
	org := testOrg(t, db)
	team := testTeam(t, db, org.ID)
	deps := Deps{DB: db, DefaultModel: "deepseek-v4-flash"}
	ctx := context.Background()

	agent, err := CreateAgent(ctx, deps, org.ID, CreateInput{
		Name:          "Builder",
		Instructions:  "coordinate work",
		Tools:         model.JSON{"bash": true},
		McpToolFilter: &model.ToolFilter{Allow: []string{"web_search"}},
		TeamID:        team.ID,
		SubAgents:     []SubAgentInput{{Name: "Helper", Tools: model.JSON{"read_file": true}}},
	})
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	token := &model.Token{OrgID: org.ID}

	// list_agents returns the top-level agent, not the sub-agent.
	listRes, _ := handleListAgents(ctx, db, token, team.ID)
	if listRes.IsError {
		t.Fatalf("list_agents error: %s", errResultText(listRes))
	}
	listText := errResultText(listRes)
	if !strings.Contains(listText, "Builder") || !strings.Contains(listText, agent.ID.String()) {
		t.Fatalf("list should contain the agent, got: %s", listText)
	}
	if strings.Contains(listText, "Helper") {
		t.Fatalf("list must not include sub-agents, got: %s", listText)
	}

	// get_agent returns full config incl. instructions, tools (runtime+mcp), sub-agents.
	getRes, _ := handleGetAgent(ctx, db, token, team.ID, "https://app.test", getAgentArgs{AgentID: agent.ID.String()})
	if getRes.IsError {
		t.Fatalf("get_agent error: %s", errResultText(getRes))
	}
	getText := errResultText(getRes)
	// The parent reports its instructions, the parent-assignable MCP grant
	// (web_search), and its sub-agent (Helper + its read_file tool). Baseline
	// runtime tools like bash are auto-granted and intentionally NOT echoed for a
	// parent so the list round-trips through the parent tools schema.
	for _, want := range []string{"coordinate work", "web_search", "Helper", "read_file"} {
		if !strings.Contains(getText, want) {
			t.Fatalf("get_agent output missing %q, got: %s", want, getText)
		}
	}
	// The parent's own tools array must not surface baseline tools.
	getObj := builderResultJSON(t, getRes)["agent"].(map[string]any)
	if parentTools := agentToolStrings(t, getObj, "tools"); containsString(parentTools, "bash") {
		t.Fatalf("parent tools must not echo baseline bash: %v", parentTools)
	}

	// Unknown id → helpful not-found error.
	nf, _ := handleGetAgent(ctx, db, token, team.ID, "", getAgentArgs{AgentID: "not-a-uuid"})
	if nf == nil || !nf.IsError {
		t.Fatalf("expected error for invalid agent_id")
	}

	// Org scoping: another org cannot fetch this agent.
	otherToken := &model.Token{OrgID: testOrg(t, db).ID}
	scoped, _ := handleGetAgent(ctx, db, otherToken, team.ID, "", getAgentArgs{AgentID: agent.ID.String()})
	if scoped == nil || !scoped.IsError || !strings.Contains(errResultText(scoped), "not found") {
		t.Fatalf("cross-org get_agent should be not found, got: %#v", scoped)
	}
}
