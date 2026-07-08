package handler

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/channelagents"
	"github.com/usehivy/hivy/internal/logging"
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
	AutoInstall          bool                          `json:"auto_install"`
	Locked               bool                          `json:"locked"`
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
	ID               string   `json:"id"`
	Slug             string   `json:"slug"`
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	HumanDescription *string  `json:"human_description,omitempty"`
	Category         string   `json:"category"`
	Tags             []string `json:"tags"`
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

// pluginActorScope carries whether the request sees agents org-wide (manager or
// API-key caller) and, for non-managers, the acting user id used to filter
// enabled_agent_ids down to agents the member may see.
type pluginActorScope struct {
	orgWide bool
	userID  *uuid.UUID
}

func (h *PluginHandler) actorScope(ctx context.Context, orgID uuid.UUID) (pluginActorScope, error) {
	orgWide, userID, err := actorSeesOrgWide(ctx, h.db, orgID)
	if err != nil {
		return pluginActorScope{}, err
	}
	return pluginActorScope{orgWide: orgWide, userID: userID}, nil
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
			ID:               skill.ID.String(),
			Slug:             skill.Slug,
			Name:             skill.Name,
			Description:      desc,
			HumanDescription: skill.HumanDescription,
			Category:         skill.Category,
			Tags:             []string(skill.Tags),
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

func (h *PluginHandler) loadPluginBySlug(w http.ResponseWriter, r *http.Request, slug string) (model.Plugin, bool) {
	if slug == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "plugin slug required"})
		return model.Plugin{}, false
	}
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return model.Plugin{}, false
	}
	// Org-owned plugins resolve only for their own org; other orgs get 404,
	// same as a nonexistent slug. Own-org plugins shadow nothing: the partial
	// unique indexes keep global and per-org slug spaces separate, so prefer
	// the org's plugin when both exist.
	var plugin model.Plugin
	if err := h.db.
		Where("slug = ? AND status = ? AND (org_id IS NULL OR org_id = ?)", slug, model.PluginStatusActive, org.ID).
		Order("org_id ASC NULLS LAST").
		First(&plugin).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "plugin not found"})
			return model.Plugin{}, false
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "load plugin by slug", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load plugin"})
		return model.Plugin{}, false
	}
	return plugin, true
}

func (h *PluginHandler) loadAgentFromRoute(w http.ResponseWriter, r *http.Request) (model.Org, model.Agent, bool) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return model.Org{}, model.Agent{}, false
	}
	scope, err := h.actorScope(r.Context(), org.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to resolve access"})
		return *org, model.Agent{}, false
	}
	agentID := chi.URLParam(r, "id")
	q := h.db.Preload("AgentCatalog").Where("id = ? AND org_id = ? AND status <> ?", agentID, org.ID, "archived")
	if !scope.orgWide {
		// A non-manager must not act on — or even confirm the existence of — an
		// agent they cannot see; treat it as not found.
		q = q.Where("id IN (?)", channelagents.VisibleAgentIDsSubquery(h.db, org.ID, scope.userID))
	}
	var agent model.Agent
	if err := q.First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "agent not found"})
			return *org, model.Agent{}, false
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "load agent from route", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load agent"})
		return *org, model.Agent{}, false
	}
	return *org, agent, true
}
