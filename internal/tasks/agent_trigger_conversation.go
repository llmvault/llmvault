package tasks

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

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
