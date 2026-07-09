package agents

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// UpdateAgent applies a true patch to an org-owned agent. Only non-nil fields
// change; provided arrays replace. Sub-agents are delete+recreate when
// provided. Verifies org ownership.
func UpdateAgent(ctx context.Context, deps Deps, orgID, agentID uuid.UUID, in UpdateInput) (*model.Agent, error) {
	if deps.DB == nil {
		return nil, fmt.Errorf("agents: nil DB")
	}
	var agent model.Agent
	if err := deps.DB.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", agentID, orgID, "archived").
		First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}

	updates := map[string]any{}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return nil, fmt.Errorf("name cannot be empty")
		}
		if agent.IsDefault && name != agent.Name {
			return nil, fmt.Errorf("default agent cannot be renamed")
		}
		updates["name"] = name
		agent.Name = name
	}
	if in.Description != nil {
		value := strings.TrimSpace(*in.Description)
		updates["description"] = value
		agent.Description = &value
	}
	if in.Instructions != nil {
		value := strings.TrimSpace(*in.Instructions)
		updates["instructions"] = value
		agent.Instructions = &value
	}
	if in.Model != nil {
		modelID := strings.TrimSpace(*in.Model)
		if modelID == "" {
			return nil, fmt.Errorf("model cannot be empty")
		}
		if err := deps.validateModel(ctx, orgID, modelID); err != nil {
			return nil, err
		}
		updates["model"] = modelID
		agent.Model = modelID
	}
	if in.Status != nil {
		status := strings.TrimSpace(*in.Status)
		if status != "active" && status != "archived" {
			return nil, fmt.Errorf("status must be active or archived")
		}
		updates["status"] = status
		agent.Status = status
	}
	if in.Tools != nil {
		value := *in.Tools
		if value == nil {
			value = model.JSON{}
		}
		updates["tools"] = value
		agent.Tools = value
	}
	if in.SetMcpFilter {
		updates["mcp_tool_filter"] = in.McpToolFilter
		agent.McpToolFilter = in.McpToolFilter
	}
	if in.Skills != nil {
		value := *in.Skills
		if value == nil {
			value = model.JSON{}
		}
		updates["skills"] = value
		agent.Skills = value
	}

	var subRows []model.Agent
	if in.SubAgents != nil {
		var err error
		subRows, err = buildSubAgentRows(ctx, deps, orgID, agent.Model, *in.SubAgents)
		if err != nil {
			return nil, err
		}
	}

	if err := deps.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(updates) > 0 {
			if err := tx.Model(&model.Agent{}).
				Where("id = ? AND org_id = ?", agent.ID, orgID).
				Updates(updates).Error; err != nil {
				return err
			}
		}
		if in.SubAgents != nil {
			if err := tx.Where("parent_agent_id = ? AND type = ?", agent.ID, model.AgentTypeSubAgent).
				Delete(&model.Agent{}).Error; err != nil {
				return err
			}
			for i := range subRows {
				subRows[i].ParentAgentID = &agent.ID
				subRows[i].TeamID = agent.TeamID
				if err := tx.Create(&subRows[i]).Error; err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		return nil, mapWriteError(err)
	}
	return &agent, nil
}
