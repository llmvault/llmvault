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
	deps := Deps{DB: db, DefaultModel: "deepseek-v4-flash"}
	ctx := context.Background()

	agent, err := CreateAgent(ctx, deps, org.ID, CreateInput{
		Name:          "Builder",
		Instructions:  "coordinate work",
		Tools:         model.JSON{"bash": true},
		McpToolFilter: &model.ToolFilter{Allow: []string{"web_search"}},
		SubAgents:     []SubAgentInput{{Name: "Helper", Tools: model.JSON{"read_file": true}}},
	})
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	token := &model.Token{OrgID: org.ID}

	// list_agents returns the top-level agent, not the sub-agent.
	listRes, _ := handleListAgents(ctx, db, token)
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
	getRes, _ := handleGetAgent(ctx, db, token, "https://app.test", getAgentArgs{AgentID: agent.ID.String()})
	if getRes.IsError {
		t.Fatalf("get_agent error: %s", errResultText(getRes))
	}
	getText := errResultText(getRes)
	for _, want := range []string{"coordinate work", "bash", "web_search", "Helper", "read_file"} {
		if !strings.Contains(getText, want) {
			t.Fatalf("get_agent output missing %q, got: %s", want, getText)
		}
	}

	// Unknown id → helpful not-found error.
	nf, _ := handleGetAgent(ctx, db, token, "", getAgentArgs{AgentID: "not-a-uuid"})
	if nf == nil || !nf.IsError {
		t.Fatalf("expected error for invalid agent_id")
	}

	// Org scoping: another org cannot fetch this agent.
	otherToken := &model.Token{OrgID: testOrg(t, db).ID}
	scoped, _ := handleGetAgent(ctx, db, otherToken, "", getAgentArgs{AgentID: agent.ID.String()})
	if scoped == nil || !scoped.IsError || !strings.Contains(errResultText(scoped), "not found") {
		t.Fatalf("cross-org get_agent should be not found, got: %#v", scoped)
	}
}
