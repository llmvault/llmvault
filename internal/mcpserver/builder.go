package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/counter"
	"github.com/usehivy/hivy/internal/model"
)

// WebToolsFunc is a callback that registers web_fetch and web_search on a
// server. Used to avoid an import cycle between mcpserver and webcrawl.
type WebToolsFunc func(server *mcp.Server, token *model.Token)

// KnowledgeToolsFunc registers org-scoped knowledge-base search tools.
type KnowledgeToolsFunc func(server *mcp.Server, token *model.Token)

// MemoryToolsFunc registers org and user memory tools for agent runtimes.
type MemoryToolsFunc func(server *mcp.Server, token *model.Token)

// ImageGenerationToolsFunc registers agent image generation tools.
type ImageGenerationToolsFunc func(server *mcp.Server, token *model.Token)

// SkillToolsFunc registers skill_view and any eligible skill-manager tools.
type SkillToolsFunc func(server *mcp.Server, token *model.Token)

// AgentBuilderToolsFunc registers the agent-builder tools (list_team_skills,
// create_agent, update_agent). The func itself gates create_agent/update_agent
// per calling agent, so it may always be invoked.
type AgentBuilderToolsFunc func(server *mcp.Server, token *model.Token)

// SheetToolsFunc registers the sheets tool group.
type SheetToolsFunc func(server *mcp.Server, token *model.Token)

// AppsToolsFunc registers the apps tool group.
type AppsToolsFunc func(server *mcp.Server, token *model.Token)

// EmailToolsFunc registers agent inbox read, search, and send tools.
type EmailToolsFunc func(server *mcp.Server, token *model.Token)

const universalSkillViewTool = "skill_view"

// hivyMCPToolNames is the complete native Hivy MCP surface. BuildServer uses
// it to remove tools before the JTI-scoped server is cached, so the runtime's
// initial tools/list response contains only the agent's granted capabilities.
var hivyMCPToolNames = []string{
	"web_fetch", "web_search", "web_crawl",
	"search_knowledge_base", "manage_memories",
	"generate_image", "generate_vector_image", "remix_image", "vectorize_image",
	"skill_view",
	"list_team_skills", "list_agents", "get_agent", "create_agent", "update_agent",
	"create_skill", "update_skill", "archive_skill",
	"sheet_create", "sheet_list", "sheet_describe", "sheet_manage", "rows_query", "rows_write", "sheet_import_csv", "sheet_operations",
	"app_create", "app_publish", "app_status", "app_logs", "app_rollback",
	"send_email", "email_read", "email_search",
	"cron", "create_http_trigger",
}

func hivyMCPToolAllowed(filter *model.ToolFilter, name string) bool {
	if name == universalSkillViewTool {
		return true
	}
	// A nil filter is reserved for non-runtime callers (e.g. focused package
	// tests). Production agent-proxy requests always receive the compiler's
	// non-nil allow-list through the JTI server factory.
	if filter == nil {
		return true
	}
	for _, denied := range filter.Deny {
		if hivyMCPToolNameMatches(denied, name) {
			return false
		}
	}
	for _, allowed := range filter.Allow {
		if hivyMCPToolNameMatches(allowed, name) {
			return true
		}
	}
	return false
}

func hivyMCPToolNameMatches(candidate, raw string) bool {
	return candidate == raw || candidate == "hivy_"+raw
}

func hasAllowedHivyMCPTool(filter *model.ToolFilter, names ...string) bool {
	for _, name := range names {
		if hivyMCPToolAllowed(filter, name) {
			return true
		}
	}
	return false
}

func filterHivyMCPTools(server *mcp.Server, filter *model.ToolFilter) {
	if server == nil || filter == nil {
		return
	}
	for _, name := range hivyMCPToolNames {
		if !hivyMCPToolAllowed(filter, name) {
			server.RemoveTools(name)
		}
	}
}

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
	addAppsTools AppsToolsFunc,
	addEmailTools EmailToolsFunc,
	toolFilters ...*model.ToolFilter,
) (*mcp.Server, error) {
	var toolFilter *model.ToolFilter
	if len(toolFilters) > 0 {
		toolFilter = toolFilters[0]
	}
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "hivy",
		Version: "v1.0.0",
	}, nil)

	if addWebTools != nil && hasAllowedHivyMCPTool(toolFilter, "web_fetch", "web_search", "web_crawl") {
		addWebTools(server, token)
	}

	if addKnowledgeTools != nil && hasAllowedHivyMCPTool(toolFilter, "search_knowledge_base") {
		addKnowledgeTools(server, token)
	}

	if addMemoryTools != nil && hasAllowedHivyMCPTool(toolFilter, "manage_memories") {
		addMemoryTools(server, token)
	}

	if addImageGenerationTools != nil && hasAllowedHivyMCPTool(toolFilter, "generate_image", "generate_vector_image", "remix_image", "vectorize_image") {
		addImageGenerationTools(server, token)
	}

	if addSkillTools != nil {
		addSkillTools(server, token)
	}

	if addAgentBuilderTools != nil && hasAllowedHivyMCPTool(toolFilter, "list_team_skills", "list_agents", "get_agent", "create_agent", "update_agent") {
		addAgentBuilderTools(server, token)
	}

	if addSheetTools != nil && hasAllowedHivyMCPTool(toolFilter, "sheet_create", "sheet_list", "sheet_describe", "sheet_manage", "rows_query", "rows_write", "sheet_import_csv", "sheet_operations") {
		addSheetTools(server, token)
	}

	if addAppsTools != nil && hasAllowedHivyMCPTool(toolFilter, "app_create", "app_publish", "app_status", "app_logs", "app_rollback") {
		addAppsTools(server, token)
	}
	if addEmailTools != nil && hasAllowedHivyMCPTool(toolFilter, "send_email", "email_read", "email_search") {
		addEmailTools(server, token)
	}

	if hasAllowedHivyMCPTool(toolFilter, "cron") {
		addCronTool(server, token, db)
	}
	if hasAllowedHivyMCPTool(toolFilter, "create_http_trigger") {
		addHTTPTriggerTool(server, token, db)
	}
	filterHivyMCPTools(server, toolFilter)

	return server, nil
}
