package tasks

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func (h *EmployeeTriggerDispatchHandler) findOrCreateTriggerConversation(ctx context.Context, agent *model.Employee, sb *model.Sandbox, triggerID uuid.UUID, resourceKey, conversationID string) (*model.EmployeeSession, error) {
	var conv model.EmployeeSession
	err := h.db.WithContext(ctx).
		Where("org_id = ? AND employee_id = ? AND source = ? AND source_id = ? AND source_resource_key = ? AND status = ?",
			*agent.OrgID, agent.ID, triggerConversationSource, triggerID, resourceKey, "active").
		First(&conv).Error
	if err == nil {
		return &conv, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("load trigger conversation: %w", err)
	}

	conv = model.EmployeeSession{
		OrgID:                 *agent.OrgID,
		EmployeeID:            agent.ID,
		SandboxID:             sb.ID,
		RuntimeConversationID: conversationID,
		Source:                triggerConversationSource,
		SourceID:              &triggerID,
		SourceResourceKey:     resourceKey,
		Status:                "active",
		Name:                  "Trigger: " + resourceKey,
	}
	if err := h.db.WithContext(ctx).Create(&conv).Error; err != nil {
		return nil, fmt.Errorf("create trigger conversation: %w", err)
	}
	h.enqueueTriggerConversationMemoryRetain(ctx, conv, "trigger_session_created", "trigger.session.created")
	return &conv, nil
}

func (h *EmployeeTriggerDispatchHandler) enqueueTriggerConversationMemoryRetain(ctx context.Context, conv model.EmployeeSession, reason, sourceEvent string) {
	if h == nil || h.enqueuer == nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("trigger employee memory retain: enqueuer missing"), triggerEmployeeMemoryRetainFields(conv, reason, sourceEvent))
		return
	}
	payload := EmployeeMemoryRetainPayload{
		EmployeeID:        conv.EmployeeID,
		SandboxID:         conv.SandboxID,
		EmployeeSessionID: conv.ID,
		SessionID:         conv.RuntimeConversationID,
		Reason:            reason,
		SourceEvent:       sourceEvent,
	}
	duplicate, err := EnqueueEmployeeMemoryRetain(ctx, h.enqueuer, payload)
	if err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("trigger employee memory retain: enqueue: %w", err), triggerEmployeeMemoryRetainFields(conv, reason, sourceEvent))
		return
	}
	logging.FromContext(ctx).InfoContext(ctx, "trigger employee memory retain enqueued",
		"org_id", conv.OrgID.String(),
		"employee_id", conv.EmployeeID.String(),
		"sandbox_id", conv.SandboxID.String(),
		"employee_session_id", conv.ID.String(),
		"runtime_conversation_id", conv.RuntimeConversationID,
		"reason", reason,
		"source_event", sourceEvent,
		"delay_seconds", int(EmployeeMemoryRetainDelay.Seconds()),
		"duplicate", duplicate,
	)
}

func triggerEmployeeMemoryRetainFields(conv model.EmployeeSession, reason, sourceEvent string) map[string]any {
	return map[string]any{
		"org_id":                  conv.OrgID.String(),
		"employee_id":             conv.EmployeeID.String(),
		"sandbox_id":              conv.SandboxID.String(),
		"employee_session_id":     conv.ID.String(),
		"runtime_conversation_id": conv.RuntimeConversationID,
		"reason":                  reason,
		"source_event":            sourceEvent,
	}
}
