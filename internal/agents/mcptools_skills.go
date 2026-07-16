package agents

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/skillaccess"
)

func registerListTeamSkills(server *mcp.Server, db *gorm.DB, token *model.Token) {
	server.AddTool(&mcp.Tool{
		Name:        toolListTeamSkills,
		Description: "List the skills available to the calling agent's team, including whether each is team-owned, explicitly granted, or recommended by a granted connection.",
		InputSchema: map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}},
	}, func(ctx context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if token == nil {
			return toolError("missing agent token"), nil
		}
		agentID, err := tokenAgentID(token)
		if err != nil {
			return toolError(err.Error()), nil
		}
		agent, err := loadOrgAgent(ctx, db, token.OrgID, agentID)
		if err != nil {
			return toolError("calling agent not found"), nil
		}
		resolved, err := skillaccess.ResolveAgent(ctx, db, *agent)
		if err != nil {
			return toolError("failed to list team skills"), nil
		}
		items := make([]map[string]any, 0, len(resolved))
		for _, item := range resolved {
			description := ""
			if item.Skill.Description != nil {
				description = *item.Skill.Description
			}
			items = append(items, map[string]any{
				"id": item.Skill.ID, "slug": item.Skill.Slug, "name": item.Skill.Name,
				"description": description, "sources": item.Sources,
			})
		}
		return toolJSON(map[string]any{"skills": items})
	})
}
