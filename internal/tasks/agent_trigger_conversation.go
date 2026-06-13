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

func (h *AgentTriggerDispatchHandler) findOrCreateTriggerSession(ctx context.Context, agent *model.Agent, sb *model.Sandbox, triggerID uuid.UUID, resourceKey string) (*model.Session, error) {
	var session model.Session
	err := h.db.WithContext(ctx).
		Where("org_id = ? AND agent_id = ? AND source = ? AND source_resource_key = ? AND status = ?",
			*agent.OrgID, agent.ID, triggerConversationSource, resourceKey, "active").
		First(&session).Error
	if err == nil {
		return &session, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("load trigger session: %w", err)
	}

	channelID, err := h.ensureTriggerChannel(ctx, agent)
	if err != nil {
		return nil, err
	}
	session = model.Session{
		ID:                stableTriggerSessionID(triggerID, resourceKey),
		OrgID:             *agent.OrgID,
		ChannelID:         channelID,
		AgentID:           agent.ID,
		SandboxID:         &sb.ID,
		Source:            triggerConversationSource,
		SourceID:          &triggerID,
		SourceResourceKey: resourceKey,
		Status:            "active",
		Name:              "Trigger: " + resourceKey,
		IntegrationScopes: model.JSON{},
	}
	if err := h.db.WithContext(ctx).Create(&session).Error; err != nil {
		return nil, fmt.Errorf("create trigger session: %w", err)
	}
	h.enqueueTriggerSessionMemoryRetain(ctx, session, "trigger_session_created", "trigger.session.created")
	return &session, nil
}

func (h *AgentTriggerDispatchHandler) ensureTriggerChannel(ctx context.Context, agent *model.Agent) (uuid.UUID, error) {
	name := "triggers-" + shortTaskUUID(agent.ID)
	channel := model.Channel{}
	scope := model.Channel{OrgID: *agent.OrgID, Origin: "native", Name: name}
	attrs := model.Channel{
		Description:      "Trigger-originated agent activity",
		Kind:             "system",
		Visibility:       "private",
		DefaultAgentID:   agent.ID,
		ExternalMetadata: model.JSON{"source": triggerConversationSource},
	}
	if err := h.db.WithContext(ctx).Where(&scope).Attrs(attrs).FirstOrCreate(&channel).Error; err != nil {
		return uuid.Nil, fmt.Errorf("ensure trigger channel: %w", err)
	}
	return channel.ID, nil
}

func stableTriggerSessionID(triggerID uuid.UUID, resourceKey string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("hivy:trigger-session:"+triggerID.String()+":"+resourceKey))
}

func shortTaskUUID(id uuid.UUID) string {
	value := id.String()
	if len(value) < 8 {
		return value
	}
	return value[:8]
}

func (h *AgentTriggerDispatchHandler) enqueueTriggerSessionMemoryRetain(ctx context.Context, session model.Session, reason, sourceEvent string) {
	if h == nil || h.enqueuer == nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("trigger agent memory retain: enqueuer missing"), triggerAgentMemoryRetainFields(session, reason, sourceEvent))
		return
	}
	if session.SandboxID == nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("trigger agent memory retain: session sandbox missing"), triggerAgentMemoryRetainFields(session, reason, sourceEvent))
		return
	}
	payload := AgentMemoryRetainPayload{
		AgentID:     session.AgentID,
		SandboxID:   *session.SandboxID,
		SessionUUID: session.ID,
		SessionID:   session.ID.String(),
		Reason:      reason,
		SourceEvent: sourceEvent,
	}
	duplicate, err := EnqueueAgentMemoryRetain(ctx, h.enqueuer, payload)
	if err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("trigger agent memory retain: enqueue: %w", err), triggerAgentMemoryRetainFields(session, reason, sourceEvent))
		return
	}
	logging.FromContext(ctx).InfoContext(ctx, "trigger agent memory retain enqueued",
		"org_id", session.OrgID.String(),
		"agent_id", session.AgentID.String(),
		"sandbox_id", session.SandboxID.String(),
		"session_id", session.ID.String(),
		"reason", reason,
		"source_event", sourceEvent,
		"delay_seconds", int(AgentMemoryRetainDelay.Seconds()),
		"duplicate", duplicate,
	)
}

func triggerAgentMemoryRetainFields(session model.Session, reason, sourceEvent string) map[string]any {
	sandboxID := ""
	if session.SandboxID != nil {
		sandboxID = session.SandboxID.String()
	}
	return map[string]any{
		"org_id":       session.OrgID.String(),
		"agent_id":     session.AgentID.String(),
		"sandbox_id":   sandboxID,
		"session_id":   session.ID.String(),
		"reason":       reason,
		"source_event": sourceEvent,
	}
}
