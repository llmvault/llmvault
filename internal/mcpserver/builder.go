package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/counter"
	"github.com/usehivy/hivy/internal/model"
)

// MemoryToolsFunc is a callback that registers memory tools on a server.
// Used to avoid an import cycle between mcpserver and hindsight.
type MemoryToolsFunc func(server *mcp.Server, agentID string, db *gorm.DB)

// WebToolsFunc is a callback that registers web_fetch and web_search on a
// server. Used to avoid an import cycle between mcpserver and spider.
type WebToolsFunc func(server *mcp.Server, token *model.Token)

// KnowledgeToolsFunc registers org-scoped knowledge-base search tools.
type KnowledgeToolsFunc func(server *mcp.Server, token *model.Token)

// BuildServer creates an MCP server with platform-native tools. Connection and
// integration access is handled by provider proxy endpoints, not MCP tools.
// If addMemoryTools is non-nil, it is called to register memory tools.
// If addWebTools is non-nil, it is called to register web_fetch and web_search
// on the same server after memory tools are registered.
func BuildServer(
	ctx context.Context,
	token *model.Token,
	db *gorm.DB,
	ctr *counter.Counter,
	addMemoryTools MemoryToolsFunc,
	addWebTools WebToolsFunc,
	addKnowledgeTools KnowledgeToolsFunc,
) (*mcp.Server, error) {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "hivy",
		Version: "v1.0.0",
	}, nil)

	if addMemoryTools != nil {
		agentID, _ := token.Meta[model.TokenMetaAgentID].(string)
		if agentID != "" {
			addMemoryTools(server, agentID, db)
		}
	}

	if addWebTools != nil {
		addWebTools(server, token)
	}

	if addKnowledgeTools != nil {
		addKnowledgeTools(server, token)
	}

	return server, nil
}

// buildInputSchema converts the JSON Schema from the catalog into a format
// accepted by the MCP SDK. The SDK expects an any that marshals to JSON Schema.
func buildInputSchema(params json.RawMessage) any {
	if len(params) == 0 {
		return map[string]any{"type": "object"}
	}
	var schema any
	if err := json.Unmarshal(params, &schema); err != nil {
		return map[string]any{"type": "object"}
	}
	return schema
}
