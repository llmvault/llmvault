package tasks

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/slackapp"
)

func (h *SlackAppMentionHandler) enrichSlackInboundContext(ctx context.Context, row *model.SlackThreadEvent, token string, client slackapp.Client) error {
	if row.EventType == slackapp.EventReactionAdded {
		return nil
	}
	messageContext, err := slackapp.FetchInboundMessageContext(ctx, client, row.SlackChannelID, row.ThreadTS, row.MessageTS)
	if err != nil {
		return fmt.Errorf("fetch slack inbound message context: %w", err)
	}
	rendered := slackRenderedMessageContextFor(ctx, h.mediaEnricher, token, row.OrgID, messageContext)
	text := slackInboundContextText(*row, messageContext, rendered)
	return h.updateSlackInboundContext(ctx, row, messageContext.ThreadTS, text)
}

func (h *SlackAppMentionHandler) updateSlackInboundContext(ctx context.Context, row *model.SlackThreadEvent, threadTS, text string) error {
	threadTS = strings.TrimSpace(threadTS)
	if threadTS == "" {
		threadTS = row.ThreadTS
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("slack inbound message context is empty")
	}
	now := time.Now().UTC()
	if err := h.db.WithContext(ctx).Model(&model.SlackThreadEvent{}).Where("id = ?", row.ID).Updates(map[string]any{
		"thread_ts":  threadTS,
		"text":       text,
		"updated_at": now,
	}).Error; err != nil {
		return fmt.Errorf("update slack inbound context: %w", err)
	}
	row.ThreadTS = threadTS
	row.Text = text
	return nil
}

func slackInboundContextText(row model.SlackThreadEvent, context slackapp.ReactionMessageContext, rendered slackRenderedMessageContext) string {
	var b strings.Builder
	b.WriteString("Slack inbound message context:\n")
	writeAutomationLine(&b, "event_type", row.EventType)
	writeAutomationLine(&b, "sender_id", row.SenderID)
	writeAutomationLine(&b, "sender_tag", slackUserTag(row.SenderID))
	writeAutomationLine(&b, "user_name", slackInboundRawString(row.Raw, "user_name"))
	writeAutomationLine(&b, "display_name", slackInboundRawString(row.Raw, "display_name"))
	writeAutomationLine(&b, "channel_id", row.SlackChannelID)
	writeAutomationLine(&b, "message_ts", row.MessageTS)
	writeAutomationLine(&b, "thread_ts", context.ThreadTS)
	writeAutomationLine(&b, "clean_text", row.Text)
	b.WriteString("\nFetched message:\n")
	b.WriteString(slackMessageSection(context.Message, rendered.Message))
	if len(rendered.ThreadMessages) > 0 {
		b.WriteString("\n\nThread context:\n")
		for i, message := range context.ThreadMessages {
			if i >= len(rendered.ThreadMessages) {
				break
			}
			b.WriteString(slackMessageSection(message, rendered.ThreadMessages[i]))
			b.WriteString("\n\n")
		}
	}
	return strings.TrimSpace(b.String())
}

func slackInboundRawString(raw model.JSON, key string) string {
	value, _ := raw[key].(string)
	return strings.TrimSpace(value)
}
