package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type agentCatalogPluginSummary struct {
	ID        string `json:"id"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
}

type agentCatalogResponse struct {
	ID                 string                      `json:"id"`
	Slug               string                      `json:"slug"`
	Name               string                      `json:"name"`
	Description        string                      `json:"description"`
	Category           string                      `json:"category"`
	AvatarURL          string                      `json:"avatar_url"`
	Developer          string                      `json:"developer"`
	Official           bool                        `json:"official"`
	IsDefault          bool                        `json:"is_default"`
	Model              string                      `json:"model"`
	AvailableModels    []string                    `json:"available_models"`
	SandboxStrategy    string                      `json:"sandbox_strategy"`
	SandboxImage       string                      `json:"sandbox_image"`
	RequiredPlugins    []agentCatalogPluginSummary `json:"required_plugins"`
	RecommendedPlugins []agentCatalogPluginSummary `json:"recommended_plugins"`
	InstalledAgentID   *string                     `json:"installed_agent_id,omitempty"`
}

type agentCatalogInstallConflictResponse struct {
	Error          string                      `json:"error"`
	MissingPlugins []agentCatalogPluginSummary `json:"missing_plugins"`
}

// ListCatalog handles GET /v1/agents/catalog.
// @Summary List agent catalog
// @Description Returns active agent catalog entries for the current organization, including install state and required plugin state.
// @Tags agents
// @Produce json
// @Success 200 {array} agentCatalogResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Router /v1/agents/catalog [get]
func (h *AgentHandler) ListCatalog(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	var catalog []model.AgentCatalog
	if err := h.db.WithContext(r.Context()).
		Where("status = ?", model.AgentCatalogStatusActive).
		Order("name ASC").
		Find(&catalog).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list agent catalog"})
		return
	}
	resp := make([]agentCatalogResponse, 0, len(catalog))
	for _, item := range catalog {
		out, err := h.toAgentCatalogResponse(r.Context(), org.ID, item)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load agent catalog details"})
			return
		}
		resp = append(resp, out)
	}
	writeJSON(w, http.StatusOK, resp)
}

// GetCatalog handles GET /v1/agents/catalog/{slug}.
// @Summary Get agent catalog entry
// @Description Returns one active agent catalog entry by slug for the current organization, including required plugin install state.
// @Tags agents
// @Produce json
// @Param slug path string true "Agent catalog slug"
// @Success 200 {object} agentCatalogResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Router /v1/agents/catalog/{slug} [get]
func (h *AgentHandler) GetCatalog(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	catalog, ok := h.loadAgentCatalogBySlug(w, r, chi.URLParam(r, "slug"))
	if !ok {
		return
	}
	resp, err := h.toAgentCatalogResponse(r.Context(), org.ID, catalog)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load agent catalog details"})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// InstallCatalog handles POST /v1/agents/catalog/{slug}/install.
// @Summary Install catalog agent
// @Description Installs an agent catalog entry into the current organization when required plugins are installed.
// @Tags agents
// @Produce json
// @Param slug path string true "Agent catalog slug"
// @Success 201 {object} agentMutationResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} agentCatalogInstallConflictResponse
// @Failure 500 {object} errorResponse
// @Router /v1/agents/catalog/{slug}/install [post]
func (h *AgentHandler) InstallCatalog(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	catalog, ok := h.loadAgentCatalogBySlug(w, r, chi.URLParam(r, "slug"))
	if !ok {
		return
	}
	missing, err := h.missingInstalledPlugins(r.Context(), org.ID, catalog.RequiredPlugins)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to validate required plugins"})
		return
	}
	if len(missing) > 0 {
		writeJSON(w, http.StatusConflict, agentCatalogInstallConflictResponse{
			Error:          "required plugins are not installed",
			MissingPlugins: missing,
		})
		return
	}
	var agent model.Agent
	err = h.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		existing, found, err := activeAgentForCatalog(r.Context(), tx, org.ID, catalog.ID)
		if err != nil {
			return err
		}
		if found {
			agent = existing
			return nil
		}
		created, err := h.createCatalogAgent(r.Context(), tx, org.ID, catalog)
		if err != nil {
			return err
		}
		agent = created
		return enableRequiredCatalogPlugins(r.Context(), tx, org.ID, agent.ID, catalog.RequiredPlugins)
	})
	if err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "agent name already exists"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to install agent"})
		return
	}
	if err := h.db.WithContext(r.Context()).Preload("AgentCatalog").First(&agent, "id = ?", agent.ID).Error; err == nil {
		writeJSON(w, http.StatusCreated, agentMutationResponse{Agent: h.agentListItem(r.Context(), org.ID, agent)})
		return
	}
	writeJSON(w, http.StatusCreated, agentMutationResponse{Agent: h.agentListItem(r.Context(), org.ID, agent)})
}

// UninstallCatalog handles DELETE /v1/agents/catalog/{slug}/install.
// @Summary Uninstall catalog agent
// @Description Archives the installed agent for a catalog entry in the current organization.
// @Tags agents
// @Produce json
// @Param slug path string true "Agent catalog slug"
// @Success 200 {object} statusResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Router /v1/agents/catalog/{slug}/install [delete]
func (h *AgentHandler) UninstallCatalog(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	catalog, ok := h.loadAgentCatalogBySlug(w, r, chi.URLParam(r, "slug"))
	if !ok {
		return
	}
	err := h.db.WithContext(r.Context()).Model(&model.Agent{}).
		Where("org_id = ? AND agent_catalog_id = ? AND status <> ?", org.ID, catalog.ID, "archived").
		Update("status", "archived").Error
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to uninstall agent"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "uninstalled"})
}
