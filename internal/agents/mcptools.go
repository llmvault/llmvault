package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/channelagents"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/pluginresolve"
)

// Tool names. list_team_plugins, list_agents, and get_agent are read-only;
// create_agent and update_agent are privileged, opt-in mutating tools gated per
// calling agent.
const (
	toolListTeamPlugins = "list_team_plugins"
	toolListAgents      = "list_agents"
	toolGetAgent        = "get_agent"
	toolCreateAgent     = "create_agent"
	toolUpdateAgent     = "update_agent"

	// AgentBuilderPluginSlug gates agent-management MCP tools. The plugin is
	// installed automatically on default Hivy agents; other agents receive it
	// only through their team's plugin grants.
	AgentBuilderPluginSlug = "agent-builder"
)

// NewToolsFunc returns the agent-builder ToolsFunc. It registers the read-only
// list_team_plugins / list_agents / get_agent tools and the mutating
// create_agent / update_agent tools on the MCP server ONLY when the calling
// agent is permitted (see agentBuilderEnabled) AND has the Agent Builder plugin
// through its effective plugin set. frontendURL is used to build the agent URL
// in tool responses.
func NewToolsFunc(deps Deps, frontendURL string) func(server *mcp.Server, token *model.Token) {
	return func(server *mcp.Server, token *model.Token) {
		if server == nil || deps.DB == nil || !agentProxyToken(token) {
			return
		}
		agentID, err := tokenAgentID(token)
		if err != nil {
			return
		}
		agent, err := loadOrgAgent(context.Background(), deps.DB, token.OrgID, agentID)
		if err != nil {
			return
		}
		if !agentBuilderEnabled(agent) {
			return
		}
		hasPlugin, err := pluginresolve.AgentHasPluginSlug(context.Background(), deps.DB, *agent, AgentBuilderPluginSlug)
		if err != nil || !hasPlugin {
			return
		}
		registerListTeamPlugins(server, deps.DB, token, frontendURL)
		registerListAgents(server, deps.DB, token)
		registerGetAgent(server, deps.DB, token, frontendURL)
		registerCreateAgent(server, deps, token, agent.TeamID, frontendURL)
		registerUpdateAgent(server, deps, token, frontendURL)
	}
}

// agentBuilderEnabled reports whether the calling agent may use the privileged
// agent-builder tools. The default Hivy agent is eligible for its catalog-defined
// builder surface; any other agent must explicitly allow-list create_agent or
// update_agent and receive the plugin through its team.
func agentBuilderEnabled(agent *model.Agent) bool {
	if agent == nil {
		return false
	}
	if agent.IsDefault {
		return true
	}
	if agent.McpToolFilter == nil {
		return false
	}
	for _, allowed := range agent.McpToolFilter.Allow {
		switch strings.TrimSpace(allowed) {
		case toolCreateAgent, toolUpdateAgent:
			return true
		}
	}
	return false
}

// --- shared low-level helpers -------------------------------------------------

func loadOrgAgent(ctx context.Context, db *gorm.DB, orgID, agentID uuid.UUID) (*model.Agent, error) {
	var agent model.Agent
	if err := db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", agentID, orgID, "archived").
		First(&agent).Error; err != nil {
		return nil, err
	}
	return &agent, nil
}

// loadVisibleOrgAgent is loadOrgAgent with actor-scoped visibility: when userID
// is non-nil the agent must also be assigned to a channel that user can use, so
// a hidden agent returns gorm.ErrRecordNotFound and never leaks its existence. A
// nil userID (manager or automated run) keeps org-wide access.
func loadVisibleOrgAgent(ctx context.Context, db *gorm.DB, orgID, agentID uuid.UUID, userID *uuid.UUID) (*model.Agent, error) {
	q := db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", agentID, orgID, "archived")
	if userID != nil {
		q = q.Where("id IN (?)", channelagents.VisibleAgentIDsSubquery(db, orgID, userID))
	}
	var agent model.Agent
	if err := q.First(&agent).Error; err != nil {
		return nil, err
	}
	return &agent, nil
}

func agentProxyToken(token *model.Token) bool {
	if token == nil || token.Meta == nil {
		return false
	}
	tokenType, _ := token.Meta[model.TokenMetaType].(string)
	return tokenType == model.TokenTypeAgentProxy
}

func tokenAgentID(token *model.Token) (uuid.UUID, error) {
	agentIDText, _ := token.Meta[model.TokenMetaAgentID].(string)
	agentID, err := uuid.Parse(strings.TrimSpace(agentIDText))
	if err != nil || agentID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("agent proxy token is missing agent_id")
	}
	return agentID, nil
}

func decodeArgs(req *mcp.CallToolRequest, dst any) *mcp.CallToolResult {
	if req == nil || req.Params.Arguments == nil {
		return nil // no arguments is valid for optional-only payloads
	}
	if err := json.Unmarshal(req.Params.Arguments, dst); err != nil {
		return toolError("invalid arguments")
	}
	return nil
}

func toolError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + msg}},
		IsError: true,
	}
}

func toolJSON(v any) (*mcp.CallToolResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return toolError("failed to serialize response"), nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil
}
