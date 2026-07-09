package handler

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/channelagents"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/pluginresolve"
	pluginstore "github.com/usehivy/hivy/internal/plugins"
)

// pluginListData holds every per-plugin lookup batched by plugin_id so a list of
// N plugins issues a bounded number of queries instead of ~6 per plugin.
type pluginListData struct {
	skills       map[uuid.UUID][]model.Skill
	reqs         map[uuid.UUID][]model.PluginIntegration
	installCount map[uuid.UUID]int64
	enabled      map[uuid.UUID][]uuid.UUID
}

func (h *PluginHandler) loadPluginListData(ctx context.Context, orgID uuid.UUID, plugins []model.Plugin, scope pluginActorScope) (pluginListData, error) {
	data := pluginListData{
		skills:       map[uuid.UUID][]model.Skill{},
		reqs:         map[uuid.UUID][]model.PluginIntegration{},
		installCount: map[uuid.UUID]int64{},
		enabled:      map[uuid.UUID][]uuid.UUID{},
	}
	if len(plugins) == 0 {
		return data, nil
	}
	ids := make([]uuid.UUID, 0, len(plugins))
	for _, p := range plugins {
		ids = append(ids, p.ID)
	}

	var skills []model.Skill
	if err := h.db.WithContext(ctx).
		Where("plugin_id IN ? AND status = ?", ids, model.SkillStatusPublished).
		Order("name ASC").
		Find(&skills).Error; err != nil {
		return pluginListData{}, err
	}
	for _, s := range skills {
		if s.PluginID != nil {
			data.skills[*s.PluginID] = append(data.skills[*s.PluginID], s)
		}
	}

	var reqs []model.PluginIntegration
	if err := h.db.WithContext(ctx).Where("plugin_id IN ?", ids).Order("provider ASC").Find(&reqs).Error; err != nil {
		return pluginListData{}, err
	}
	for _, req := range reqs {
		data.reqs[req.PluginID] = append(data.reqs[req.PluginID], req)
	}

	type installCountRow struct {
		PluginID uuid.UUID
		Count    int64
	}
	var counts []installCountRow
	if err := h.db.WithContext(ctx).Model(&model.OrgPluginInstall{}).
		Select("plugin_id, count(*) as count").
		Where("org_id = ? AND plugin_id IN ? AND revoked_at IS NULL", orgID, ids).
		Group("plugin_id").
		Scan(&counts).Error; err != nil {
		return pluginListData{}, err
	}
	for _, c := range counts {
		data.installCount[c.PluginID] = c.Count
	}

	// enabled_agent_ids is derived from each agent's effective plugin set. It must
	// not reveal agents the caller cannot see, so a non-manager member's query is
	// scoped to agents visible to them. Each visible agent's effective set is
	// resolved once and inverted into plugin -> agent IDs (the pair rule is applied
	// per agent by the resolver).
	agentsQ := h.db.WithContext(ctx).Where("org_id = ? AND status <> ?", orgID, "archived")
	if !scope.orgWide {
		agentsQ = agentsQ.Where("id IN (?)", channelagents.VisibleAgentIDsSubquery(h.db, orgID, scope.userID))
	}
	var agents []model.Agent
	if err := agentsQ.Order("created_at ASC").Find(&agents).Error; err != nil {
		return pluginListData{}, err
	}
	pluginSet := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		pluginSet[id] = true
	}
	for _, agent := range agents {
		effective, err := pluginresolve.EffectivePluginIDs(ctx, h.db, agent)
		if err != nil {
			return pluginListData{}, err
		}
		for _, pid := range effective {
			if pluginSet[pid] {
				data.enabled[pid] = append(data.enabled[pid], agent.ID)
			}
		}
	}
	return data, nil
}

func (h *PluginHandler) toPluginResponse(ctx context.Context, orgID uuid.UUID, plugin model.Plugin, scope pluginActorScope) (pluginResponse, error) {
	batch, err := h.loadPluginListData(ctx, orgID, []model.Plugin{plugin}, scope)
	if err != nil {
		return pluginResponse{}, err
	}
	return h.assemblePluginResponse(ctx, orgID, plugin,
		batch.skills[plugin.ID], batch.reqs[plugin.ID], batch.installCount[plugin.ID], batch.enabled[plugin.ID],
		newPluginConnCache(h, orgID))
}

func (h *PluginHandler) assemblePluginResponse(ctx context.Context, orgID uuid.UUID, plugin model.Plugin, skills []model.Skill, reqs []model.PluginIntegration, installCount int64, enabled []uuid.UUID, cache *pluginConnCache) (pluginResponse, error) {
	enabledIDs := make([]string, 0, len(enabled))
	for _, agentID := range enabled {
		enabledIDs = append(enabledIDs, agentID.String())
	}
	missing, err := cache.computeMissing(ctx, reqs)
	if err != nil {
		return pluginResponse{}, err
	}
	resourceRequirements, err := cache.computeResourceRequirements(ctx, reqs, installCount > 0)
	if err != nil {
		return pluginResponse{}, err
	}
	presentation := pluginPresentationMetadata(plugin)
	return pluginResponse{
		ID:                   plugin.ID.String(),
		Slug:                 plugin.Slug,
		Name:                 plugin.Name,
		Description:          plugin.Description,
		Category:             plugin.Category,
		DetailCategory:       presentation.DetailCategory,
		Icon:                 plugin.Icon,
		IconColor:            plugin.IconColor,
		Developer:            plugin.Developer,
		Official:             presentation.Official,
		Featured:             presentation.Featured,
		Capabilities:         presentation.Capabilities,
		Examples:             presentation.Examples,
		Links:                presentation.Links,
		LongDescription:      presentation.LongDescription,
		Version:              plugin.Version,
		Status:               plugin.Status,
		AutoInstall:          pluginstore.PluginAutoInstall(plugin),
		Locked:               pluginstore.PluginLocked(plugin),
		Skills:               toPluginSkillResponses(skills),
		RequiredConnections:  toPluginRequirementResponses(reqs),
		ResourceRequirements: resourceRequirements,
		Installed:            installCount > 0,
		MissingRequirements:  missing,
		EnabledAgentIDs:      enabledIDs,
		CreatedAt:            plugin.CreatedAt.Format(time.RFC3339),
		UpdatedAt:            plugin.UpdatedAt.Format(time.RFC3339),
	}, nil
}
