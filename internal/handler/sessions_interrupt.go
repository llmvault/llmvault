package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

type sessionInterruptResponse struct {
	SessionID   string `json:"session_id"`
	Interrupted bool   `json:"interrupted"`
	Status      string `json:"status"`
}

// Interrupt handles POST /v1/sessions/{id}/interrupt.
// @Summary Interrupt a running session turn
// @Description Requests the sandbox runtime to stop the active agent turn for this session.
// @Tags sessions
// @Produce json
// @Param id path string true "Session ID"
// @Success 200 {object} sessionInterruptResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sessions/{id}/interrupt [post]
func (h *SessionHandler) Interrupt(w http.ResponseWriter, r *http.Request) {
	session, _, ok := h.authorizeSession(w, r, true)
	if !ok {
		return
	}
	if session.SandboxID == nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "session sandbox is not available yet"})
		return
	}
	var sb model.Sandbox
	if err := h.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ?", *session.SandboxID, session.OrgID).
		First(&sb).Error; err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "session sandbox is not available yet"})
		return
	}
	client, err := h.runtimeClientForSessionSandbox(r.Context(), &sb)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "session interrupt is not available"})
		return
	}
	interrupt, err := client.InterruptSession(r.Context(), session.ID.String())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to interrupt session"})
		return
	}
	h.releaseSessionTurnAfterInterrupt(r.Context(), session.ID)
	writeJSON(w, http.StatusOK, sessionInterruptResponse{
		SessionID:   session.ID.String(),
		Interrupted: interrupt.Interrupted,
		Status:      interrupt.Status,
	})
}

func (h *SessionHandler) runtimeClientForSessionSandbox(ctx context.Context, sb *model.Sandbox) (*agentruntime.Client, error) {
	if h.orchestrator != nil {
		return h.orchestrator.GetRuntimeClient(ctx, sb)
	}
	key := h.runtimeEncKey
	if key == nil {
		key = h.compileDeps.EncKey
	}
	if key == nil {
		return nil, errors.New("runtime encryption key is not configured")
	}
	runtimeSecret, err := key.DecryptString(sb.EncryptedRuntimeSecret)
	if err != nil {
		return nil, fmt.Errorf("decrypt runtime secret: %w", err)
	}
	return agentruntime.NewClient(sb.RuntimeURL, runtimeSecret), nil
}

func (h *SessionHandler) releaseSessionTurnAfterInterrupt(ctx context.Context, sessionID uuid.UUID) {
	res := h.db.WithContext(ctx).Model(&model.Session{}).
		Where("id = ? AND agent_turn_status = ?", sessionID, model.SessionAgentTurnActive).
		Updates(map[string]any{
			"agent_turn_status":     model.SessionAgentTurnIdle,
			"agent_turn_id":         "",
			"agent_stream_id":       "",
			"agent_turn_started_at": nil,
		})
	if res.Error != nil || res.RowsAffected == 0 {
		return
	}
	var pending int64
	if err := h.db.WithContext(ctx).Model(&model.SessionMessageQueue{}).
		Where("session_id = ? AND status <> ?", sessionID, "delivered").
		Count(&pending).Error; err != nil || pending == 0 {
		return
	}
	if h.enqueuer != nil {
		_ = tasks.EnqueueSessionMessageDeliver(ctx, h.enqueuer, sessionID)
	}
}
