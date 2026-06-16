package hindsight

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// MemoryRefreshFunc is called after a destructive memory change so agent
// runtimes can reload their precomputed memory context.
type MemoryRefreshFunc func(ctx context.Context, agent *model.Agent)

// NewMemoryToolsFunc returns a callback compatible with mcpserver.MemoryToolsFunc.
// Designed to be passed to mcpserver.BuildServer to avoid import cycles.
func NewMemoryToolsFunc(client *Client, refreshFns ...MemoryRefreshFunc) func(server *mcp.Server, agentID string, db *gorm.DB) {
	var refresh MemoryRefreshFunc
	if len(refreshFns) > 0 {
		refresh = refreshFns[0]
	}
	return func(server *mcp.Server, agentID string, db *gorm.DB) {
		var agent model.Agent
		if err := db.Where("id = ?", agentID).First(&agent).Error; err != nil {
			return
		}
		AddMemoryTools(server, &agent, client, db, refresh)
	}
}

// AddMemoryTools registers memory tools on an existing MCP server. Memory is
// scoped per org.
func AddMemoryTools(server *mcp.Server, agent *model.Agent, client *Client, db *gorm.DB, refresh MemoryRefreshFunc) {
	if agent.OrgID == nil || client == nil {
		return
	}
	bankID := OrgBankID(*agent.OrgID)
	banks := NewBankProvisioner(db, client)

	addRecallTool(server, agent, client, db, bankID)
	addRetainTool(server, agent, client, db, banks, bankID, refresh)
	addForgetTool(server, agent, client, db, bankID, refresh)
	addReflectTool(server, agent, client, db, bankID)
}

func toolError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: %s", msg)}},
		IsError: true,
	}
}

func toolJSON(v any) (*mcp.CallToolResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return toolError("failed to serialize response"), nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(b)}},
	}, nil
}
