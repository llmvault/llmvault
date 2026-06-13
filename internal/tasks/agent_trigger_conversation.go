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

func (h *AgentTriggerDispatchHandler) findOrCreateTriggerConversation(ctx context.Context, agent *model.Agent, sb *model.Sandbox, triggerID uuid.UUID, resourceKey, conversationID string) (*model.AgentSession, error) {
	var conv model.AgentSession
	err := h.db.WithContext(ctx).
		Where("org_id = ? AND agent_id = ? AND source = ? AND source_resource_key = ? AND status = ?",
			*agent.OrgID, agent.ID, triggerConversationSource, resourceKey, "active").
		First(&conv).Error
	if err == nil {
		return &conv, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("load trigger conversation: %w", err)
	}

	conv = model.AgentSession{
		OrgID:                 *agent.OrgID,
		AgentID:               agent.ID,
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

func (h *AgentTriggerDispatchHandler) enqueueTriggerConversationMemoryRetain(ctx context.Context, conv model.AgentSession, reason, sourceEvent string) {
	if h == nil || h.enqueuer == nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("trigger agent memory retain: enqueuer missing"), triggerAgentMemoryRetainFields(conv, reason, sourceEvent))
		return
	}
	payload := AgentMemoryRetainPayload{
		AgentID:        conv.AgentID,
		SandboxID:      conv.SandboxID,
		AgentSessionID: conv.ID,
		SessionID:      conv.RuntimeConversationID,
		Reason:         reason,
		SourceEvent:    sourceEvent,
	}
	duplicate, err := EnqueueAgentMemoryRetain(ctx, h.enqueuer, payload)
	if err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("trigger agent memory retain: enqueue: %w", err), triggerAgentMemoryRetainFields(conv, reason, sourceEvent))
		return
	}
	logging.FromContext(ctx).InfoContext(ctx, "trigger agent memory retain enqueued",
		"org_id", conv.OrgID.String(),
		"agent_id", conv.AgentID.String(),
		"sandbox_id", conv.SandboxID.String(),
		"agent_session_id", conv.ID.String(),
		"runtime_conversation_id", conv.RuntimeConversationID,
		"reason", reason,
		"source_event", sourceEvent,
		"delay_seconds", int(AgentMemoryRetainDelay.Seconds()),
		"duplicate", duplicate,
	)
}

func triggerAgentMemoryRetainFields(conv model.AgentSession, reason, sourceEvent string) map[string]any {
	return map[string]any{
		"org_id":                  conv.OrgID.String(),
		"agent_id":                conv.AgentID.String(),
		"sandbox_id":              conv.SandboxID.String(),
		"agent_session_id":        conv.ID.String(),
		"runtime_conversation_id": conv.RuntimeConversationID,
		"reason":                  reason,
		"source_event":            sourceEvent,
	}
}
