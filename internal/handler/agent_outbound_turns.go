package handler

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

func (h *AgentOutboundWebhookHandler) completeSessionTurn(ctx context.Context, session *model.Session, payload map[string]any) {
	if h == nil || h.db == nil || session == nil || session.ID == uuid.Nil {
		return
	}
	turnID := stringValue(payload, "turn_id")
	query := h.db.WithContext(ctx).Model(&model.Session{}).
		Where("id = ? AND agent_turn_status = ?", session.ID, model.SessionAgentTurnActive)
	if turnID != "" {
		query = query.Where("(agent_turn_id = '' OR agent_turn_id = ?)", turnID)
	}
	res := query.Updates(map[string]any{
		"agent_turn_status":     model.SessionAgentTurnIdle,
		"agent_turn_id":         "",
		"agent_stream_id":       "",
		"agent_turn_started_at": nil,
	})
	if res.Error != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("release completed session turn: %w", res.Error), map[string]any{
			"session_id": session.ID.String(),
			"turn_id":    turnID,
		})
		return
	}
	if res.RowsAffected == 0 {
		return
	}
	if !h.hasPendingSessionMessages(ctx, session) {
		return
	}
	if err := tasks.EnqueueSessionMessageDeliver(ctx, h.enqueuer, session.ID); err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("enqueue queued session message after turn completion: %w", err), map[string]any{
			"session_id": session.ID.String(),
			"turn_id":    turnID,
		})
	}
}

func (h *AgentOutboundWebhookHandler) hasPendingSessionMessages(ctx context.Context, session *model.Session) bool {
	if session == nil || session.ID == uuid.Nil {
		return false
	}
	var count int64
	if err := h.db.WithContext(ctx).Model(&model.SessionMessageQueue{}).
		Where("session_id = ? AND status <> ?", session.ID, "delivered").
		Count(&count).Error; err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("count queued session messages after turn completion: %w", err), map[string]any{
			"session_id": session.ID.String(),
		})
		return false
	}
	return count > 0
}
