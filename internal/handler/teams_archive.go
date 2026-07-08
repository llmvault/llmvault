package handler

import (
	"net/http"
	"time"

	"github.com/usehivy/hivy/internal/model"
)

// @Summary Archive a team
// @Description Archives an active team after all channels are removed from it. Admin-only. Rejected if it is the org's last team.
// @Tags teams
// @Produce json
// @Param id path string true "Team ID"
// @Success 200 {object} teamMutationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/teams/{id} [delete]
func (h *TeamHandler) Archive(w http.ResponseWriter, r *http.Request) {
	team, ok := h.loadTeamForRequest(w, r)
	if !ok {
		return
	}
	// An organization must always keep at least one team (the floor that
	// guarantees every org has a self-sufficient Hivy + #general).
	var teamCount int64
	if err := h.db.WithContext(r.Context()).
		Model(&model.Team{}).
		Where("org_id = ? AND archived_at IS NULL", team.OrgID).
		Count(&teamCount).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to check org teams"})
		return
	}
	if teamCount <= 1 {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "an organization must keep at least one team"})
		return
	}
	var channelCount int64
	if err := h.db.WithContext(r.Context()).
		Model(&model.Channel{}).
		Where("org_id = ? AND team_id = ? AND archived_at IS NULL", team.OrgID, team.ID).
		Count(&channelCount).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to check team channels"})
		return
	}
	if channelCount > 0 {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "remove channels from this team before archiving it"})
		return
	}
	now := time.Now()
	if err := h.db.WithContext(r.Context()).
		Model(&model.Team{}).
		Where("id = ? AND org_id = ?", team.ID, team.OrgID).
		Update("archived_at", &now).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to archive team"})
		return
	}
	team.ArchivedAt = &now
	writeJSON(w, http.StatusOK, teamMutationResponse{Team: h.teamResponse(r.Context(), team)})
}
