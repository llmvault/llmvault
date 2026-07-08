package handler

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	pluginstore "github.com/usehivy/hivy/internal/plugins"
	ragmodel "github.com/usehivy/hivy/internal/rag/model"
	"github.com/usehivy/hivy/internal/teamprovision"
)

type teamPluginResponse struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type teamPluginsResponse struct {
	Data []teamPluginResponse `json:"data"`
}

type enableTeamPluginRequest struct {
	PluginID string `json:"plugin_id"`
}

type teamRagSourceResponse struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Status string `json:"status"`
}

type teamRagSourcesResponse struct {
	Data []teamRagSourceResponse `json:"data"`
}

type grantTeamRagSourceRequest struct {
	RagSourceID string `json:"rag_source_id"`
}

// loadTeam resolves the org from context and the {teamID} path param, and
// confirms the team is an active team in that org (404 otherwise).
func (h *TeamProvisioningHandler) loadTeam(w http.ResponseWriter, r *http.Request) (*model.Org, model.Team, bool) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return nil, model.Team{}, false
	}
	teamID, err := uuid.Parse(chi.URLParam(r, "teamID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid team id"})
		return nil, model.Team{}, false
	}
	var team model.Team
	err = h.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", teamID, org.ID).
		First(&team).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "team not found"})
		return nil, model.Team{}, false
	}
	if err != nil {
		h.fail(w, r, "failed to load team", err)
		return nil, model.Team{}, false
	}
	return org, team, true
}

// loadReadableTeam is loadTeam plus the team-visibility check that guards the
// member-readable routes: org managers and API keys pass, a plain member must
// belong to the team, and everyone else gets 404 (matching teamHandler.Get).
func (h *TeamProvisioningHandler) loadReadableTeam(w http.ResponseWriter, r *http.Request) (*model.Org, model.Team, bool) {
	org, team, ok := h.loadTeam(w, r)
	if !ok {
		return nil, model.Team{}, false
	}
	if !callerCanAccessTeam(r.Context(), h.db, org.ID, team.ID) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "team not found"})
		return nil, model.Team{}, false
	}
	return org, team, true
}

func (h *TeamProvisioningHandler) actingUser(r *http.Request) *uuid.UUID {
	id, _ := currentRequestUserID(r.Context())
	return id
}

// writeProvisionError maps teamprovision sentinels to HTTP responses. Returns
// true when it handled err.
func (h *TeamProvisioningHandler) writeProvisionError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, teamprovision.ErrTeamNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "team not found"})
	case errors.Is(err, teamprovision.ErrPluginNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "plugin not found in org"})
	case errors.Is(err, teamprovision.ErrSourceNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "knowledge source not found in org"})
	case errors.Is(err, teamprovision.ErrPluginNotInstalled):
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "plugin is not installed for this org"})
	case errors.Is(err, teamprovision.ErrPluginAlwaysEnabled):
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "plugin is always enabled and cannot be toggled per team"})
	default:
		return false
	}
	return true
}

func (h *TeamProvisioningHandler) respondTeamPlugins(w http.ResponseWriter, r *http.Request, teamID uuid.UUID, status int) {
	ids, err := teamprovision.EnabledPluginIDs(r.Context(), h.db, teamID)
	if err != nil {
		h.fail(w, r, "failed to load team plugins", err)
		return
	}
	var plugins []model.Plugin
	if len(ids) > 0 {
		if err := h.db.WithContext(r.Context()).
			Where("id IN ?", ids).Order("name").Find(&plugins).Error; err != nil {
			h.fail(w, r, "failed to load team plugins", err)
			return
		}
	}
	data := make([]teamPluginResponse, 0, len(plugins))
	for _, p := range plugins {
		// Auto-install system plugins are enabled for every team implicitly and
		// are not per-team provisionable; never list them as team-enabled even if
		// a stale/legacy team_plugins row exists.
		if pluginstore.PluginAutoInstall(p) {
			continue
		}
		data = append(data, teamPluginResponse{ID: p.ID.String(), Slug: p.Slug, Name: p.Name})
	}
	writeJSON(w, status, teamPluginsResponse{Data: data})
}

func (h *TeamProvisioningHandler) respondTeamRagSources(w http.ResponseWriter, r *http.Request, teamID uuid.UUID, status int) {
	ids, err := teamprovision.GrantedSourceIDs(r.Context(), h.db, teamID)
	if err != nil {
		h.fail(w, r, "failed to load team rag sources", err)
		return
	}
	var sources []ragmodel.RAGSource
	if len(ids) > 0 {
		if err := h.db.WithContext(r.Context()).
			Where("id IN ?", ids).Order("name").Find(&sources).Error; err != nil {
			h.fail(w, r, "failed to load team rag sources", err)
			return
		}
	}
	data := make([]teamRagSourceResponse, 0, len(sources))
	for _, s := range sources {
		data = append(data, teamRagSourceResponse{
			ID: s.ID.String(), Name: s.Name, Kind: string(s.KindValue), Status: string(s.Status),
		})
	}
	writeJSON(w, status, teamRagSourcesResponse{Data: data})
}

func (h *TeamProvisioningHandler) fail(w http.ResponseWriter, r *http.Request, msg string, err error) {
	logging.FromContext(r.Context()).ErrorContext(r.Context(), msg, "error", err)
	writeJSON(w, http.StatusInternalServerError, errorResponse{Error: msg})
}
