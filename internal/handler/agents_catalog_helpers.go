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
	required := make([]agentCatalogConnectionSummary, 0, len(c.RequiredConnections))
	for _, provider := range c.RequiredConnections {
		provider = strings.TrimSpace(provider)
		if provider != "" {
			required = append(required, agentCatalogConnectionSummary{Provider: provider})
		}
	}
	orgWide, userID, err := actorSeesOrgWide(ctx, h.db, orgID)
	if err != nil {
		return agentCatalogResponse{}, err
	}
	var installedID *string
	q := h.db.WithContext(ctx).Where("org_id = ? AND agent_catalog_id = ? AND status <> ? AND parent_agent_id IS NULL", orgID, c.ID, "archived")
	if !orgWide {
		q = q.Where("id IN (?)", channelagents.VisibleAgentIDsSubquery(h.db, orgID, userID))
	}
	var agent model.Agent
	if err = q.First(&agent).Error; err == nil {
		value := agent.ID.String()
		installedID = &value
	} else if err != gorm.ErrRecordNotFound {
		return agentCatalogResponse{}, err
	}
	teamQ := h.db.WithContext(ctx).Model(&model.Agent{}).Where("org_id = ? AND agent_catalog_id = ? AND status <> ?", orgID, c.ID, "archived")
	if !orgWide {
		teamQ = teamQ.Where("team_id IN (?)", visibleTeamSubquery(h.db, userID))
	}
	var teamIDs []uuid.UUID
	if err := teamQ.Distinct("team_id").Order("team_id").Pluck("team_id", &teamIDs).Error; err != nil {
		return agentCatalogResponse{}, err
	}
	installedTeams := make([]string, 0, len(teamIDs))
	for _, id := range teamIDs {
		installedTeams = append(installedTeams, id.String())
	}
	return agentCatalogResponse{ID: c.ID.String(), Slug: c.Slug, Name: c.Name, Description: c.Description, Category: c.Category, AvatarURL: c.AvatarURL, Developer: c.Developer, Official: c.Official, IsDefault: c.IsDefault, Model: c.Model, SandboxImage: model.NormalizeSandboxImage(c.SandboxImage), RequiredConnections: required, InstalledAgentID: installedID, InstalledTeamIDs: installedTeams}, nil
}

func (h *AgentHandler) loadAgentCatalogBySlug(w http.ResponseWriter, r *http.Request, slug string) (model.AgentCatalog, bool) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "agent catalog slug required"})
		return model.AgentCatalog{}, false
	}
	var catalog model.AgentCatalog
	err := h.db.WithContext(r.Context()).Where("slug = ? AND status = ?", slug, model.AgentCatalogStatusActive).First(&catalog).Error
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
