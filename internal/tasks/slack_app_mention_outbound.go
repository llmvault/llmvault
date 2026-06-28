package tasks

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/slackapp"
)

func (h *SlackAppMentionHandler) recordSlackOutbound(ctx context.Context, inbound *model.SlackThreadEvent, session model.Session, text, replyTS string) error {
	messageAt, err := slackapp.ParseTimestamp(replyTS)
	if err != nil {
		messageAt = time.Now().UTC()
	}
	now := time.Now().UTC()
	row := model.SlackThreadEvent{
		OrgID:          inbound.OrgID,
		ConnectionID:   inbound.ConnectionID,
		ChannelID:      inbound.ChannelID,
		TriggerID:      inbound.TriggerID,
		SessionID:      &session.ID,
		TeamID:         inbound.TeamID,
		SlackChannelID: inbound.SlackChannelID,
		ThreadTS:       inbound.ThreadTS,
		MessageTS:      strings.TrimSpace(replyTS),
		MessageAt:      messageAt,
		EventType:      slackResponseEventType(inbound),
		Direction:      model.SlackThreadEventDirectionOutbound,
		SenderID:       "hivy",
		Text:           strings.TrimSpace(text),
		Status:         model.SlackThreadEventStatusSent,
		SlackReplyTS:   strings.TrimSpace(replyTS),
		Raw: model.JSON{
			"inbound_slack_thread_event_id": inbound.ID.String(),
			"inbound_event_id":              inbound.EventID,
		},
		ReceivedAt:       now,
		SlackReplySentAt: slackTimePtr(now),
		CompletedAt:      slackTimePtr(now),
	}
	if err := h.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("record slack outbound: %w", err)
	}
	return nil
}

func slackResponseEventType(inbound *model.SlackThreadEvent) string {
	if inbound == nil {
		return "app_mention.response"
	}
	switch inbound.EventType {
	case slackapp.EventReactionAdded:
		return "reaction_added.response"
	case slackapp.EventMessage:
		return "message.response"
	default:
		return "app_mention.response"
	}
}

func slackTimePtr(t time.Time) *time.Time {
	return &t
}
