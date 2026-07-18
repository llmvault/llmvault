package mcpserver

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/model"
)

func TestBuildServerWithNoScopes(t *testing.T) {
	server, err := BuildServer(
		context.Background(),
		&model.Token{Meta: model.JSON{model.TokenMetaType: model.TokenTypeAgentProxy}},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("build server: %v", err)
	}
	if server == nil {
		t.Fatal("server is nil")
	}
}

// This exercises the MCP protocol boundary rather than the runtime's second
// filter: an ungranted tool must be absent from tools/list before the JTI's
// server instance enters ServerCache.
func TestBuildServerFiltersNativeToolsBeforeDiscovery(t *testing.T) {
	register := func(server *mcp.Server, _ *model.Token) {
		for _, name := range []string{"web_search", "web_fetch", "sheet_list", "app_create", "skill_view"} {
			server.AddTool(&mcp.Tool{
				Name:        name,
				Description: name,
				InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
			}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return &mcp.CallToolResult{}, nil
			})
		}
	}
	filter := &model.ToolFilter{Allow: []string{"sheet_list"}}
	server, err := BuildServer(context.Background(), &model.Token{}, nil, nil, nil, nil, nil, nil, nil, nil, register, nil, nil, filter)
	if err != nil {
		t.Fatalf("build server: %v", err)
	}
	names := listServerToolNames(t, server)
	if !names["sheet_list"] || !names["skill_view"] {
		t.Fatalf("tools/list = %v, want allowed sheet_list plus universal skill_view", names)
	}
	for _, ungranted := range []string{"web_fetch", "web_search", "app_create"} {
		if names[ungranted] {
			t.Fatalf("tools/list leaked ungranted %s: %v", ungranted, names)
		}
	}
}
