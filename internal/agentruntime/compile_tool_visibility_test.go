package agentruntime

import (
	"context"
	"testing"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

// compiledRuntimeToolTypes extracts the "type" of each compiled runtime tool
// (e.g. "builtin.bash") from an AgentDefinition.Tools array.
func compiledRuntimeToolTypes(tools []map[string]any) []string {
	out := make([]string, 0, len(tools))
	for _, tool := range tools {
		if value, ok := tool["type"].(string); ok {
			out = append(out, value)
		}
	}
	return out
}

// TestCompile_ToolAndSubAgentVisibilityFiltersEndToEnd exercises all three
// tool-visibility layers through the real DB + real compile path and proves
// they are independent:
//
//  1. Agent builtin-tool selection: only enabled tools reach the compiled
//     config; disabled tools are absent (the runtime only builds what is in the
//     Tools array, so absence == not visible to the agent).
//  2. MCP tool filter (allow/deny) rides on the parent definition.
//  3. A sub-agent's tools and MCP filter are its own: a tool enabled on the
//     parent is NOT visible to the sub-agent unless the sub-agent enables it,
//     and vice versa; the sub-agent's MCP filter is independent of the parent's.
func TestCompile_ToolAndSubAgentVisibilityFiltersEndToEnd(t *testing.T) {
	db := connectCompileTestDB(t)
	org := createOrg(t, db)
	team := seedCompileTeam(t, db, org.ID)

	// Parent: only bash, read_file, grep enabled; deny the web_search MCP tool.
	parent := userAgentRow(org.ID, team.ID, "RestrictedParent")
	parent.Tools = model.JSON{"bash": true, "read_file": true, "grep": true}
	parent.McpToolFilter = &model.ToolFilter{Deny: []string{"web_search"}}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatalf("create parent: %v", err)
	}

	// Sub-agent: a DIFFERENT set (read_file, write_file) and the OPPOSITE MCP
	// filter (allow web_search). write_file is enabled here but not on the
	// parent; grep is enabled on the parent but not here.
	sub := userAgentRow(org.ID, team.ID, "RestrictedSub")
	sub.Type = model.AgentTypeSubAgent
	sub.ParentAgentID = &parent.ID
	sub.Tools = model.JSON{"read_file": true, "write_file": true}
	sub.McpToolFilter = &model.ToolFilter{Allow: []string{"web_search"}}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatalf("create sub-agent: %v", err)
	}

	def, err := Compile(context.Background(), CompileDeps{DB: db, Cfg: &config.Config{}}, &parent)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	// --- Layer 1: parent builtin tools ---
	parentTools := compiledRuntimeToolTypes(def.Tools)
	for _, want := range []string{"builtin.bash", "builtin.read_file", "builtin.grep"} {
		if !containsString(parentTools, want) {
			t.Fatalf("parent tools %v missing enabled tool %q", parentTools, want)
		}
	}
	for _, denied := range []string{"builtin.write_file", "builtin.glob", "builtin.lsp", "builtin.multi_grep"} {
		if containsString(parentTools, denied) {
			t.Fatalf("parent tools %v leaked disabled tool %q", parentTools, denied)
		}
	}

	// --- Layer 2: parent MCP filter ---
	if def.McpToolFilter == nil || !containsString(def.McpToolFilter.Deny, "web_search") {
		t.Fatalf("parent mcp filter = %#v, want deny web_search", def.McpToolFilter)
	}
	if len(def.McpToolFilter.Allow) != 0 {
		t.Fatalf("parent mcp filter should not have an allow list: %#v", def.McpToolFilter)
	}

	// --- Layer 3: sub-agent independence ---
	subDef := def.SubAgents[sub.ID.String()]
	if subDef == nil {
		t.Fatalf("missing sub-agent def in %#v", def.SubAgents)
	}
	subTools := compiledRuntimeToolTypes(subDef.Tools)
	for _, want := range []string{"builtin.read_file", "builtin.write_file"} {
		if !containsString(subTools, want) {
			t.Fatalf("sub-agent tools %v missing enabled tool %q", subTools, want)
		}
	}
	// Parent-only tools must NOT be visible to the sub-agent.
	for _, notInSub := range []string{"builtin.bash", "builtin.grep"} {
		if containsString(subTools, notInSub) {
			t.Fatalf("sub-agent tools %v leaked parent-only tool %q", subTools, notInSub)
		}
	}
	// Sub-agent-only tools must NOT be visible to the parent.
	if containsString(parentTools, "builtin.write_file") {
		t.Fatalf("parent tools %v leaked sub-agent-only tool builtin.write_file", parentTools)
	}

	// Sub-agent MCP filter is independent of (and opposite to) the parent's.
	if subDef.McpToolFilter == nil || !containsString(subDef.McpToolFilter.Allow, "web_search") {
		t.Fatalf("sub-agent mcp filter = %#v, want allow web_search", subDef.McpToolFilter)
	}
	if len(subDef.McpToolFilter.Deny) != 0 {
		t.Fatalf("sub-agent mcp filter should not have a deny list: %#v", subDef.McpToolFilter)
	}
}
