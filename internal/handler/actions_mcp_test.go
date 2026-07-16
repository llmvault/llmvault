package handler

import (
	"encoding/json"
	"testing"

	"github.com/usehivy/hivy/internal/mcp/catalog"
)

func TestActionsFromProviderMarksOnlyExecutableMCPActions(t *testing.T) {
	push := true
	provider := &catalog.ProviderActions{
		PushToMCP: &push,
		Actions: map[string]catalog.ActionDef{
			"executable":    {Execution: &catalog.ExecutionConfig{Method: "GET", Path: "/items"}},
			"metadata_only": {Parameters: json.RawMessage(`{"type":"object"}`)},
		},
	}
	actions := actionsFromProvider(provider)
	if len(actions) != 2 || actions[0].Key != "executable" || !actions[0].MCPEnabled {
		t.Fatalf("executable action = %#v, want mcp_enabled", actions)
	}
	if actions[1].Key != "metadata_only" || actions[1].MCPEnabled {
		t.Fatalf("metadata-only action = %#v, want MCP disabled", actions[1])
	}

	push = false
	actions = actionsFromProvider(provider)
	if actions[0].MCPEnabled {
		t.Fatalf("provider with push_to_mcp=false exposed action: %#v", actions[0])
	}
}
