package agents

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// --- list_agents -------------------------------------------------------------

func registerListAgents(server *mcp.Server, db *gorm.DB, token *model.Token, teamID uuid.UUID) {
	server.AddTool(&mcp.Tool{
		Name:        toolListAgents,
		Description: "List the agents in the calling Hivy agent's team (id, name, description, model, status). Use get_agent to inspect one before calling update_agent.",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties":           map[string]any{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleListAgents(ctx, db, token, teamID)
	})
}

func handleListAgents(ctx context.Context, db *gorm.DB, token *model.Token, teamID uuid.UUID) (*mcp.CallToolResult, error) {
	var rows []model.Agent
	if err := db.WithContext(ctx).
		Where("org_id = ? AND team_id = ? AND parent_agent_id IS NULL AND status <> ?",
			token.OrgID, teamID, "archived").
		Order("is_default DESC, name ASC").
		Find(&rows).Error; err != nil {
		return toolError("failed to list agents: " + err.Error()), nil
	}
	out := make([]map[string]any, 0, len(rows))
	for i := range rows {
		a := &rows[i]
		out = append(out, map[string]any{
			"id":          a.ID.String(),
			"name":        a.Name,
			"description": derefString(a.Description),
			"model":       a.Model,
			"status":      a.Status,
			"is_default":  a.IsDefault,
		})
	}
	return toolJSON(map[string]any{"agents": out})
}

// --- get_agent ---------------------------------------------------------------

type getAgentArgs struct {
	AgentID string `json:"agent_id"`
}

func registerGetAgent(server *mcp.Server, db *gorm.DB, token *model.Token, teamID uuid.UUID, frontendURL string) {
	server.AddTool(&mcp.Tool{
		Name:        toolGetAgent,
		Description: "Get the full configuration of one agent in the calling Hivy agent's team: instructions, model, connections, skills, tools, and sub-agents. Use this to inspect an agent before calling update_agent.",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"agent_id": map[string]any{"type": "string", "description": "UUID of the agent to fetch."},
			},
			"required": []string{"agent_id"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args getAgentArgs
		if errResult := decodeArgs(req, &args); errResult != nil {
			return errResult, nil
		}
		return handleGetAgent(ctx, db, token, teamID, frontendURL, args)
	})
}

func handleGetAgent(ctx context.Context, db *gorm.DB, token *model.Token, teamID uuid.UUID, frontendURL string, args getAgentArgs) (*mcp.CallToolResult, error) {
	agentID, err := uuid.Parse(strings.TrimSpace(args.AgentID))
	if err != nil || agentID == uuid.Nil {
		return toolError("agent_id must be a valid UUID"), nil
	}
	agent, err := loadTeamAgent(ctx, db, token.OrgID, teamID, agentID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return toolError("agent not found"), nil
		}
		return toolError("failed to load agent: " + err.Error()), nil
	}
	subs, err := agentSubAgentsDetailed(ctx, db, agent.ID)
	if err != nil {
		return toolError(err.Error()), nil
	}
	return toolJSON(map[string]any{
		"agent": map[string]any{
			"id":           agent.ID.String(),
			"name":         agent.Name,
			"description":  derefString(agent.Description),
			"instructions": derefString(agent.Instructions),
			"model":        agent.Model,
			"status":       agent.Status,
			"is_default":   agent.IsDefault,
			"skills":       agentSkillSlugs(agent),
			"tools":        agentToolIDs(agent),
			"sub_agents":   subs,
		},
		"url": agentURL(frontendURL, agent.ID),
	})
}

// agentSubAgentsDetailed returns sub-agents with the fields needed to inspect
// and re-send them via update_agent (name, description, instructions, model,
// skills, tools).
func agentSubAgentsDetailed(ctx context.Context, db *gorm.DB, parentID uuid.UUID) ([]map[string]any, error) {
	var rows []model.Agent
	if err := db.WithContext(ctx).
		Where("parent_agent_id = ? AND type = ? AND status <> ?", parentID, model.AgentTypeSubAgent, "archived").
		Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load sub-agents: %w", err)
	}
	out := make([]map[string]any, 0, len(rows))
	for i := range rows {
		r := &rows[i]
		out = append(out, map[string]any{
			"id":           r.ID.String(),
			"name":         r.Name,
			"description":  derefString(r.Description),
			"instructions": derefString(r.Instructions),
			"model":        r.Model,
			"skills":       agentSkillSlugs(r),
			"tools":        agentToolIDs(r),
		})
	}
	return out, nil
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
