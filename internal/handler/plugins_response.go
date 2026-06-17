package handler

import (
	"context"
	"net/http"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type pluginResponse struct {
	ID                   string                        `json:"id"`
	Slug                 string                        `json:"slug"`
	Name                 string                        `json:"name"`
	Description          string                        `json:"description"`
	Category             string                        `json:"category"`
	DetailCategory       string                        `json:"detail_category"`
	Icon                 string                        `json:"icon"`
	IconColor            string                        `json:"icon_color"`
	Developer            string                        `json:"developer"`
	Official             bool                          `json:"official"`
	Featured             bool                          `json:"featured"`
	Capabilities         []string                      `json:"capabilities"`
	Examples             []string                      `json:"examples"`
	Links                *pluginLinksResponse          `json:"links,omitempty"`
	LongDescription      string                        `json:"long_description"`
	Version              string                        `json:"version"`
	Status               string                        `json:"status"`
	Skills               []pluginSkillResponse         `json:"skills"`
	RequiredConnections  []pluginConnectionRequirement `json:"required_connections"`
	ResourceRequirements []pluginResourceRequirement   `json:"resource_requirements"`
	Installed            bool                          `json:"installed"`
	MissingRequirements  []pluginConnectionRequirement `json:"missing_requirements,omitempty"`
	EnabledAgentIDs      []string                      `json:"enabled_agent_ids,omitempty"`
	CreatedAt            string                        `json:"created_at"`
	UpdatedAt            string                        `json:"updated_at"`
}

type pluginSkillResponse struct {
	ID          string   `json:"id"`
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Tags        []string `json:"tags"`
}

type pluginConnectionRequirement struct {
	Provider string `json:"provider"`
	Kind     string `json:"kind"`
	Required bool   `json:"required"`
}

type pluginResourceRequirement struct {
	Provider      string `json:"provider"`
	Kind          string `json:"kind"`
	Required      bool   `json:"required"`
	ConnectionID  string `json:"connection_id,omitempty"`
	ResourceKey   string `json:"resource_key"`
	DisplayName   string `json:"display_name"`
	Description   string `json:"description"`
	Selected      bool   `json:"selected"`
	SelectedCount int    `json:"selected_count"`
	Missing       bool   `json:"missing"`
}

type pluginLinksResponse struct {
	Website string `json:"website,omitempty"`
	Privacy string `json:"privacy,omitempty"`
	Terms   string `json:"terms,omitempty"`
}

type pluginInstallConflictResponse struct {
	Error               string                        `json:"error"`
	MissingRequirements []pluginConnectionRequirement `json:"missing_requirements"`
}

func (h *PluginHandler) toPluginResponse(ctx context.Context, orgID uuid.UUID, plugin model.Plugin) (pluginResponse, error) {
	var skills []model.Skill
	if err := h.db.WithContext(ctx).
		Where("plugin_id = ? AND status = ?", plugin.ID, model.SkillStatusPublished).
		Order("name ASC").
		Find(&skills).Error; err != nil {
		return pluginResponse{}, err
	}
	var reqs []model.PluginIntegration
	if err := h.db.WithContext(ctx).Where("plugin_id = ?", plugin.ID).Order("provider ASC").Find(&reqs).Error; err != nil {
		return pluginResponse{}, err
	}
	var installCount int64
	if err := h.db.WithContext(ctx).Model(&model.OrgPluginInstall{}).
		Where("org_id = ? AND plugin_id = ? AND revoked_at IS NULL", orgID, plugin.ID).
		Count(&installCount).Error; err != nil {
		return pluginResponse{}, err
	}
	var enabled []model.AgentPluginInstall
	if err := h.db.WithContext(ctx).Where("org_id = ? AND plugin_id = ?", orgID, plugin.ID).Find(&enabled).Error; err != nil {
		return pluginResponse{}, err
	}
	enabledIDs := make([]string, 0, len(enabled))
	for _, row := range enabled {
		enabledIDs = append(enabledIDs, row.AgentID.String())
	}
	missing, err := h.missingRequirements(ctx, orgID, plugin.ID)
	if err != nil {
		return pluginResponse{}, err
	}
	resourceRequirements, err := h.resourceRequirements(ctx, orgID, reqs, installCount > 0)
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

type pluginPresentation struct {
	DetailCategory  string
	Official        bool
	Featured        bool
	Capabilities    []string
	Examples        []string
	Links           *pluginLinksResponse
	LongDescription string
}

func toPluginSkillResponses(skills []model.Skill) []pluginSkillResponse {
	out := make([]pluginSkillResponse, 0, len(skills))
	for _, skill := range skills {
		desc := ""
		if skill.Description != nil {
			desc = *skill.Description
		}
		out = append(out, pluginSkillResponse{
			ID:          skill.ID.String(),
			Slug:        skill.Slug,
			Name:        skill.Name,
			Description: desc,
			Category:    skill.Category,
			Tags:        []string(skill.Tags),
		})
	}
	return out
}

func toPluginRequirementResponses(reqs []model.PluginIntegration) []pluginConnectionRequirement {
	out := make([]pluginConnectionRequirement, 0, len(reqs))
	for _, req := range reqs {
		out = append(out, pluginConnectionRequirement{
			Provider: req.Provider,
			Kind:     req.Kind,
			Required: req.Required,
		})
	}
	return out
}

func (h *PluginHandler) resourceRequirements(ctx context.Context, orgID uuid.UUID, reqs []model.PluginIntegration, installed bool) ([]pluginResourceRequirement, error) {
	cat := h.catalog
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
		conn, hasConnection, err := h.firstActiveConnectionForProvider(ctx, orgID, req.Provider)
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

func (h *PluginHandler) loadPluginBySlug(w http.ResponseWriter, r *http.Request, slug string) (model.Plugin, bool) {
	if slug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "plugin slug required"})
		return model.Plugin{}, false
	}
	var plugin model.Plugin
	if err := h.db.Where("slug = ? AND status = ?", slug, model.PluginStatusActive).First(&plugin).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "plugin not found"})
			return model.Plugin{}, false
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load plugin"})
		return model.Plugin{}, false
	}
	return plugin, true
}

func (h *PluginHandler) loadAgentFromRoute(w http.ResponseWriter, r *http.Request) (model.Org, model.Agent, bool) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return model.Org{}, model.Agent{}, false
	}
	agentID := chi.URLParam(r, "id")
	var agent model.Agent
	if err := h.db.Where("id = ? AND org_id = ? AND status <> ?", agentID, org.ID, "archived").First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return *org, model.Agent{}, false
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load agent"})
		return *org, model.Agent{}, false
	}
	return *org, agent, true
}
