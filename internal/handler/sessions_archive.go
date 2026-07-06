package handler

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
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
	if !h.requireSessionIdleForArchive(w, session) {
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

// requireSessionIdleForArchive rejects a manual archive while the session's
// agent turn is in progress. Archiving tears down the sandbox, which would
// abort the live turn, so a session can only be archived when it is idle.
func (h *SessionHandler) requireSessionIdleForArchive(w http.ResponseWriter, session model.Session) bool {
	if session.AgentTurnStatus == model.SessionAgentTurnActive {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "session has an in-progress turn"})
		return false
	}
	return true
}

// canArchiveSession allows an org admin or a member of the session's channel to
// archive it — the same admin-or-member rule as channel access.
func (h *SessionHandler) canArchiveSession(ctx context.Context, session model.Session, userID *uuid.UUID) (bool, error) {
	channel, found, err := h.loadSessionChannel(ctx, session)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	return h.canViewChannel(ctx, channel, userID), nil
}

func (h *SessionHandler) archiveSession(ctx context.Context, session *model.Session) error {
	now := time.Now()
	updates := map[string]any{"status": "archived"}
	session.Status = "archived"
	if session.EndedAt == nil {
		updates["ended_at"] = &now
		session.EndedAt = &now
	}
	if err := h.db.WithContext(ctx).
		Model(&model.Session{}).
		Where("id = ? AND org_id = ?", session.ID, session.OrgID).
		Updates(updates).Error; err != nil {
		return err
	}
	// Final reflection pass over the session's unreflected event tail.
	if err := tasks.EnqueueSessionReflection(ctx, h.enqueuer, session.ID); err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("enqueue session reflection on archive: %w", err), map[string]any{
			"session_id": session.ID.String(),
		})
	}
	h.enqueueSessionSandboxTeardown(ctx, session)
	return nil
}

// enqueueSessionSandboxTeardown schedules deletion of the session's sandbox once
// the session has been archived. No-op when the session has no sandbox or no
// enqueuer is configured. Enqueue failures are logged, not surfaced: the DB
// archive already succeeded and the sandbox reaper is the backstop.
func (h *SessionHandler) enqueueSessionSandboxTeardown(ctx context.Context, session *model.Session) {
	if session == nil || session.SandboxID == nil {
		return
	}
	if err := tasks.EnqueueSandboxDelete(ctx, h.enqueuer, *session.SandboxID); err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("enqueue sandbox delete on session archive: %w", err), map[string]any{
			"session_id": session.ID.String(),
			"sandbox_id": session.SandboxID.String(),
		})
	}
}
