package tasks

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

const triggerSystemChannelName = "system"

func (h *AgentTriggerDispatchHandler) findOrCreateTriggerSession(ctx context.Context, agent *model.Agent, trigger model.AgentTrigger, resourceKey string) (*model.Session, error) {
	channelID, err := h.resolveTriggerChannel(ctx, agent, trigger.ChannelID)
	if err != nil {
		return nil, err
	}
	var session model.Session
	err = h.db.WithContext(ctx).
		Where("org_id = ? AND agent_id = ? AND channel_id = ? AND source = ? AND source_resource_key = ? AND status = ?",
			*agent.OrgID, agent.ID, channelID, triggerConversationSource, resourceKey, "active").
		First(&session).Error
	if err == nil {
		return &session, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("load trigger session: %w", err)
	}

	session = model.Session{
		ID:                stableTriggerSessionID(trigger.ID, channelID, resourceKey),
		OrgID:             *agent.OrgID,
		ChannelID:         channelID,
		AgentID:           agent.ID,
		Model:             agent.Model,
		Source:            triggerConversationSource,
		SourceID:          &trigger.ID,
		SourceResourceKey: resourceKey,
		Status:            "active",
		Name:              "Trigger: " + resourceKey,
		IntegrationScopes: model.JSON{},
	}
	if err := h.db.WithContext(ctx).Create(&session).Error; err != nil {
		return nil, fmt.Errorf("create trigger session: %w", err)
	}
	return &session, nil
}

func (h *AgentTriggerDispatchHandler) resolveTriggerChannel(ctx context.Context, agent *model.Agent, configured *uuid.UUID) (uuid.UUID, error) {
	if configured == nil || *configured == uuid.Nil {
		return h.ensureTriggerSystemChannel(ctx, agent)
	}
	var channel model.Channel
	err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", *configured, *agent.OrgID).
		First(&channel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return uuid.Nil, fmt.Errorf("trigger channel not found")
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("load trigger channel: %w", err)
	}
	return channel.ID, nil
}

func (h *AgentTriggerDispatchHandler) ensureTriggerSystemChannel(ctx context.Context, agent *model.Agent) (uuid.UUID, error) {
	scope := model.Channel{OrgID: *agent.OrgID, Origin: "native", Name: triggerSystemChannelName}
	attrs := model.Channel{
		Description:      "System-managed jobs",
		Kind:             "system",
		Visibility:       "private",
		DefaultAgentID:   agent.ID,
		ExternalMetadata: model.JSON{"source": "system"},
	}
	var channel model.Channel
	if err := h.db.WithContext(ctx).
		Where(&scope).
		Where("archived_at IS NULL").
		Attrs(attrs).
		FirstOrCreate(&channel).Error; err != nil {
		return uuid.Nil, fmt.Errorf("ensure trigger system channel: %w", err)
	}
	return channel.ID, nil
}

func stableTriggerSessionID(triggerID, channelID uuid.UUID, resourceKey string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("hivy:trigger-session:"+triggerID.String()+":"+channelID.String()+":"+resourceKey))
}
