package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// Tool names. list_team_skills is universal. Agent discovery and mutation are
// reserved for the team's default Hivy agent.
const (
	toolListTeamSkills = "list_team_skills"
	toolListAgents     = "list_agents"
	toolGetAgent       = "get_agent"
	toolCreateAgent    = "create_agent"
	toolUpdateAgent    = "update_agent"
)

// NewToolsFunc returns the agent-builder ToolsFunc. Every top-level agent gets
// list_team_skills. Only the default Hivy agent gets agent discovery and
// mutation, and every one of those operations remains bound to Hivy's team.
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
		registerListTeamSkills(server, deps.DB, token)
		if !agent.IsDefault {
			return
		}
		registerListAgents(server, deps.DB, token, agent.TeamID)
		registerGetAgent(server, deps.DB, token, agent.TeamID, frontendURL)
		registerCreateAgent(server, deps, token, agent.TeamID, frontendURL)
		registerUpdateAgent(server, deps, token, agent.TeamID, frontendURL)
	}
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

func loadTeamAgent(ctx context.Context, db *gorm.DB, orgID, teamID, agentID uuid.UUID) (*model.Agent, error) {
	var agent model.Agent
	if err := db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND team_id = ? AND parent_agent_id IS NULL AND status <> ?",
			agentID, orgID, teamID, "archived").
		First(&agent).Error; err != nil {
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
