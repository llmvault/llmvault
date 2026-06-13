package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

type sessionParticipantRequest struct {
	Role string `json:"role,omitempty"`
}

// PutParticipant handles PUT /v1/sessions/{id}/participants/{userID}.
// @Summary Add a session participant
// @Description Invites an org member into a session.
// @Tags sessions
// @Produce json
// @Param id path string true "Session ID"
// @Param userID path string true "User ID"
// @Success 200 {object} sessionDetailResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sessions/{id}/participants/{userID} [put]
func (h *SessionHandler) PutParticipant(w http.ResponseWriter, r *http.Request) {
	session, inviter, ok := h.authorizeSession(w, r, true)
	if !ok {
		return
	}
	target, ok := sessionUserIDParam(w, r)
	if !ok {
		return
	}
	if !h.userInOrg(r.Context(), session.OrgID, target) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "user must belong to this org"})
		return
	}
	now := time.Now()
	participant := model.SessionParticipant{
		SessionID: session.ID,
		UserID:    target,
		Role:      "collaborator",
		InvitedBy: inviter,
		JoinedAt:  &now,
	}
	if err := h.db.WithContext(r.Context()).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "session_id"}, {Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"role", "invited_by", "joined_at"}),
	}).Create(&participant).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to update participant"})
		return
	}
	_ = h.writeSystemEvent(r, &session, inviter, "participant.joined", model.JSON{"user_id": target.String()})
	writeSessionDetail(w, r, h, session)
}

// DeleteParticipant handles DELETE /v1/sessions/{id}/participants/{userID}.
// @Summary Remove a session participant
// @Description Removes a participant from a session.
// @Tags sessions
// @Produce json
// @Param id path string true "Session ID"
// @Param userID path string true "User ID"
// @Success 200 {object} sessionDetailResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sessions/{id}/participants/{userID} [delete]
func (h *SessionHandler) DeleteParticipant(w http.ResponseWriter, r *http.Request) {
	session, actor, ok := h.authorizeSession(w, r, true)
	if !ok {
		return
	}
	target, ok := sessionUserIDParam(w, r)
	if !ok {
		return
	}
	if actor == nil || (*actor != target && !h.actorCanManageParticipants(r, session, actor)) {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "session access denied"})
		return
	}
	if h.removingLastSessionOwner(r, session.ID, target) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "cannot remove the last session owner"})
		return
	}
	if err := h.db.WithContext(r.Context()).
		Where("session_id = ? AND user_id = ?", session.ID, target).
		Delete(&model.SessionParticipant{}).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to remove participant"})
		return
	}
	_ = h.writeSystemEvent(r, &session, actor, "participant.left", model.JSON{"user_id": target.String()})
	writeSessionDetail(w, r, h, session)
}

func sessionUserIDParam(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	userID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid user id"})
		return uuid.Nil, false
	}
	return userID, true
}

func (h *SessionHandler) userInOrg(ctx context.Context, orgID, userID uuid.UUID) bool {
	var count int64
	_ = h.db.WithContext(ctx).Model(&model.OrgMembership{}).
		Where("org_id = ? AND user_id = ?", orgID, userID).
		Count(&count).Error
	return count == 1
}

func (h *SessionHandler) actorCanManageParticipants(r *http.Request, session model.Session, actor *uuid.UUID) bool {
	role, _ := h.orgRole(r.Context(), session.OrgID, actor)
	if isOrgManager(role) {
		return true
	}
	participantRole, _ := h.sessionParticipantRole(r.Context(), session.ID, actor)
	return participantRole == "owner"
}

func (h *SessionHandler) removingLastSessionOwner(r *http.Request, sessionID, userID uuid.UUID) bool {
	var target model.SessionParticipant
	if err := h.db.WithContext(r.Context()).
		Where("session_id = ? AND user_id = ?", sessionID, userID).
		First(&target).Error; err != nil || target.Role != "owner" {
		return false
	}
	var owners int64
	_ = h.db.WithContext(r.Context()).Model(&model.SessionParticipant{}).
		Where("session_id = ? AND role = ?", sessionID, "owner").
		Count(&owners).Error
	return owners <= 1
}

func writeSessionDetail(w http.ResponseWriter, r *http.Request, h *SessionHandler, session model.Session) {
	stats := h.statsForSessions(r.Context(), []uuid.UUID{session.ID})[session.ID]
	writeJSON(w, http.StatusOK, sessionDetailResponse{
		Session:      sessionToResponse(session, stats.ParticipantCount, stats.EventCount, stats.LastEvent),
		Participants: h.participants(r.Context(), session.ID),
	})
}
