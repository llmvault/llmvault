package handler

import (
	"context"
	"sort"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/model"
)

// pluginConnCache memoizes the org-scoped connection lookups used when building
// plugin responses, so a list of N plugins does not re-run the same
// provider-connection queries once per plugin.
type pluginConnCache struct {
	h        *PluginHandler
	orgID    uuid.UUID
	firstCon map[string]connLookup // provider -> first active connection
	hasReq   map[string]bool       // provider:kind -> requirement satisfied
}

type connLookup struct {
	conn model.Connection
	ok   bool
}

func newPluginConnCache(h *PluginHandler, orgID uuid.UUID) *pluginConnCache {
	return &pluginConnCache{
		h:        h,
		orgID:    orgID,
		firstCon: map[string]connLookup{},
		hasReq:   map[string]bool{},
	}
}

func (c *pluginConnCache) firstConnection(ctx context.Context, provider string) (model.Connection, bool, error) {
	if v, ok := c.firstCon[provider]; ok {
		return v.conn, v.ok, nil
	}
	conn, has, err := c.h.firstActiveConnectionForProvider(ctx, c.orgID, provider)
	if err != nil {
		return model.Connection{}, false, err
	}
	c.firstCon[provider] = connLookup{conn: conn, ok: has}
	return conn, has, nil
}

func (c *pluginConnCache) hasRequirement(ctx context.Context, req model.PluginIntegration) (bool, error) {
	key := req.Provider + ":" + req.Kind
	if v, ok := c.hasReq[key]; ok {
		return v, nil
	}
	ok, err := c.h.hasConnectionRequirement(ctx, c.orgID, req)
	if err != nil {
		return false, err
	}
	c.hasReq[key] = ok
	return ok, nil
}

func (c *pluginConnCache) computeMissing(ctx context.Context, reqs []model.PluginIntegration) ([]pluginConnectionRequirement, error) {
	missing := make([]pluginConnectionRequirement, 0)
	for _, req := range reqs {
		if !req.Required {
			continue
		}
		ok, err := c.hasRequirement(ctx, req)
		if err != nil {
			return nil, err
		}
		if !ok {
			missing = append(missing, pluginConnectionRequirement{Provider: req.Provider, Kind: req.Kind, Required: req.Required})
		}
	}
	return missing, nil
}

func (c *pluginConnCache) computeResourceRequirements(ctx context.Context, reqs []model.PluginIntegration, installed bool) ([]pluginResourceRequirement, error) {
	cat := c.h.catalog
	if cat == nil {
		cat = catalog.Global()
	}
	out := make([]pluginResourceRequirement, 0)
	seen := map[string]struct{}{}
	for _, req := range reqs {
		if req.Kind != model.PluginIntegrationKindIntegration {
			continue
		}
		resources := cat.GetConfigurableResources(req.Provider)
		if len(resources) == 0 {
			continue
		}
		conn, hasConnection, err := c.firstConnection(ctx, req.Provider)
		if err != nil {
			return nil, err
		}
		for _, resource := range resources {
			key := req.Provider + ":" + resource.Key
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			count := 0
			connectionID := ""
			if hasConnection {
				count = selectedResourceCount(connectionDefaultResources(conn), resource.Key)
				connectionID = conn.ID.String()
			}
			out = append(out, pluginResourceRequirement{
				Provider:      req.Provider,
				Kind:          req.Kind,
				Required:      req.Required,
				ConnectionID:  connectionID,
				ResourceKey:   resource.Key,
				DisplayName:   resource.DisplayName,
				Description:   resource.Description,
				Selected:      count > 0,
				SelectedCount: count,
				Missing:       installed && hasConnection && count == 0,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider == out[j].Provider {
			return out[i].ResourceKey < out[j].ResourceKey
		}
		return out[i].Provider < out[j].Provider
	})
	return out, nil
}

func (h *PluginHandler) firstActiveConnectionForProvider(ctx context.Context, orgID uuid.UUID, provider string) (model.Connection, bool, error) {
	var conn model.Connection
	err := h.db.WithContext(ctx).
		Preload("Integration").
		Joins("JOIN integrations ON integrations.id = connections.integration_id AND integrations.deleted_at IS NULL").
		Where("connections.org_id = ? AND connections.revoked_at IS NULL AND integrations.provider = ?", orgID, provider).
		Order("connections.created_at ASC").
		First(&conn).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return model.Connection{}, false, nil
		}
		return model.Connection{}, false, err
	}
	return conn, true, nil
}

func (h *PluginHandler) missingRequirements(ctx context.Context, orgID, pluginID uuid.UUID) ([]pluginConnectionRequirement, error) {
	var reqs []model.PluginIntegration
	if err := h.db.WithContext(ctx).
		Where("plugin_id = ? AND required = true", pluginID).
		Find(&reqs).Error; err != nil {
		return nil, err
	}
	missing := make([]pluginConnectionRequirement, 0)
	for _, req := range reqs {
		ok, err := h.hasConnectionRequirement(ctx, orgID, req)
		if err != nil {
			return nil, err
		}
		if !ok {
			missing = append(missing, pluginConnectionRequirement{Provider: req.Provider, Kind: req.Kind, Required: req.Required})
		}
	}
	return missing, nil
}

func (h *PluginHandler) hasConnectionRequirement(ctx context.Context, orgID uuid.UUID, req model.PluginIntegration) (bool, error) {
	switch req.Kind {
	case model.PluginIntegrationKindDatabase:
		var count int64
		err := h.db.WithContext(ctx).Model(&model.DatabaseConnection{}).
			Where("org_id = ? AND provider = ? AND revoked_at IS NULL", orgID, req.Provider).
			Count(&count).Error
		return count > 0, err
	default:
		var count int64
		err := h.db.WithContext(ctx).Model(&model.Connection{}).
			Joins("JOIN integrations ON integrations.id = connections.integration_id AND integrations.deleted_at IS NULL").
			Where("connections.org_id = ? AND connections.revoked_at IS NULL AND integrations.provider = ?", orgID, req.Provider).
			Count(&count).Error
		return count > 0, err
	}
}
