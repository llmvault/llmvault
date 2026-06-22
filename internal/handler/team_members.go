package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

type TeamHandler struct {
	db *gorm.DB
}

func NewTeamHandler(db *gorm.DB) *TeamHandler {
	return &TeamHandler{db: db}
}

type teamMemberRequest struct {
	Role *string `json:"role,omitempty"`
}

// @Summary Add or update a team member
// @Description Adds an existing organization member to a team or updates their team role. Admin-only.
// @Tags teams
// @Accept json
// @Produce json
// @Param id path string true "Team ID"
// @Param userID path string true "User ID"
// @Param body body teamMemberRequest false "Team member parameters"
// @Success 200 {object} teamDetailResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/teams/{id}/members/{userID} [put]
func (h *TeamHandler) PutMember(w http.ResponseWriter, r *http.Request) {
	team, ok := h.loadTeamForRequest(w, r)
	if !ok {
		return
	}
	targetUserID, ok := teamUserIDFromRequest(w, r)
	if !ok {
		return
	}
	if !userBelongsToOrg(r.Context(), h.db, team.OrgID, targetUserID) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "user must belong to this org"})
		return
	}
	req := teamMemberRequest{}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	role := defaultString(cleanStringPtr(req.Role), "member")
	if !validTeamMemberRole(role) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "role must be owner or member"})
		return
	}
	member := model.TeamMember{OrgID: team.OrgID, TeamID: team.ID, UserID: targetUserID, Role: role}
	if err := h.db.WithContext(r.Context()).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "team_id"}, {Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"role", "updated_at"}),
	}).Create(&member).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to update team member"})
		return
	}
	writeJSON(w, http.StatusOK, teamDetailResponse{
		Team:    h.teamResponse(r.Context(), team),
		Members: h.teamMembers(r.Context(), team.ID),
	})
}

// @Summary Remove a team member
// @Description Removes an organization member from a team. Admin-only.
// @Tags teams
// @Produce json
// @Param id path string true "Team ID"
// @Param userID path string true "User ID"
// @Success 200 {object} teamDetailResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/teams/{id}/members/{userID} [delete]
func (h *TeamHandler) DeleteMember(w http.ResponseWriter, r *http.Request) {
	team, ok := h.loadTeamForRequest(w, r)
	if !ok {
		return
	}
	targetUserID, ok := teamUserIDFromRequest(w, r)
	if !ok {
		return
	}
	if err := h.db.WithContext(r.Context()).
		Where("team_id = ? AND user_id = ?", team.ID, targetUserID).
		Delete(&model.TeamMember{}).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to remove team member"})
		return
	}
	writeJSON(w, http.StatusOK, teamDetailResponse{
		Team:    h.teamResponse(r.Context(), team),
		Members: h.teamMembers(r.Context(), team.ID),
	})
}

func teamUserIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	userID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid user id"})
		return uuid.Nil, false
	}
	return userID, true
}

func (h *TeamHandler) teamMembers(ctx context.Context, teamID uuid.UUID) []teamMemberResponse {
	var members []model.TeamMember
	_ = h.db.WithContext(ctx).
		Preload("User").
		Where("team_id = ?", teamID).
		Order("created_at ASC").
		Find(&members).Error
	out := make([]teamMemberResponse, len(members))
	for i, member := range members {
		out[i] = teamMemberResponse{
			UserID:    member.UserID.String(),
			Email:     member.User.Email,
			Name:      member.User.Name,
			Role:      member.Role,
			CreatedAt: member.CreatedAt.Format(time.RFC3339),
		}
	}
	return out
}

func userBelongsToOrg(ctx context.Context, db *gorm.DB, orgID, userID uuid.UUID) bool {
	var count int64
	_ = db.WithContext(ctx).
		Model(&model.OrgMembership{}).
		Where("org_id = ? AND user_id = ?", orgID, userID).
		Count(&count).Error
	return count == 1
}
