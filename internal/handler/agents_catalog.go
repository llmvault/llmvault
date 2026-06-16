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
	SandboxStrategy    string                      `json:"sandbox_strategy"`
	RequiredPlugins    []agentCatalogPluginSummary `json:"required_plugins"`
	RecommendedPlugins []agentCatalogPluginSummary `json:"recommended_plugins"`
	InstalledAgentID   *string                     `json:"installed_agent_id,omitempty"`
}

type agentCatalogInstallConflictResponse struct {
	Error          string                      `json:"error"`
	MissingPlugins []agentCatalogPluginSummary `json:"missing_plugins"`
}

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
