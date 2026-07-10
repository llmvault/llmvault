package handler

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/pluginresolve"
)

// normalizeDisabledAgentPluginIDsForRequest validates the replacement set for
// an agent's optional team-plugin overrides. A nil value means the caller did
// not change overrides; an empty slice intentionally clears all overrides.
func (h *AgentHandler) normalizeDisabledAgentPluginIDsForRequest(ctx context.Context, w http.ResponseWriter, orgID uuid.UUID, agent *model.Agent, raw *[]string) ([]uuid.UUID, bool) {
	if raw == nil {
		return nil, true
	}

	requested := make([]uuid.UUID, 0, len(*raw))
	seen := make(map[uuid.UUID]bool, len(*raw))
	for _, value := range *raw {
		id, err := uuid.Parse(strings.TrimSpace(value))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "disabled_plugin_ids must contain UUIDs"})
			return nil, false
		}
		if seen[id] {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "disabled_plugin_ids must not contain duplicates"})
			return nil, false
		}
		seen[id] = true
		requested = append(requested, id)
	}
	if len(requested) == 0 {
		return []uuid.UUID{}, true
	}

	var teamPlugins []model.Plugin
	if err := h.db.WithContext(ctx).
		Table("plugins p").
		Select("p.*").
		Joins("JOIN team_plugins tp ON tp.plugin_id = p.id").
		Joins("JOIN org_plugin_installs opi ON opi.org_id = tp.org_id AND opi.plugin_id = tp.plugin_id AND opi.revoked_at IS NULL").
		Where("tp.org_id = ? AND tp.team_id = ? AND p.status = ? AND p.id IN ?", orgID, agent.TeamID, model.PluginStatusActive, requested).
		Find(&teamPlugins).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to validate agent plugins"})
		return nil, false
	}

	byID := make(map[uuid.UUID]model.Plugin, len(teamPlugins))
	for _, plugin := range teamPlugins {
		byID[plugin.ID] = plugin
	}
	required := make(map[string]bool)
	if agent.AgentCatalog != nil {
		for _, slug := range agent.AgentCatalog.RequiredPlugins {
			required[slug] = true
		}
	}
	for _, id := range requested {
		plugin, ok := byID[id]
		if !ok {
			writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "plugins can be disabled only when they are active team plugins"})
			return nil, false
		}
		if pluginresolve.PluginAutoInstall(plugin) || (agent.IsDefault && pluginresolve.PluginDefaultAgentInstall(plugin)) {
			writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "this plugin is always enabled for the agent"})
			return nil, false
		}
		if required[plugin.Slug] {
			writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "this plugin is required by the agent"})
			return nil, false
		}
	}
	return requested, true
}

func replaceAgentPluginOverrides(ctx context.Context, tx *gorm.DB, orgID, agentID uuid.UUID, pluginIDs []uuid.UUID, disabledBy *uuid.UUID) error {
	var previousPluginIDs []uuid.UUID
	if err := tx.WithContext(ctx).
		Model(&model.AgentPluginOverride{}).
		Where("org_id = ? AND agent_id = ?", orgID, agentID).
		Pluck("plugin_id", &previousPluginIDs).Error; err != nil {
		return fmt.Errorf("load agent plugin overrides: %w", err)
	}
	if err := tx.WithContext(ctx).
		Where("org_id = ? AND agent_id = ?", orgID, agentID).
		Delete(&model.AgentPluginOverride{}).Error; err != nil {
		return fmt.Errorf("clear agent plugin overrides: %w", err)
	}
	if len(pluginIDs) > 0 {
		rows := make([]model.AgentPluginOverride, 0, len(pluginIDs))
		for _, pluginID := range pluginIDs {
			rows = append(rows, model.AgentPluginOverride{
				OrgID:      orgID,
				AgentID:    agentID,
				PluginID:   pluginID,
				DisabledBy: disabledBy,
			})
		}
		if err := tx.WithContext(ctx).Create(&rows).Error; err != nil {
			return fmt.Errorf("create agent plugin overrides: %w", err)
		}
	}

	changed := make(map[uuid.UUID]bool, len(previousPluginIDs)+len(pluginIDs))
	for _, pluginID := range previousPluginIDs {
		changed[pluginID] = true
	}
	for _, pluginID := range pluginIDs {
		changed[pluginID] = true
	}
	orderedPluginIDs := make([]uuid.UUID, 0, len(changed))
	for pluginID := range changed {
		orderedPluginIDs = append(orderedPluginIDs, pluginID)
	}
	sort.Slice(orderedPluginIDs, func(i, j int) bool {
		return orderedPluginIDs[i].String() < orderedPluginIDs[j].String()
	})
	for _, pluginID := range orderedPluginIDs {
		if err := pluginresolve.RefreshPluginSkillInstallCounts(ctx, tx, pluginID); err != nil {
			return fmt.Errorf("refresh plugin skill install counts: %w", err)
		}
	}
	return nil
}

func (h *AgentHandler) loadDisabledAgentPluginIDs(ctx context.Context, orgID uuid.UUID, agentIDs ...uuid.UUID) (map[uuid.UUID][]string, error) {
	result := make(map[uuid.UUID][]string, len(agentIDs))
	if len(agentIDs) == 0 {
		return result, nil
	}
	var rows []model.AgentPluginOverride
	if err := h.db.WithContext(ctx).
		Where("org_id = ? AND agent_id IN ?", orgID, agentIDs).
		Order("agent_id ASC, plugin_id ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load agent plugin overrides: %w", err)
	}
	for _, row := range rows {
		result[row.AgentID] = append(result[row.AgentID], row.PluginID.String())
	}
	for _, ids := range result {
		sort.Strings(ids)
	}
	return result, nil
}
