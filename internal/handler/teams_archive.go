package handler

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// @Summary Archive a team
// @Description Archives an active team. Admin-only. Rejected if it is the org's last team.
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
	// guarantees every org has a self-sufficient default Hivy agent).
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
	now := time.Now()
	// Archive the team AND cascade its provisioning in one tx so no orphaned,
	// still-active agent (the team's Hivy + catalog clones) is left org-listable
	// behind the archived team, and no stale connection/skill/RAG/team-member
	// grants survive.
	if err := h.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		var agentIDs []uuid.UUID
		if err := tx.Model(&model.Agent{}).
			Where("org_id = ? AND team_id = ? AND status <> ?", team.OrgID, team.ID, "archived").
			Pluck("id", &agentIDs).Error; err != nil {
			return err
		}
		if len(agentIDs) > 0 {
			if err := tx.Model(&model.Agent{}).
				Where("id IN ?", agentIDs).
				Update("status", "archived").Error; err != nil {
				return err
			}
			// Disable the archived agents' triggers (re-homed onto a surviving
			// default agent if one exists), so none fires behind an archived agent.
			if err := reassignAgentTriggersToDefault(tx, team.OrgID, agentIDs); err != nil {
				return err
			}
		}
		// Clear the team's provisioning rows.
		if err := tx.Where("org_id = ? AND team_id = ?", team.OrgID, team.ID).
			Delete(&model.TeamConnectionGrant{}).Error; err != nil {
			return err
		}
		if err := tx.Where("org_id = ? AND team_id = ?", team.OrgID, team.ID).
			Delete(&model.TeamSkillGrant{}).Error; err != nil {
			return err
		}
		if err := tx.Where("org_id = ? AND team_id = ?", team.OrgID, team.ID).
			Delete(&model.TeamRagSource{}).Error; err != nil {
			return err
		}
		if err := tx.Where("org_id = ? AND team_id = ?", team.OrgID, team.ID).
			Delete(&model.TeamExternalResourceRoute{}).Error; err != nil {
			return err
		}
		if err := tx.Where("team_id = ?", team.ID).
			Delete(&model.TeamMember{}).Error; err != nil {
			return err
		}
		return tx.Model(&model.Team{}).
			Where("id = ? AND org_id = ?", team.ID, team.OrgID).
			Update("archived_at", &now).Error
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to archive team"})
		return
	}
	team.ArchivedAt = &now
	writeJSON(w, http.StatusOK, teamMutationResponse{Team: h.teamResponse(r.Context(), team)})
}
