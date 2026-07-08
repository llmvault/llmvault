package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/channelagents"
	"github.com/usehivy/hivy/internal/model"
)

func (h *AgentHandler) toAgentCatalogResponse(ctx context.Context, orgID uuid.UUID, c model.AgentCatalog) (agentCatalogResponse, error) {
	required, err := h.catalogPluginSummaries(ctx, orgID, c.RequiredPlugins)
	if err != nil {
		return agentCatalogResponse{}, err
	}
	recommended, err := h.catalogPluginSummaries(ctx, orgID, c.RecommendedPlugins)
	if err != nil {
		return agentCatalogResponse{}, err
	}
	// installed_agent_id points at a live agent; a non-manager must not learn
	// the id of an installed agent they cannot see. Managers and API-key callers
	// see it whenever an install exists.
	orgWide, userID, err := actorSeesOrgWide(ctx, h.db, orgID)
	if err != nil {
		return agentCatalogResponse{}, err
	}
	var installedID *string
	installedQ := h.db.WithContext(ctx).
		Where("org_id = ? AND agent_catalog_id = ? AND status <> ? AND parent_agent_id IS NULL", orgID, c.ID, "archived")
	if !orgWide {
		installedQ = installedQ.Where("id IN (?)", channelagents.VisibleAgentIDsSubquery(h.db, orgID, userID))
	}
	var agent model.Agent
	err = installedQ.First(&agent).Error
	if err == nil {
		value := agent.ID.String()
		installedID = &value
	} else if err != gorm.ErrRecordNotFound {
		return agentCatalogResponse{}, err
	}
	// installed_team_ids: which of the caller's visible teams already have a
	// (non-archived) clone of this catalog agent. Org-wide callers see all teams;
	// members are scoped to teams they belong to.
	teamIDsQ := h.db.WithContext(ctx).Model(&model.Agent{}).
		Where("org_id = ? AND agent_catalog_id = ? AND status <> ? AND team_id IS NOT NULL", orgID, c.ID, "archived")
	if !orgWide {
		teamIDsQ = teamIDsQ.Where("team_id IN (?)", visibleTeamSubquery(h.db, userID))
	}
	var teamIDs []uuid.UUID
	if err := teamIDsQ.Distinct("team_id").Order("team_id").Pluck("team_id", &teamIDs).Error; err != nil {
		return agentCatalogResponse{}, err
	}
	installedTeamIDs := make([]string, 0, len(teamIDs))
	for _, id := range teamIDs {
		installedTeamIDs = append(installedTeamIDs, id.String())
	}
	return agentCatalogResponse{
		ID:                 c.ID.String(),
		Slug:               c.Slug,
		Name:               c.Name,
		Description:        c.Description,
		Category:           c.Category,
		AvatarURL:          c.AvatarURL,
		Developer:          c.Developer,
		Official:           c.Official,
		IsDefault:          c.IsDefault,
		Model:              c.Model,
		SandboxImage:       model.NormalizeSandboxImage(c.SandboxImage),
		RequiredPlugins:    required,
		RecommendedPlugins: recommended,
		InstalledAgentID:   installedID,
		InstalledTeamIDs:   installedTeamIDs,
	}, nil
}

func (h *AgentHandler) catalogPluginSummaries(ctx context.Context, orgID uuid.UUID, slugs []string) ([]agentCatalogPluginSummary, error) {
	out := make([]agentCatalogPluginSummary, 0, len(slugs))
	for _, slug := range slugs {
		slug = strings.TrimSpace(slug)
		if slug == "" {
			continue
		}
		summary, err := h.catalogPluginSummary(ctx, orgID, slug)
		if err != nil {
			return nil, err
		}
		out = append(out, summary)
	}
	return out, nil
}

func (h *AgentHandler) catalogPluginSummary(ctx context.Context, orgID uuid.UUID, slug string) (agentCatalogPluginSummary, error) {
	var plugin model.Plugin
	err := h.db.WithContext(ctx).
		Where("slug = ? AND status = ?", slug, model.PluginStatusActive).
		First(&plugin).Error
	if err == gorm.ErrRecordNotFound {
		return agentCatalogPluginSummary{Slug: slug, Name: slug}, nil
	}
	if err != nil {
		return agentCatalogPluginSummary{}, err
	}
	installed, err := h.orgPluginInstalled(ctx, orgID, plugin.ID)
	if err != nil {
		return agentCatalogPluginSummary{}, err
	}
	return agentCatalogPluginSummary{
		ID:        plugin.ID.String(),
		Slug:      plugin.Slug,
		Name:      plugin.Name,
		Installed: installed,
	}, nil
}

func (h *AgentHandler) orgPluginInstalled(ctx context.Context, orgID, pluginID uuid.UUID) (bool, error) {
	var count int64
	err := h.db.WithContext(ctx).Model(&model.OrgPluginInstall{}).
		Where("org_id = ? AND plugin_id = ? AND revoked_at IS NULL", orgID, pluginID).
		Count(&count).Error
	return count > 0, err
}

func (h *AgentHandler) loadAgentCatalogBySlug(w http.ResponseWriter, r *http.Request, slug string) (model.AgentCatalog, bool) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "agent catalog slug required"})
		return model.AgentCatalog{}, false
	}
	var catalog model.AgentCatalog
	err := h.db.WithContext(r.Context()).
		Where("slug = ? AND status = ?", slug, model.AgentCatalogStatusActive).
		First(&catalog).Error
	if err == nil {
		return catalog, true
	}
	if err == gorm.ErrRecordNotFound {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "agent catalog entry not found"})
		return model.AgentCatalog{}, false
	}
	writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load agent catalog entry"})
	return model.AgentCatalog{}, false
}
