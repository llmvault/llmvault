package tasks

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func runtimeMessageFromEvent(sessionID uuid.UUID, event model.SessionEvent) agentruntime.HTTPMessageRequest {
	payload := map[string]any{}
	for key, value := range event.Payload {
		payload[key] = value
	}
	text, _ := payload["text"].(string)
	user, _ := payload["user"].(string)
	display, _ := payload["user_display_name"].(string)
	if user == "" && event.ActorUserID != nil {
		user = event.ActorUserID.String()
	}
	return agentruntime.HTTPMessageRequest{
		Text:            strings.TrimSpace(text),
		SessionID:       sessionID.String(),
		User:            strings.TrimSpace(user),
		UserDisplayName: strings.TrimSpace(display),
		DynamicContext:  stringSlice(payload["dynamic_context"]),
		Raw:             payload,
	}
}

func stringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		if typed, ok := value.([]string); ok {
			return append([]string(nil), typed...)
		}
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

func (h *SessionMessageDeliverHandler) markDelivered(ctx context.Context, queueID uuid.UUID, delivery *agentruntime.HTTPMessageResponse) error {
	now := time.Now()
	updates := map[string]any{
		"status":       "delivered",
		"delivered_at": now,
		"leased_by":    "",
		"leased_until": nil,
		"last_error":   "",
	}
	if delivery != nil {
		updates["runtime_stream_id"] = strings.TrimSpace(delivery.StreamID)
		updates["runtime_stream_url"] = strings.TrimSpace(delivery.StreamURL)
		updates["runtime_trace_id"] = strings.TrimSpace(delivery.TraceID)
		updates["runtime_turn_id"] = strings.TrimSpace(delivery.TurnID)
	}
	err := h.db.WithContext(ctx).Model(&model.SessionMessageQueue{}).
		Where("id = ?", queueID).
		Updates(updates).Error
	if err != nil {
		return fmt.Errorf("mark session message delivered: %w", err)
	}
	return nil
}

func (h *SessionMessageDeliverHandler) releaseClaim(ctx context.Context, queueID uuid.UUID, cause error) error {
	msg := ""
	if cause != nil {
		msg = cause.Error()
		if len(msg) > 1000 {
			msg = msg[:1000]
		}
	}
	return h.db.WithContext(ctx).Model(&model.SessionMessageQueue{}).
		Where("id = ? AND status = ?", queueID, "processing").
		Updates(map[string]any{
			"status":       "pending",
			"leased_by":    "",
			"leased_until": nil,
			"last_error":   msg,
		}).Error
}

func (h *SessionMessageDeliverHandler) hasPending(ctx context.Context, sessionID uuid.UUID) bool {
	var count int64
	if err := h.db.WithContext(ctx).Model(&model.SessionMessageQueue{}).
		Where("session_id = ? AND status <> ?", sessionID, "delivered").
		Count(&count).Error; err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("count pending session messages: %w", err), map[string]any{
			"session_id": sessionID.String(),
		})
		return false
	}
	return count > 0
}
