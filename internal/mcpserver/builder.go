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

// ImageGenerationToolsFunc registers agent image generation tools.
type ImageGenerationToolsFunc func(server *mcp.Server, token *model.Token)

// SkillToolsFunc registers the read-only skill tools (skills_list, skill_view).
type SkillToolsFunc func(server *mcp.Server, token *model.Token)

// AgentBuilderToolsFunc registers the agent-builder tools (list_org_plugins,
// create_agent, update_agent). The func itself gates create_agent/update_agent
// per calling agent, so it may always be invoked.
type AgentBuilderToolsFunc func(server *mcp.Server, token *model.Token)

// SheetToolsFunc registers the sheets tool group (sheet_create, sheet_list,
// sheet_describe, sheet_manage, rows_query, rows_write, sheet_import_csv,
// sheet_operations). The func itself gates registration on the calling
// agent's sheets plugin install, so it may always be invoked.
type SheetToolsFunc func(server *mcp.Server, token *model.Token)

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
	addImageGenerationTools ImageGenerationToolsFunc,
	addSkillTools SkillToolsFunc,
	addAgentBuilderTools AgentBuilderToolsFunc,
	addSheetTools SheetToolsFunc,
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

	if addImageGenerationTools != nil {
		addImageGenerationTools(server, token)
	}

	if addSkillTools != nil {
		addSkillTools(server, token)
	}

	if addAgentBuilderTools != nil {
		addAgentBuilderTools(server, token)
	}

	if addSheetTools != nil {
		addSheetTools(server, token)
	}

	addCronTool(server, token, db)
	addHTTPTriggerTool(server, token, db)

	return server, nil
}
