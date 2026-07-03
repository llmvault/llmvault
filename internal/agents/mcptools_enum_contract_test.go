package agents

import (
	"reflect"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

// --- schema / list drift pins ------------------------------------------------

func TestParentAssignableToolIDs_ExactContract(t *testing.T) {
	got := ParentAssignableToolIDs()
	want := []string{
		"lsp",
		"web_search",
		"web_fetch",
		"generate_image",
		"generate_vector_image",
		"remix_image",
		"search_memories",
		"retain_memory",
		"forget_memory",
		"search_knowledge_base",
		"cron",
		"create_http_trigger",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParentAssignableToolIDs() = %v, want %v", got, want)
	}
	if len(got) != 12 {
		t.Fatalf("parent enum length = %d, want 12", len(got))
	}
	// No baseline or read-only floor id may appear in the parent enum.
	for _, id := range got {
		if baselineRuntimeToolSet[id] {
			t.Fatalf("parent enum must not contain baseline tool %q", id)
		}
		if readOnlyMCPFloorSet[id] {
			t.Fatalf("parent enum must not contain read-only floor tool %q", id)
		}
		if id == "subagent_task" {
			t.Fatalf("parent enum must not contain subagent_task")
		}
	}
}

func TestSubAgentEnumIsFullUnion(t *testing.T) {
	sub := AssignableToolIDs()
	want := len(model.RuntimeBuiltInToolIDs) + len(AssignableMCPTools)
	if len(sub) != want {
		t.Fatalf("sub-agent enum length = %d, want %d", len(sub), want)
	}
	set := map[string]bool{}
	for _, id := range sub {
		set[id] = true
	}
	for _, id := range model.RuntimeBuiltInToolIDs {
		if !set[id] {
			t.Fatalf("sub-agent enum missing runtime tool %q", id)
		}
	}
	for _, id := range AssignableMCPTools {
		if !set[id] {
			t.Fatalf("sub-agent enum missing MCP tool %q", id)
		}
	}
}

func TestSharedConstantSubsets(t *testing.T) {
	// ReadOnlyMCPToolFloor must be a subset of AssignableMCPTools so every floor
	// tool is a real, assignable MCP tool.
	if !readOnlyMCPFloorSet["list_channels"] {
		t.Fatalf("sanity: floor set missing list_channels")
	}
	for _, floor := range model.ReadOnlyMCPToolFloor {
		if !assignableMCPToolSet[floor] {
			t.Fatalf("read-only floor tool %q must be in AssignableMCPTools", floor)
		}
	}
	// BaselineRuntimeToolIDs must be a subset of RuntimeBuiltInToolIDs.
	for _, id := range model.BaselineRuntimeToolIDs {
		if !model.IsValidRuntimeBuiltInToolID(id) {
			t.Fatalf("baseline runtime tool %q must be in RuntimeBuiltInToolIDs", id)
		}
	}
}
