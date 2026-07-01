package agentruntime

import (
	"context"
	"testing"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

// A user-created agent's own mcp_tool_filter (and each sub-agent's) is applied at
// compile time: the parent denies generate_image while a sub-agent allows it.
func TestCompile_AppliesUserAgentAndSubAgentMcpFilter(t *testing.T) {
	db := connectCompileTestDB(t)
	org := createOrg(t, db)

	parent := userAgentRow(org.ID, "Filtered")
	parent.McpToolFilter = &model.ToolFilter{Deny: []string{"generate_image"}}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatalf("create parent: %v", err)
	}

	sub := userAgentRow(org.ID, "Imager")
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

	if def.McpToolFilter == nil || !containsString(def.McpToolFilter.Deny, "generate_image") {
		t.Fatalf("parent mcp filter = %#v, want deny generate_image", def.McpToolFilter)
	}
	subDef := def.SubAgents[sub.ID.String()]
	if subDef == nil {
		t.Fatalf("missing sub-agent def: %#v", def.SubAgents)
	}
	if subDef.McpToolFilter == nil || !containsString(subDef.McpToolFilter.Allow, "generate_image") {
		t.Fatalf("sub-agent mcp filter = %#v, want allow generate_image", subDef.McpToolFilter)
	}
}
