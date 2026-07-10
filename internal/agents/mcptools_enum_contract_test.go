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
		"search_knowledge_base",
		"cron",
		"create_http_trigger",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParentAssignableToolIDs() = %v, want %v", got, want)
	}
	if len(got) != 9 {
		t.Fatalf("parent enum length = %d, want 9", len(got))
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

// TestValidBuiltInToolsMatchesRuntimeAndMCP pins model.ValidBuiltInTools to its
// two ground-truth sources so drift shows up as a deliberate test edit:
//   - the runtime-native tools (model.RuntimeBuiltInToolIDs, itself the Go mirror
//     of the ToolSpec enum in sandboxes/runtime/crates/domain/src/tool_specs.rs),
//   - the curated grantable Hivy MCP tools (AssignableMCPTools).
//
// ValidBuiltInTools must contain exactly the union of those two sets: every id an
// agent can be granted, and nothing fictional.
func TestValidBuiltInToolsMatchesRuntimeAndMCP(t *testing.T) {
	want := map[string]bool{}
	for _, id := range model.RuntimeBuiltInToolIDs {
		want[id] = true
	}
	for _, id := range AssignableMCPTools {
		want[id] = true
	}

	got := map[string]bool{}
	for _, id := range model.BuiltInToolIDs() {
		if got[id] {
			t.Fatalf("ValidBuiltInTools has duplicate id %q", id)
		}
		got[id] = true
	}

	if len(got) != len(want) {
		t.Fatalf("ValidBuiltInTools has %d ids, want %d (RuntimeBuiltInToolIDs ∪ AssignableMCPTools)", len(got), len(want))
	}
	for id := range want {
		if !got[id] {
			t.Fatalf("ValidBuiltInTools is missing id %q", id)
		}
	}
	for id := range got {
		if !want[id] {
			t.Fatalf("ValidBuiltInTools has stale id %q not in RuntimeBuiltInToolIDs or AssignableMCPTools", id)
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
