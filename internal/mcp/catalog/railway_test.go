package catalog

import "testing"

func TestRailwayActionsAreExposedToMCP(t *testing.T) {
	t.Parallel()

	railway, ok := Global().GetProvider("railway")
	if !ok {
		t.Fatal("Railway action catalog is missing")
	}
	if !railway.ShouldPushToMCP() {
		t.Fatal("Railway actions are disabled for MCP")
	}
	if len(railway.Actions) == 0 {
		t.Fatal("Railway has no executable actions")
	}
}
