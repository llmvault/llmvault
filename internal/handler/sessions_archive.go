package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// Archive handles DELETE /v1/sessions/{id}.
// @Summary Archive a session
// @Description Archives a session when the caller is a session member or owner.
// @Tags sessions
// @Produce json
// @Param id path string true "Session ID"
// @Success 200 {object} sessionMutationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sessions/{id} [delete]
func (h *SessionHandler) Archive(w http.ResponseWriter, r *http.Request) {
	session, userID, ok := h.authorizeSession(w, r, false)
	if !ok {
		return
	}
	if !h.requireSessionArchivePermission(w, r, session, userID) {
		return
	}
	if session.Status != "archived" || session.EndedAt == nil {
		if err := h.archiveSession(r.Context(), &session); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to archive session"})
			return
		}
	}
	stats := h.statsForSessions(r.Context(), []uuid.UUID{session.ID})[session.ID]
	writeJSON(w, http.StatusOK, sessionMutationResponse{
		Session: sessionToResponse(session, stats.ParticipantCount, stats.EventCount, stats.LastEvent),
	})
}

func (h *SessionHandler) requireSessionArchivePermission(w http.ResponseWriter, r *http.Request, session model.Session, userID *uuid.UUID) bool {
	allowed, err := h.canArchiveSession(r.Context(), session, userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to check session archive access"})
		return false
	}
	if !allowed {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "session access denied"})
		return false
	}
	return true
}

func (h *SessionHandler) canArchiveSession(ctx context.Context, session model.Session, userID *uuid.UUID) (bool, error) {
	if userID == nil {
		return false, nil
	}
	role, err := h.sessionParticipantRole(ctx, session.ID, userID)
	if err != nil {
		return false, err
	}
	return role == "owner" || role == "collaborator" || role == "member", nil
}

func (h *SessionHandler) archiveSession(ctx context.Context, session *model.Session) error {
	now := time.Now()
	updates := map[string]any{"status": "archived"}
	session.Status = "archived"
	if session.EndedAt == nil {
		updates["ended_at"] = &now
		session.EndedAt = &now
	}
	return h.db.WithContext(ctx).
		Model(&model.Session{}).
		Where("id = ? AND org_id = ?", session.ID, session.OrgID).
		Updates(updates).Error
}
