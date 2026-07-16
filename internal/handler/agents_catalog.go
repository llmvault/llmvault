package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// snapshotCatalogInstructions stores a fallback copy of the current catalog
// prompt. Clones with nil Instructions still prefer the live catalog prompt.
func snapshotCatalogInstructions(instructions string) *string {
	trimmed := strings.TrimSpace(instructions)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

type agentCatalogConnectionSummary struct {
	Provider string `json:"provider"`
}

type agentCatalogResponse struct {
	ID                  string                          `json:"id"`
	Slug                string                          `json:"slug"`
	Name                string                          `json:"name"`
	Description         string                          `json:"description"`
	Category            string                          `json:"category"`
	AvatarURL           string                          `json:"avatar_url"`
	Developer           string                          `json:"developer"`
	Official            bool                            `json:"official"`
	IsDefault           bool                            `json:"is_default"`
	Model               string                          `json:"model"`
	SandboxImage        string                          `json:"sandbox_image"`
	RequiredConnections []agentCatalogConnectionSummary `json:"required_connections"`
	InstalledAgentID    *string                         `json:"installed_agent_id,omitempty"`
	// InstalledTeamIDs lists the ids of the caller's visible teams that already
	// have this catalog agent installed (one clone per team). Managers and
	// API-key callers see every team in the org; a plain member sees only the
	// teams they belong to. The UI uses it to render per-team install/uninstall.
	InstalledTeamIDs []string `json:"installed_team_ids"`
}

// agentCatalogInstallRequest is the body for install/uninstall: the team the
// catalog agent is being cloned into (install) or removed from (uninstall).
type agentCatalogInstallRequest struct {
	TeamID *string `json:"team_id"`
}

// agentCatalogMissingConnectionsResponse reports required providers unavailable to the team.
type agentCatalogMissingConnectionsResponse struct {
	Error              string   `json:"error"`
	MissingConnections []string `json:"missing_connections"`
}

// ListCatalog handles GET /v1/agents/catalog.
// @Summary List agent catalog
// @Description Returns active agent catalog entries, install state, and required connections.
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
// @Description Returns one active agent catalog entry and its required connections.
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
// @Summary Install catalog agent into a team
// @Description Clones a catalog agent into the caller's team when all required connections are granted.
// @Tags agents
// @Produce json
// @Param slug path string true "Agent catalog slug"
// @Param body body agentCatalogInstallRequest true "Target team"
// @Success 201 {object} agentMutationResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 422 {object} agentCatalogMissingConnectionsResponse
// @Failure 500 {object} errorResponse
// @Router /v1/agents/catalog/{slug}/install [post]
func (h *AgentHandler) InstallCatalog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	catalog, ok := h.loadAgentCatalogBySlug(w, r, chi.URLParam(r, "slug"))
	if !ok {
		return
	}
	teamID, ok := h.resolveCatalogInstallTeam(ctx, w, r, org.ID)
	if !ok {
		return
	}
	// A missing required connection refuses the install; no agent is created.
	missing, err := h.teamMissingRequiredConnections(ctx, org.ID, *teamID, catalog.RequiredConnections)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to validate required connections"})
		return
	}
	if len(missing) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, agentCatalogMissingConnectionsResponse{
			Error:              "your team is missing required connections",
			MissingConnections: missing,
		})
		return
	}
	var agent model.Agent
	err = h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		existing, found, err := activeAgentForCatalogTeam(ctx, tx, org.ID, catalog.ID, *teamID)
		if err != nil {
			return err
		}
		if found {
			agent = existing
			return nil
		}
		created, err := h.createCatalogAgent(ctx, tx, org.ID, *teamID, catalog)
		if err != nil {
			return err
		}
		agent = created
		return nil
	})
	if err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "agent name already exists"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to install agent"})
		return
	}
	_ = h.db.WithContext(ctx).Preload("AgentCatalog").First(&agent, "id = ?", agent.ID).Error
	item, err := h.agentListItem(ctx, org.ID, agent)
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "load installed agent response", "error", err, "agent_id", agent.ID, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load agent"})
		return
	}
	writeJSON(w, http.StatusCreated, agentMutationResponse{Agent: item})
}

// UninstallCatalog handles DELETE /v1/agents/catalog/{slug}/install.
// @Summary Uninstall a team's catalog agent
// @Description Archives the catalog agent clone belonging to the caller's team. The actor must be able to manage the team.
// @Tags agents
// @Produce json
// @Param slug path string true "Agent catalog slug"
// @Param team_id query string false "Target team (may also be sent in the JSON body)"
// @Success 200 {object} statusResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Router /v1/agents/catalog/{slug}/install [delete]
func (h *AgentHandler) UninstallCatalog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	catalog, ok := h.loadAgentCatalogBySlug(w, r, chi.URLParam(r, "slug"))
	if !ok {
		return
	}
	if catalog.IsDefault {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "default agent cannot be uninstalled"})
		return
	}
	teamID, ok := h.resolveCatalogInstallTeam(ctx, w, r, org.ID)
	if !ok {
		return
	}
	var agents []model.Agent
	if err := h.db.WithContext(ctx).
		Where("is_default = ?", false).
		Where("org_id = ? AND agent_catalog_id = ? AND team_id = ? AND status <> ?", org.ID, catalog.ID, *teamID, "archived").
		Find(&agents).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to uninstall agent"})
		return
	}
	if len(agents) == 0 {
		writeJSON(w, http.StatusOK, map[string]string{"status": "uninstalled"})
		return
	}
	// Route each clone through the SAME archive routine as DELETE /agents/{id}
	// (busy-turn check, channel-default re-pointing, session/sandbox teardown,
	// trigger disable) rather than a divergent bulk status update — so uninstall
	// can't strand a channel default or rip out a live session's sandbox.
	for i := range agents {
		if err := h.archiveAgentWithSessions(ctx, org.ID, &agents[i]); err != nil {
			switch {
			case errors.Is(err, errAgentBusy):
				writeJSON(w, http.StatusConflict, errorResponse{Error: "an agent has sessions with an in-progress turn; try again once it finishes"})
			case errors.Is(err, errAgentChannelDefaultUnresolved):
				writeJSON(w, http.StatusConflict, errorResponse{Error: "an agent is a channel's default agent and no replacement Hivy agent could be found; re-point those channels first"})
			default:
				writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to uninstall agent"})
			}
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "uninstalled"})
}
