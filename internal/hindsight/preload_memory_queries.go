package hindsight

import (
	"context"
	"sort"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/connectionaccess"
	"github.com/usehivy/hivy/internal/model"
)

type MemoryListQuery struct {
	Name        string
	TagGroups   []any
	ExcludeTags []string
}

func PreloadMemoryListQueries(ctx context.Context, db *gorm.DB, agent *model.Agent) ([]MemoryListQuery, error) {
	if agent == nil || agent.OrgID == nil || db == nil {
		return nil, nil
	}
	current := *agent
	if err := db.WithContext(ctx).Where("id = ? AND org_id = ?", agent.ID, *agent.OrgID).First(&current).Error; err != nil {
		return nil, nil
	}
	connections, err := loadAgentMemoryConnections(ctx, db, *agent.OrgID, agent.ID)
	if err != nil {
		return nil, err
	}

	byProvider := map[string]map[string][]string{}
	for _, conn := range connections {
		provider := normalizeTagValue(conn.Integration.Provider)
		if provider == "" {
			continue
		}
		if byProvider[provider] == nil {
			byProvider[provider] = map[string][]string{}
		}
		resources := connectionaccess.EffectiveResources(current.Resources, conn)
		for resourceType, rawItems := range resources {
			resourceType = normalizeTagValue(resourceType)
			if resourceType == "" {
				continue
			}
			byProvider[provider][resourceType] = append(
				byProvider[provider][resourceType],
				resourceIDsFromAny(provider, resourceType, rawItems)...,
			)
		}
	}

	queries := make([]MemoryListQuery, 0, len(byProvider)+1)
	for _, provider := range sortedKeys(byProvider) {
		queries = append(queries, providerMemoryListQuery(provider, byProvider[provider]))
	}
	queries = append(queries, MemoryListQuery{
		Name:        "org",
		ExcludeTags: []string{"scope:" + MemoryScopeProvider, "scope:" + MemoryScopeResource},
	})
	return queries, nil
}

func loadAgentMemoryConnections(ctx context.Context, db *gorm.DB, orgID, agentID uuid.UUID) ([]model.Connection, error) {
	var connections []model.Connection
	err := db.WithContext(ctx).
		Preload("Integration").
		Joins("JOIN integrations ON integrations.id = connections.integration_id AND integrations.deleted_at IS NULL").
		Joins("JOIN plugin_integrations ON plugin_integrations.provider = integrations.provider AND plugin_integrations.kind = ?", model.PluginIntegrationKindIntegration).
		Joins("JOIN agent_plugin_installs ON agent_plugin_installs.plugin_id = plugin_integrations.plugin_id AND agent_plugin_installs.org_id = connections.org_id AND agent_plugin_installs.agent_id = ?", agentID).
		Joins("JOIN org_plugin_installs ON org_plugin_installs.plugin_id = plugin_integrations.plugin_id AND org_plugin_installs.org_id = connections.org_id AND org_plugin_installs.revoked_at IS NULL").
		Joins("JOIN plugins ON plugins.id = plugin_integrations.plugin_id AND plugins.status = ?", model.PluginStatusActive).
		Where("connections.org_id = ? AND connections.revoked_at IS NULL", orgID).
		Order("integrations.provider ASC, connections.created_at ASC").
		Find(&connections).Error
	return connections, err
}

func providerMemoryListQuery(provider string, resources map[string][]string) MemoryListQuery {
	seen := map[string]struct{}{}
	groups := []any{map[string]any{"tags": []string{"scope:" + MemoryScopeProvider, "provider:" + provider}, "match": "all_strict"}}
	for _, resourceType := range sortedKeys(resources) {
		for _, resourceID := range resources[resourceType] {
			if resourceID == "" {
				continue
			}
			resourceTag := "resource:" + provider + ":" + resourceType + ":" + resourceID
			if _, ok := seen[resourceTag]; ok {
				continue
			}
			seen[resourceTag] = struct{}{}
			groups = append(groups, map[string]any{
				"tags":  []string{"scope:" + MemoryScopeResource, "provider:" + provider, "resource_type:" + resourceType, resourceTag},
				"match": "all_strict",
			})
		}
	}
	return MemoryListQuery{Name: provider, TagGroups: groups}
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
