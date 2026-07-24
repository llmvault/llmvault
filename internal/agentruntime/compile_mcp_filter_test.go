package agentruntime

import (
	"context"
	"testing"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

// A user-created agent's own mcp_tool_filter (and each sub-agent's) is applied
// at compile time as an explicit allow-list. A legacy deny-only filter must not
// retain the runtime's historical allow-all behavior.
func TestCompile_AppliesUserAgentAndSubAgentMcpFilter(t *testing.T) {
	db := connectCompileTestDB(t)
	org := createOrg(t, db)
	team := seedCompileTeam(t, db, org.ID)

	parent := userAgentRow(org.ID, team.ID, "Filtered")
	parent.McpToolFilter = &model.ToolFilter{Deny: []string{"generate_image"}}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatalf("create parent: %v", err)
	}

	sub := userAgentRow(org.ID, team.ID, "Imager")
	sub.Type = model.AgentTypeSubAgent
	sub.ParentAgentID = &parent.ID
	sub.McpToolFilter = &model.ToolFilter{Allow: []string{"generate_image"}}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatalf("create sub-agent: %v", err)
	}

	def, err := Compile(context.Background(), CompileDeps{DB: db, Cfg: &config.Config{}}, &parent)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	if def.McpToolFilter == nil || containsString(def.McpToolFilter.Allow, "generate_image") {
		t.Fatalf("parent mcp filter = %#v, want no optional MCP grant", def.McpToolFilter)
	}
	subDef := def.SubAgents[sub.ID.String()]
	if subDef == nil {
		t.Fatalf("missing sub-agent def: %#v", def.SubAgents)
	}
	if subDef.McpToolFilter == nil || !containsString(subDef.McpToolFilter.Allow, "generate_image") {
		t.Fatalf("sub-agent mcp filter = %#v, want allow generate_image", subDef.McpToolFilter)
	}
}

func TestCompile_DeniesMCPToolsWithoutExplicitFilter(t *testing.T) {
	db := connectCompileTestDB(t)
	org := createOrg(t, db)
	team := seedCompileTeam(t, db, org.ID)

	parent := userAgentRow(org.ID, team.ID, "No MCP grants")
	if err := db.Create(&parent).Error; err != nil {
		t.Fatalf("create parent: %v", err)
	}

	sub := userAgentRow(org.ID, team.ID, "No subagent MCP grants")
	sub.Type = model.AgentTypeSubAgent
	sub.ParentAgentID = &parent.ID
	if err := db.Create(&sub).Error; err != nil {
		t.Fatalf("create sub-agent: %v", err)
	}

	def, err := Compile(context.Background(), CompileDeps{DB: db, Cfg: &config.Config{}}, &parent)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	assertMCPToolsDeniedByDefault(t, def.McpToolFilter, model.BaselineParentMCPToolIDs)

	subDef := def.SubAgents[sub.ID.String()]
	if subDef == nil {
		t.Fatalf("missing sub-agent def: %#v", def.SubAgents)
	}
	assertMCPToolsDeniedByDefault(t, subDef.McpToolFilter, model.SubAgentReadOnlyMCPToolFloor)
}

func assertMCPToolsDeniedByDefault(t *testing.T, filter *model.ToolFilter, wantFloor []string) {
	t.Helper()
	if filter == nil {
		t.Fatal("mcp tool filter = nil, which grants every MCP tool")
	}
	if filter.Allow == nil {
		t.Fatalf("mcp allow list = nil, want explicit allow list: %#v", filter)
	}
	if len(filter.Allow) != len(wantFloor) {
		t.Fatalf("mcp allow list = %#v, want only universal MCP tools", filter.Allow)
	}
	for _, id := range wantFloor {
		if !containsString(filter.Allow, id) {
			t.Fatalf("mcp allow list = %#v, want universal tool %q", filter.Allow, id)
		}
	}
}
