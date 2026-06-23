package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/counter"
	"github.com/usehivy/hivy/internal/model"
)

// WebToolsFunc is a callback that registers web_fetch and web_search on a
// server. Used to avoid an import cycle between mcpserver and spider.
type WebToolsFunc func(server *mcp.Server, token *model.Token)

// KnowledgeToolsFunc registers org-scoped knowledge-base search tools.
type KnowledgeToolsFunc func(server *mcp.Server, token *model.Token)

// MemoryToolsFunc registers org and user memory tools for agent runtimes.
type MemoryToolsFunc func(server *mcp.Server, token *model.Token)

// BuildServer creates an MCP server with platform-native tools. Connection and
// integration access is handled by provider proxy endpoints, not MCP tools.
// If addWebTools is non-nil, it is called to register web_fetch and web_search
// on the same server.
func BuildServer(
	ctx context.Context,
	token *model.Token,
	db *gorm.DB,
	ctr *counter.Counter,
	addWebTools WebToolsFunc,
	addKnowledgeTools KnowledgeToolsFunc,
	addMemoryTools MemoryToolsFunc,
) (*mcp.Server, error) {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "hivy",
		Version: "v1.0.0",
	}, nil)

	if addWebTools != nil {
		addWebTools(server, token)
	}

	if addKnowledgeTools != nil {
		addKnowledgeTools(server, token)
	}

	if addMemoryTools != nil {
		addMemoryTools(server, token)
	}

	addCronTool(server, token, db)

	return server, nil
}
