package tasks

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/slackapp"
	"github.com/usehivy/hivy/internal/slackworkflow"
)

func (h *SlackAppMentionHandler) resolveChannelAndAgent(ctx context.Context, row *model.SlackThreadEvent, client slackapp.Client, token string) (model.Channel, model.Agent, error) {
	channel, found, err := h.findSlackChannel(ctx, *row)
	if err != nil {
		return model.Channel{}, model.Agent{}, err
	}
	if found {
		agent, err := h.loadAgent(ctx, channel.OrgID, channel.DefaultAgentID)
		return channel, agent, err
	}
	agent, err := h.ensureOrgAgent(ctx, row.OrgID)
	if err != nil {
		return model.Channel{}, model.Agent{}, err
	}
	info, err := h.slackChannelInfo(ctx, client, token, row.SlackChannelID)
	if err != nil {
		return model.Channel{}, model.Agent{}, err
	}
	channel, err = h.createSlackChannel(ctx, *row, agent, info)
	if err != nil {
		return model.Channel{}, model.Agent{}, err
	}
	return channel, agent, nil
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

func (h *SlackAppMentionHandler) slackChannelInfo(ctx context.Context, client slackapp.Client, token, channelID string) (slackapp.Channel, error) {
	info, err := slackapp.GetChannelInfo(ctx, client, channelID)
	if err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("slack channel info: %w", err), map[string]any{
			"slack_channel_id": channelID,
		})
		return slackapp.Channel{}, fmt.Errorf("slack channel info: %w", err)
	}
	if info.ID == "" {
		info.ID = channelID
	}
	if !info.IsPrivate && !info.IsMember {
		if joined, err := slackapp.JoinChannel(ctx, token, channelID); err == nil && joined.ID != "" {
			info = joined
		} else if err != nil {
			logging.CaptureWithFields(ctx, fmt.Errorf("slack channel join: %w", err), map[string]any{
				"slack_channel_id": channelID,
			})
			return slackapp.Channel{}, fmt.Errorf("slack channel join: %w", err)
		}
	}
	if strings.TrimSpace(info.Name) == "" {
		info.Name = channelID
	}
	return info, nil
}

func (h *SlackAppMentionHandler) createSlackChannel(ctx context.Context, row model.SlackThreadEvent, agent model.Agent, info slackapp.Channel) (model.Channel, error) {
	connID := row.ConnectionID
	channel := model.Channel{
		OrgID:                row.OrgID,
		Name:                 slackChannelName(info.Name),
		Kind:                 "standard",
		Visibility:           "public",
		DefaultAgentID:       agent.ID,
		Origin:               "external",
		ExternalProvider:     slackapp.Provider,
		ExternalConnectionID: &connID,
		ExternalWorkspaceKey: row.Connection.NangoConnectionID,
		ExternalResourceType: "slack_channel",
		ExternalResourceKey:  row.SlackChannelID,
		ExternalResourceName: info.Name,
		ExternalMetadata: model.JSON{
			"team_id":                 row.TeamID,
			"is_private":              info.IsPrivate,
			"auto_created_from":       "slack_app_mention",
			"slack_thread_event_id":   row.ID.String(),
			"slack_app_mention_event": row.EventID,
		},
	}
	result := h.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&channel)
	if result.Error != nil {
		return model.Channel{}, fmt.Errorf("create slack channel: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		found, ok, err := h.findSlackChannel(ctx, row)
		if err != nil {
			return model.Channel{}, err
		}
		if ok {
			return found, nil
		}
		return model.Channel{}, fmt.Errorf("slack channel create conflicted but no external channel was found")
	}
	_ = slackworkflow.RecordChannelResolved(ctx, h.db, row.ID, channel.ID)
	return channel, nil
}

func (h *SlackAppMentionHandler) findOrCreateSlackSession(ctx context.Context, row *model.SlackThreadEvent, channel model.Channel, agent model.Agent) (model.Session, error) {
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
		ID:                stableSlackSessionID(row.ConnectionID, row.TeamID, row.SlackChannelID, row.ThreadTS),
		OrgID:             row.OrgID,
		ChannelID:         channel.ID,
		AgentID:           agent.ID,
		Model:             agent.Model,
		ReasoningEffort:   "high",
		Source:            model.SessionSourceExternal,
		SourceID:          &connID,
		SourceResourceKey: key,
		Name:              "Slack: " + channel.ExternalResourceName,
		Status:            "active",
		AgentTurnStatus:   model.SessionAgentTurnIdle,
		IntegrationScopes: model.JSON{},
	}
	if strings.TrimSpace(session.Name) == "Slack:" {
		session.Name = "Slack: " + row.SlackChannelID
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

func slackChannelName(name string) string {
	name = normalizeSlackName(name)
	if strings.HasPrefix(name, "slack-") {
		return name
	}
	return "slack-" + name
}

func normalizeSlackName(raw string) string {
	value := strings.TrimLeft(strings.ToLower(strings.TrimSpace(raw)), "#")
	if strings.ContainsAny(value, " \t\n\r") {
		value = strings.Join(strings.Fields(value), "-")
	}
	if value == "" {
		return "channel"
	}
	return value
}

func slackSessionResourceKey(row model.SlackThreadEvent) string {
	return strings.Join([]string{slackapp.Provider, row.ConnectionID.String(), row.TeamID, row.SlackChannelID, row.ThreadTS}, ":")
}

func stableSlackSessionID(connectionID uuid.UUID, teamID, channelID, threadTS string) uuid.UUID {
	key := "hivy:slack-session:" + connectionID.String() + ":" + teamID + ":" + channelID + ":" + threadTS
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(key))
}
