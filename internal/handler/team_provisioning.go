package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/teamprovision"
)

// TeamProvisioningHandler serves the org-admin endpoints that provision plugins
// and knowledge (RAG) sources to a team. The admin gate (RequireOrgAdmin) is
// applied by the route layer around Mount; the handler still resolves the org
// from context for tenancy and defensively org-scopes every lookup.
type TeamProvisioningHandler struct {
	db *gorm.DB
}

// NewTeamProvisioningHandler constructs the handler.
func NewTeamProvisioningHandler(db *gorm.DB) *TeamProvisioningHandler {
	return &TeamProvisioningHandler{db: db}
}

// Mount registers the provisioning sub-routes on r. The route layer MUST call
// this inside a group already scoped to /v1/orgs/current and wrapped with the
// RequireOrgAdmin middleware, e.g.:
//
//	r.Route("/v1/orgs/current", func(r chi.Router) {
//	    r.Group(func(r chi.Router) {
//	        r.Use(mw.RequireOrgAdmin)
//	        teamProvisioningHandler.Mount(r)
//	    })
//	})
func (h *TeamProvisioningHandler) Mount(r chi.Router) {
	r.Post("/teams/{teamID}/plugins", h.EnableTeamPlugin)
	r.Delete("/teams/{teamID}/plugins/{pluginID}", h.DisableTeamPlugin)
	r.Get("/teams/{teamID}/rag-sources", h.ListTeamRagSources)
	r.Post("/teams/{teamID}/rag-sources", h.GrantTeamRagSource)
	r.Delete("/teams/{teamID}/rag-sources/{sourceID}", h.RevokeTeamRagSource)
}

// MountReadable registers the provisioning read routes that a plain member of
// the team may reach. The route layer mounts this OUTSIDE the RequireOrgAdmin
// group; each handler enforces team-membership visibility itself.
func (h *TeamProvisioningHandler) MountReadable(r chi.Router) {
	r.Get("/teams/{teamID}/plugins", h.ListTeamPlugins)
}

// ListTeamPlugins handles GET /v1/orgs/current/teams/{teamID}/plugins.
// @Summary List a team's enabled plugins
// @Description Lists the plugins enabled for a team. Visible to org managers, API keys, and members of that team; other members receive 404.
// @Tags team-provisioning
// @Produce json
// @Param teamID path string true "Team ID"
// @Success 200 {object} teamPluginsResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/teams/{teamID}/plugins [get]
func (h *TeamProvisioningHandler) ListTeamPlugins(w http.ResponseWriter, r *http.Request) {
	_, team, ok := h.loadReadableTeam(w, r)
	if !ok {
		return
	}
	h.respondTeamPlugins(w, r, team.ID, http.StatusOK)
}

// EnableTeamPlugin handles POST /v1/orgs/current/teams/{teamID}/plugins.
// @Summary Enable a plugin for a team
// @Description Adds a plugin to a team's allowlist. The plugin must belong to the org and be installed. Admin-only.
// @Tags team-provisioning
// @Accept json
// @Produce json
// @Param teamID path string true "Team ID"
// @Param body body enableTeamPluginRequest true "Plugin to enable"
// @Success 201 {object} teamPluginsResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 422 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/teams/{teamID}/plugins [post]
func (h *TeamProvisioningHandler) EnableTeamPlugin(w http.ResponseWriter, r *http.Request) {
	org, team, ok := h.loadTeam(w, r)
	if !ok {
		return
	}
	var req enableTeamPluginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	pluginID, err := uuid.Parse(req.PluginID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid plugin_id"})
		return
	}
	already, err := teamprovision.PluginEnabledForTeam(r.Context(), h.db, team.ID, pluginID)
	if err != nil {
		h.fail(w, r, "failed to check team plugin", err)
		return
	}
	if already {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "plugin already enabled for team"})
		return
	}
	if err := teamprovision.EnablePlugin(r.Context(), h.db, org.ID, team.ID, pluginID, h.actingUser(r)); err != nil {
		if h.writeProvisionError(w, err) {
			return
		}
		h.fail(w, r, "failed to enable team plugin", err)
		return
	}
	h.respondTeamPlugins(w, r, team.ID, http.StatusCreated)
}

// DisableTeamPlugin handles DELETE /v1/orgs/current/teams/{teamID}/plugins/{pluginID}.
// @Summary Disable a plugin for a team
// @Description Removes a plugin from a team's allowlist. Idempotent. Admin-only.
// @Tags team-provisioning
// @Produce json
// @Param teamID path string true "Team ID"
// @Param pluginID path string true "Plugin ID"
// @Success 200 {object} teamPluginsResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/teams/{teamID}/plugins/{pluginID} [delete]
func (h *TeamProvisioningHandler) DisableTeamPlugin(w http.ResponseWriter, r *http.Request) {
	org, team, ok := h.loadTeam(w, r)
	if !ok {
		return
	}
	pluginID, err := uuid.Parse(chi.URLParam(r, "pluginID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid plugin id"})
		return
	}
	if err := teamprovision.DisablePlugin(r.Context(), h.db, org.ID, team.ID, pluginID); err != nil {
		if h.writeProvisionError(w, err) {
			return
		}
		h.fail(w, r, "failed to disable team plugin", err)
		return
	}
	h.respondTeamPlugins(w, r, team.ID, http.StatusOK)
}

// ListTeamRagSources handles GET /v1/orgs/current/teams/{teamID}/rag-sources.
// @Summary List a team's knowledge sources
// @Description Lists the RAG sources granted to a team. A team's agents may search only these sources. Admin-only.
// @Tags team-provisioning
// @Produce json
// @Param teamID path string true "Team ID"
// @Success 200 {object} teamRagSourcesResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/teams/{teamID}/rag-sources [get]
func (h *TeamProvisioningHandler) ListTeamRagSources(w http.ResponseWriter, r *http.Request) {
	_, team, ok := h.loadTeam(w, r)
	if !ok {
		return
	}
	h.respondTeamRagSources(w, r, team.ID, http.StatusOK)
}

// GrantTeamRagSource handles POST /v1/orgs/current/teams/{teamID}/rag-sources.
// @Summary Grant a knowledge source to a team
// @Description Adds a RAG source to a team's allowlist. The source must belong to the org. Admin-only.
// @Tags team-provisioning
// @Accept json
// @Produce json
// @Param teamID path string true "Team ID"
// @Param body body grantTeamRagSourceRequest true "Source to grant"
// @Success 201 {object} teamRagSourcesResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/teams/{teamID}/rag-sources [post]
func (h *TeamProvisioningHandler) GrantTeamRagSource(w http.ResponseWriter, r *http.Request) {
	org, team, ok := h.loadTeam(w, r)
	if !ok {
		return
	}
	var req grantTeamRagSourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	sourceID, err := uuid.Parse(req.RagSourceID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid rag_source_id"})
		return
	}
	already, err := teamprovision.SourceGrantedToTeam(r.Context(), h.db, team.ID, sourceID)
	if err != nil {
		h.fail(w, r, "failed to check team rag source", err)
		return
	}
	if already {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "source already granted to team"})
		return
	}
	if err := teamprovision.GrantSource(r.Context(), h.db, org.ID, team.ID, sourceID, h.actingUser(r)); err != nil {
		if h.writeProvisionError(w, err) {
			return
		}
		h.fail(w, r, "failed to grant team rag source", err)
		return
	}
	h.respondTeamRagSources(w, r, team.ID, http.StatusCreated)
}

// RevokeTeamRagSource handles DELETE /v1/orgs/current/teams/{teamID}/rag-sources/{sourceID}.
// @Summary Revoke a knowledge source from a team
// @Description Removes a RAG source from a team's allowlist. Idempotent. Admin-only.
// @Tags team-provisioning
// @Produce json
// @Param teamID path string true "Team ID"
// @Param sourceID path string true "RAG source ID"
// @Success 200 {object} teamRagSourcesResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/teams/{teamID}/rag-sources/{sourceID} [delete]
func (h *TeamProvisioningHandler) RevokeTeamRagSource(w http.ResponseWriter, r *http.Request) {
	org, team, ok := h.loadTeam(w, r)
	if !ok {
		return
	}
	sourceID, err := uuid.Parse(chi.URLParam(r, "sourceID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid source id"})
		return
	}
	if err := teamprovision.RevokeSource(r.Context(), h.db, org.ID, team.ID, sourceID); err != nil {
		if h.writeProvisionError(w, err) {
			return
		}
		h.fail(w, r, "failed to revoke team rag source", err)
		return
	}
	h.respondTeamRagSources(w, r, team.ID, http.StatusOK)
}
