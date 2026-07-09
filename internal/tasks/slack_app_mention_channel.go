package tasks

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/slackapp"
	"github.com/usehivy/hivy/internal/slackworkflow"
)

var errSlackChannelNotConfigured = errors.New("slack channel is not linked to a hivy team")

const slackChannelNotConfiguredMessage = "This Slack channel isn't connected to a Hivy team yet. An admin can link it to a team from the Hivy dashboard to enable the assistant here."

func (h *SlackAppMentionHandler) resolveChannelAndAgent(ctx context.Context, row *model.SlackThreadEvent, client slackapp.Client, token string) (model.Channel, model.Agent, error) {
	if row.SessionID != nil && *row.SessionID != uuid.Nil {
		session, err := h.loadActiveSlackSession(ctx, row.OrgID, *row.SessionID)
		if err != nil {
			return model.Channel{}, model.Agent{}, fmt.Errorf("load slack continuation session: %w", err)
		}
		channel, err := h.loadSlackSessionChannel(ctx, row.OrgID, session.ChannelID)
		if err != nil {
			return model.Channel{}, model.Agent{}, err
		}
		agent, err := h.loadAgent(ctx, row.OrgID, session.AgentID)
		if err != nil {
			return model.Channel{}, model.Agent{}, err
		}
		row.ChannelID = &channel.ID
		_ = slackworkflow.RecordChannelResolved(ctx, h.db, row.ID, channel.ID)
		return channel, agent, nil
	}
	channel, found, err := h.findSlackChannel(ctx, *row)
	if err != nil {
		return model.Channel{}, model.Agent{}, err
	}
	if row.TriggerID != nil && *row.TriggerID != uuid.Nil {
		if !found {
			return model.Channel{}, model.Agent{}, fmt.Errorf("slack trigger channel not found")
		}
		agent, err := h.loadSlackTriggerAgent(ctx, row.OrgID, *row.TriggerID, channel.ID)
		return channel, agent, err
	}
	if found {
		agent, err := h.routeChannelAgent(ctx, row, channel)
		return channel, agent, err
	}
	if err := h.replyChannelNotConfigured(ctx, row, client); err != nil {
		return model.Channel{}, model.Agent{}, err
	}
	return model.Channel{}, model.Agent{}, errSlackChannelNotConfigured
}

func (h *SlackAppMentionHandler) replyChannelNotConfigured(ctx context.Context, row *model.SlackThreadEvent, client slackapp.Client) error {
	replyTS, err := slackapp.PostThreadReply(ctx, client, row.SlackChannelID, row.ThreadTS, slackChannelNotConfiguredMessage)
	if err != nil {
		return fmt.Errorf("post slack channel-not-configured notice: %w", err)
	}
	if err := slackworkflow.RecordReplySent(ctx, h.db, row.ID, replyTS); err != nil {
		return err
	}
	return nil
}

func (h *SlackAppMentionHandler) loadSlackTriggerAgent(ctx context.Context, orgID, triggerID, channelID uuid.UUID) (model.Agent, error) {
	var trigger model.AgentTrigger
	err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND channel_id = ? AND enabled = true", triggerID, orgID, channelID).
		Where("trigger_key = ?", slackapp.EventReactionAdded).
		First(&trigger).Error
	if err != nil {
		return model.Agent{}, fmt.Errorf("load slack trigger: %w", err)
	}
	return h.loadAgent(ctx, orgID, trigger.AgentID)
}

func (h *SlackAppMentionHandler) findSlackChannel(ctx context.Context, row model.SlackThreadEvent) (model.Channel, bool, error) {
	var channel model.Channel
	err := h.db.WithContext(ctx).
		Where("org_id = ? AND origin = ? AND external_provider = ?", row.OrgID, "external", slackapp.Provider).
		Where("external_connection_id = ? AND external_resource_type = ?", row.ConnectionID, "slack_channel").
		Where("external_resource_key = ? AND archived_at IS NULL", row.SlackChannelID).
		First(&channel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Channel{}, false, nil
	}
	if err != nil {
		return model.Channel{}, false, fmt.Errorf("load slack channel: %w", err)
	}
	_ = slackworkflow.RecordChannelResolved(ctx, h.db, row.ID, channel.ID)
	return channel, true, nil
}

func (h *SlackAppMentionHandler) findOrCreateSlackSession(ctx context.Context, row *model.SlackThreadEvent, channel model.Channel, agent model.Agent) (model.Session, error) {
	if row.SessionID != nil && *row.SessionID != uuid.Nil {
		session, err := h.loadActiveSlackSession(ctx, row.OrgID, *row.SessionID)
		if err != nil {
			return model.Session{}, fmt.Errorf("load slack continuation session: %w", err)
		}
		_ = slackworkflow.RecordSessionResolved(ctx, h.db, row.ID, session.ID)
		return session, nil
	}
	key := slackSessionResourceKey(*row)
	var session model.Session
	err := h.db.WithContext(ctx).
		Where("org_id = ? AND source = ? AND source_id = ? AND source_resource_key = ? AND status = ?",
			row.OrgID, model.SessionSourceExternal, row.ConnectionID, key, "active").
		First(&session).Error
	if err == nil {
		_ = slackworkflow.RecordSessionResolved(ctx, h.db, row.ID, session.ID)
		return session, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Session{}, fmt.Errorf("load slack session: %w", err)
	}
	connID := row.ConnectionID
	session = model.Session{
		ID:                stableSlackSessionID(*row),
		OrgID:             row.OrgID,
		ChannelID:         channel.ID,
		AgentID:           agent.ID,
		Model:             agent.Model,
		ReasoningEffort:   "high",
		Source:            model.SessionSourceExternal,
		SourceID:          &connID,
		SourceResourceKey: key,
		// Name is left empty so Slack sessions follow the same naming strategy
		// as web/API sessions: no name up front, then an async LLM title
		// generated from the first message (enqueued after delivery commits it).
		Status:            "active",
		AgentTurnStatus:   model.SessionAgentTurnIdle,
		IntegrationScopes: model.JSON{},
	}
	if err := h.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&session).Error; err != nil {
		return model.Session{}, fmt.Errorf("create slack session: %w", err)
	}
	if err := h.db.WithContext(ctx).First(&session, "id = ?", session.ID).Error; err != nil {
		return model.Session{}, fmt.Errorf("reload slack session: %w", err)
	}
	_ = slackworkflow.RecordSessionResolved(ctx, h.db, row.ID, session.ID)
	return session, nil
}

func (h *SlackAppMentionHandler) loadActiveSlackSession(ctx context.Context, orgID, sessionID uuid.UUID) (model.Session, error) {
	var session model.Session
	err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND source = ? AND status = ?", sessionID, orgID, model.SessionSourceExternal, "active").
		First(&session).Error
	if err != nil {
		return model.Session{}, err
	}
	return session, nil
}

func (h *SlackAppMentionHandler) loadSlackSessionChannel(ctx context.Context, orgID, channelID uuid.UUID) (model.Channel, error) {
	var channel model.Channel
	err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND origin = ? AND external_provider = ? AND archived_at IS NULL",
			channelID, orgID, "external", slackapp.Provider).
		First(&channel).Error
	if err != nil {
		return model.Channel{}, fmt.Errorf("load slack session channel: %w", err)
	}
	return channel, nil
}

func slackSessionResourceKey(row model.SlackThreadEvent) string {
	parts := []string{slackapp.Provider, row.ConnectionID.String(), row.TeamID, row.SlackChannelID, row.ThreadTS}
	if row.TriggerID != nil && *row.TriggerID != uuid.Nil {
		parts = append(parts, "trigger", row.TriggerID.String())
	}
	return strings.Join(parts, ":")
}

func stableSlackSessionID(row model.SlackThreadEvent) uuid.UUID {
	key := "hivy:slack-session:" + slackSessionResourceKey(row)
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(key))
}
