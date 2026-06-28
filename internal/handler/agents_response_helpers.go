package handler

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
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
		ChannelID    *uuid.UUID     `gorm:"column:channel_id"`
		ConnID       *uuid.UUID     `gorm:"column:conn_id"`
		Provider     *string        `gorm:"column:provider"`
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
			at.channel_id,
			at.connection_id AS conn_id,
			ii.provider,
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
		}
		if row.ConnID != nil {
			response.ConnectionID = row.ConnID.String()
		}
		if row.ChannelID != nil {
			response.ChannelID = row.ChannelID.String()
		}
		if row.Provider != nil {
			response.Provider = *row.Provider
		}

		result[row.AgentID] = append(result[row.AgentID], response)
	}
	return result
}

// loadAgentSkills batch-loads skill summaries for one or more agents. Skills are
// inherited from the plugins installed on each agent.
func (h *AgentHandler) loadAgentSkills(agentIDs ...uuid.UUID) map[uuid.UUID][]agentSkillSummary {
	if len(agentIDs) == 0 {
		return nil
	}
	var installs []model.AgentPluginInstall
	if err := h.db.Where("agent_id IN ?", agentIDs).Find(&installs).Error; err != nil {
		return nil
	}
	if len(installs) == 0 {
		return nil
	}
	pluginIDSet := make(map[uuid.UUID]bool)
	agentPlugins := make(map[uuid.UUID][]uuid.UUID, len(agentIDs))
	for _, install := range installs {
		pluginIDSet[install.PluginID] = true
		agentPlugins[install.AgentID] = append(agentPlugins[install.AgentID], install.PluginID)
	}
	pluginIDs := make([]uuid.UUID, 0, len(pluginIDSet))
	for id := range pluginIDSet {
		pluginIDs = append(pluginIDs, id)
	}
	var skills []model.Skill
	if err := h.db.Select("id, name, description, human_description, source_type, plugin_id").
		Where("plugin_id IN ? AND hidden = false AND status = ?", pluginIDs, model.SkillStatusPublished).
		Find(&skills).Error; err != nil {
		return nil
	}
	skillsByPlugin := make(map[uuid.UUID][]model.Skill, len(pluginIDs))
	for _, skill := range skills {
		if skill.PluginID == nil {
			continue
		}
		skillsByPlugin[*skill.PluginID] = append(skillsByPlugin[*skill.PluginID], skill)
	}
	result := make(map[uuid.UUID][]agentSkillSummary, len(agentIDs))
	for agentID, plugins := range agentPlugins {
		for _, pluginID := range plugins {
			for _, skill := range skillsByPlugin[pluginID] {
				result[agentID] = append(result[agentID], agentSkillSummary{
					ID:               skill.ID.String(),
					Name:             skill.Name,
					Description:      skill.Description,
					HumanDescription: skill.HumanDescription,
					SourceType:       skill.SourceType,
				})
			}
		}
	}
	return result
}

// errGitHubAppExclusive is returned when an agent's integrations payload
// attaches both GitHub Apps to the same agent. We restrict agents to a single
// GitHub App identity so it's unambiguous which app authored a given action
// (the primary opens PRs, the code-reviews app reviews them on a different
// agent).
var errGitHubAppExclusive = errors.New("an agent can connect to only one of github-app or github-app-code-reviews, not both")

// validateAgentIntegrationsExclusivity checks the proposed integrations map
// against mutually-exclusive provider rules. integrations is keyed by
// connection UUID (matching the JSONB shape on agents.integrations); we
// resolve those connections to providers via connections → integrations
// scoped to the org.
func validateAgentIntegrationsExclusivity(db *gorm.DB, orgID uuid.UUID, integrations model.JSON) error {
	if len(integrations) == 0 {
		return nil
	}
	connectionIDs := make([]uuid.UUID, 0, len(integrations))
	for key := range integrations {
		id, err := uuid.Parse(key)
		if err != nil {
			continue
		}
		connectionIDs = append(connectionIDs, id)
	}
	if len(connectionIDs) == 0 {
		return nil
	}

	type row struct {
		Provider string
	}
	var rows []row
	err := db.
		Table("connections").
		Select("DISTINCT integrations.provider AS provider").
		Joins("JOIN integrations ON integrations.id = connections.integration_id AND integrations.deleted_at IS NULL").
		Where("connections.id IN ? AND connections.org_id = ? AND connections.revoked_at IS NULL", connectionIDs, orgID).
		Scan(&rows).Error
	if err != nil {
		return fmt.Errorf("resolving integration providers: %w", err)
	}

	hasPrimary, hasReviews := false, false
	for _, r := range rows {
		switch r.Provider {
		case "github-app":
			hasPrimary = true
		case "github-app-code-reviews":
			hasReviews = true
		}
	}
	if hasPrimary && hasReviews {
		return errGitHubAppExclusive
	}
	return nil
}

func validateAgentModel(reg *registry.Registry, modelID string) error {
	if reg == nil || modelID == "" {
		return nil
	}
	return reg.ValidateCanonicalModel(modelID)
}
