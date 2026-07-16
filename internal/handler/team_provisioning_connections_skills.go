package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/skillaccess"
	"github.com/usehivy/hivy/internal/teamprovision"
)

type teamConnectionRequest struct {
	ConnectionID string `json:"connection_id"`
}

type teamConnectionResponse struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Provider string `json:"provider"`
	Name     string `json:"name"`
}

type teamConnectionsResponse struct {
	Data []teamConnectionResponse `json:"data"`
}

type teamSkillRequest struct {
	SkillID string `json:"skill_id"`
}

type teamSkillResponse struct {
	ID      string   `json:"id"`
	Slug    string   `json:"slug"`
	Name    string   `json:"name"`
	Sources []string `json:"sources"`
}

type teamSkillsResponse struct {
	Data []teamSkillResponse `json:"data"`
}

// @Summary List a team's connections
// @Tags team-provisioning
// @Produce json
// @Param teamID path string true "Team ID"
// @Success 200 {object} teamConnectionsResponse
// @Router /v1/orgs/current/teams/{teamID}/connections [get]
func (h *TeamProvisioningHandler) ListTeamConnections(w http.ResponseWriter, r *http.Request) {
	org, team, ok := h.loadReadableTeam(w, r)
	if !ok {
		return
	}
	h.respondTeamConnections(w, r, org.ID, team.ID, http.StatusOK)
}

// @Summary Grant a connection to a team
// @Tags team-provisioning
// @Accept json
// @Produce json
// @Param teamID path string true "Team ID"
// @Param body body teamConnectionRequest true "Connection grant"
// @Success 201 {object} teamConnectionsResponse
// @Router /v1/orgs/current/teams/{teamID}/connections [post]
func (h *TeamProvisioningHandler) GrantTeamConnection(w http.ResponseWriter, r *http.Request) {
	org, team, ok := h.loadTeam(w, r)
	if !ok {
		return
	}
	var req teamConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	id, err := uuid.Parse(req.ConnectionID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid connection_id"})
		return
	}
	if err := teamprovision.GrantConnection(r.Context(), h.db, org.ID, team.ID, id, h.actingUser(r)); err != nil {
		if !h.writeProvisionError(w, err) {
			h.fail(w, r, "failed to grant team connection", err)
		}
		return
	}
	h.respondTeamConnections(w, r, org.ID, team.ID, http.StatusCreated)
}

// @Summary Revoke a connection from a team
// @Tags team-provisioning
// @Produce json
// @Param teamID path string true "Team ID"
// @Param connectionID path string true "Connection ID"
// @Success 200 {object} teamConnectionsResponse
// @Router /v1/orgs/current/teams/{teamID}/connections/{connectionID} [delete]
func (h *TeamProvisioningHandler) RevokeTeamConnection(w http.ResponseWriter, r *http.Request) {
	org, team, ok := h.loadTeam(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "connectionID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid connection id"})
		return
	}
	if err := teamprovision.RevokeConnection(r.Context(), h.db, org.ID, team.ID, id); err != nil {
		if !h.writeProvisionError(w, err) {
			h.fail(w, r, "failed to revoke team connection", err)
		}
		return
	}
	h.respondTeamConnections(w, r, org.ID, team.ID, http.StatusOK)
}

// @Summary List a team's effective skills
// @Tags team-provisioning
// @Produce json
// @Param teamID path string true "Team ID"
// @Success 200 {object} teamSkillsResponse
// @Router /v1/orgs/current/teams/{teamID}/skills [get]
func (h *TeamProvisioningHandler) ListTeamSkills(w http.ResponseWriter, r *http.Request) {
	org, team, ok := h.loadReadableTeam(w, r)
	if !ok {
		return
	}
	h.respondTeamSkills(w, r, org.ID, team.ID, http.StatusOK)
}

// @Summary Grant a skill to a team
// @Tags team-provisioning
// @Accept json
// @Produce json
// @Param teamID path string true "Team ID"
// @Param body body teamSkillRequest true "Skill grant"
// @Success 201 {object} teamSkillsResponse
// @Router /v1/orgs/current/teams/{teamID}/skills [post]
func (h *TeamProvisioningHandler) GrantTeamSkill(w http.ResponseWriter, r *http.Request) {
	org, team, ok := h.loadTeam(w, r)
	if !ok {
		return
	}
	var req teamSkillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	id, err := uuid.Parse(req.SkillID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid skill_id"})
		return
	}
	if err := teamprovision.GrantSkill(r.Context(), h.db, org.ID, team.ID, id, h.actingUser(r)); err != nil {
		if !h.writeProvisionError(w, err) {
			h.fail(w, r, "failed to grant team skill", err)
		}
		return
	}
	h.respondTeamSkills(w, r, org.ID, team.ID, http.StatusCreated)
}

// @Summary Revoke a skill from a team
// @Tags team-provisioning
// @Produce json
// @Param teamID path string true "Team ID"
// @Param skillID path string true "Skill ID"
// @Success 200 {object} teamSkillsResponse
// @Router /v1/orgs/current/teams/{teamID}/skills/{skillID} [delete]
func (h *TeamProvisioningHandler) RevokeTeamSkill(w http.ResponseWriter, r *http.Request) {
	org, team, ok := h.loadTeam(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "skillID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid skill id"})
		return
	}
	if err := teamprovision.RevokeSkill(r.Context(), h.db, org.ID, team.ID, id); err != nil {
		if !h.writeProvisionError(w, err) {
			h.fail(w, r, "failed to revoke team skill", err)
		}
		return
	}
	h.respondTeamSkills(w, r, org.ID, team.ID, http.StatusOK)
}

func (h *TeamProvisioningHandler) respondTeamConnections(w http.ResponseWriter, r *http.Request, orgID, teamID uuid.UUID, status int) {
	grants, err := teamprovision.ConnectionGrants(r.Context(), h.db, orgID, teamID)
	if err != nil {
		h.fail(w, r, "failed to load team connections", err)
		return
	}
	data := make([]teamConnectionResponse, 0, len(grants))
	for _, grant := range grants {
		if grant.ConnectionID != nil && grant.Connection != nil {
			data = append(data, teamConnectionResponse{ID: grant.Connection.ID.String(), Kind: model.ConnectionKindIntegration, Provider: grant.Connection.Integration.Provider, Name: grant.Connection.Name})
		}
		if grant.DatabaseConnectionID != nil && grant.DatabaseConnection != nil {
			data = append(data, teamConnectionResponse{ID: grant.DatabaseConnection.ID.String(), Kind: model.ConnectionKindDatabase, Provider: grant.DatabaseConnection.Provider, Name: grant.DatabaseConnection.Name})
		}
	}
	writeJSON(w, status, teamConnectionsResponse{Data: data})
}

func (h *TeamProvisioningHandler) respondTeamSkills(w http.ResponseWriter, r *http.Request, orgID, teamID uuid.UUID, status int) {
	effective, err := skillaccess.ResolveAgent(r.Context(), h.db, model.Agent{OrgID: &orgID, TeamID: teamID})
	if err != nil {
		h.fail(w, r, "failed to load team skills", err)
		return
	}
	data := make([]teamSkillResponse, 0, len(effective))
	for _, entry := range effective {
		data = append(data, teamSkillResponse{ID: entry.Skill.ID.String(), Slug: entry.Skill.Slug, Name: entry.Skill.Name, Sources: entry.Sources})
	}
	writeJSON(w, status, teamSkillsResponse{Data: data})
}
