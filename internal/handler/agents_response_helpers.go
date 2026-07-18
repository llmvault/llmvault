package handler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/skillaccess"
)

func validateHarness(harness string) error {
	switch harness {
	case "", "claude", "open_code":
		return nil
	default:
		return fmt.Errorf("invalid harness %q (must be 'claude' or 'open_code')", harness)
	}
}

// loadAgentTriggers loads triggers configured for one or more agents.
// Returns a map from agent ID to trigger responses. Uses a single query with
// joins to avoid N+1.
func (h *AgentHandler) loadAgentTriggers(agentIDs ...uuid.UUID) map[uuid.UUID][]agentTriggerResponse {
	if len(agentIDs) == 0 {
		return nil
	}

	type triggerRow struct {
		AgentID      uuid.UUID      `gorm:"column:agent_id"`
		TriggerID    uuid.UUID      `gorm:"column:trigger_id"`
		TriggerType  string         `gorm:"column:trigger_type"`
		ConnID       *uuid.UUID     `gorm:"column:conn_id"`
		Provider     *string        `gorm:"column:provider"`
		ResourceType string         `gorm:"column:resource_type"`
		ResourceKey  string         `gorm:"column:resource_key"`
		TriggerKeys  pq.StringArray `gorm:"column:trigger_keys;type:text[]"`
		TriggerKey   string         `gorm:"column:trigger_key"`
		TriggerValue string         `gorm:"column:trigger_value"`
		Enabled      bool           `gorm:"column:enabled"`
		Conditions   model.RawJSON  `gorm:"column:conditions"`
		SourceSlug   string         `gorm:"column:source_slug"`
		Instructions string         `gorm:"column:instructions"`
		SecretKey    string         `gorm:"column:secret_key"`
	}

	var rows []triggerRow
	h.db.Raw(`
		SELECT
			at.agent_id,
			at.id AS trigger_id,
			at.trigger_type,
			at.connection_id AS conn_id,
			ii.provider,
			at.resource_type,
			at.resource_key,
			at.trigger_keys,
			at.trigger_key,
			at.trigger_value,
			at.enabled,
			at.conditions,
			at.source_slug,
			at.instructions,
			at.secret_key
		FROM agent_triggers at
		LEFT JOIN connections ic ON ic.id = at.connection_id
		LEFT JOIN integrations ii ON ii.id = ic.integration_id
		WHERE at.agent_id IN ?
		ORDER BY at.id ASC
	`, agentIDs).Scan(&rows)

	result := make(map[uuid.UUID][]agentTriggerResponse, len(agentIDs))
	for _, row := range rows {
		var conditions any
		if len(row.Conditions) > 0 {
			var parsed model.TriggerMatch
			if err := json.Unmarshal(row.Conditions, &parsed); err == nil && len(parsed.Conditions) > 0 {
				conditions = parsed
			}
		}

		response := agentTriggerResponse{
			ID:           row.TriggerID.String(),
			TriggerType:  row.TriggerType,
			TriggerKeys:  []string(row.TriggerKeys),
			TriggerKey:   row.TriggerKey,
			TriggerValue: row.TriggerValue,
			Enabled:      row.Enabled,
			Conditions:   conditions,
			SourceSlug:   row.SourceSlug,
			Instructions: row.Instructions,
			SecretSet:    row.SecretKey != "",
			ResourceType: row.ResourceType,
			ResourceKey:  row.ResourceKey,
		}
		if row.ConnID != nil {
			response.ConnectionID = row.ConnID.String()
		}
		if row.Provider != nil {
			response.Provider = *row.Provider
		}

		result[row.AgentID] = append(result[row.AgentID], response)
	}
	return result
}

// loadAgentSkills loads the effective team skill set for each agent.
func (h *AgentHandler) loadAgentSkills(ctx context.Context, agentIDs ...uuid.UUID) map[uuid.UUID][]agentSkillSummary {
	if len(agentIDs) == 0 {
		return nil
	}
	var agents []model.Agent
	if err := h.db.WithContext(ctx).Where("id IN ?", agentIDs).Find(&agents).Error; err != nil {
		return nil
	}
	if len(agents) == 0 {
		return nil
	}
	result := make(map[uuid.UUID][]agentSkillSummary, len(agentIDs))
	for _, agent := range agents {
		resolved, err := skillaccess.ResolveAgent(ctx, h.db, agent)
		if err != nil {
			return nil
		}
		for _, item := range resolved {
			result[agent.ID] = append(result[agent.ID], agentSkillSummary{
				ID: item.Skill.ID.String(), Name: item.Skill.Name,
				Description: item.Skill.Description, HumanDescription: item.Skill.HumanDescription,
				SourceType: item.Sources[0],
			})
		}
	}
	return result
}

func validateAgentModel(reg *registry.Registry, modelID string) error {
	if reg == nil || modelID == "" {
		return nil
	}
	return reg.ValidateCanonicalModel(modelID)
}
