package tasks

import (
	"context"
	"strings"

	"github.com/google/uuid"
	slacksdk "github.com/slack-go/slack"

	"github.com/usehivy/hivy/internal/slackapp"
)

type slackRenderedMessageContext struct {
	Message        string
	ThreadMessages []string
}

func slackRenderedMessageContextFor(ctx context.Context, enricher SlackMediaEnricher, token string, orgID uuid.UUID, context slackapp.ReactionMessageContext) slackRenderedMessageContext {
	render := slackMessageRenderer(ctx, enricher, token, orgID)
	rendered := slackRenderedMessageContext{
		Message: render(context.Message),
	}
	rendered.ThreadMessages = make([]string, 0, len(context.ThreadMessages))
	for _, message := range context.ThreadMessages {
		rendered.ThreadMessages = append(rendered.ThreadMessages, render(message))
	}
	return rendered
}

func slackRenderedSlackMessage(ctx context.Context, enricher SlackMediaEnricher, token string, orgID uuid.UUID, message slacksdk.Message) string {
	parts := []string{slackapp.RenderSlackMessageMarkdown(message)}
	if media := EnrichSlackMedia(ctx, enricher, token, orgID, message); media != "" {
		parts = append(parts, media)
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func slackMessageRenderer(ctx context.Context, enricher SlackMediaEnricher, token string, orgID uuid.UUID) func(slacksdk.Message) string {
	renderedByTS := map[string]string{}
	return func(message slacksdk.Message) string {
		ts := strings.TrimSpace(message.Timestamp)
		if ts != "" {
			if rendered, ok := renderedByTS[ts]; ok {
				return rendered
			}
		}
		rendered := slackRenderedSlackMessage(ctx, enricher, token, orgID, message)
		if ts != "" {
			renderedByTS[ts] = rendered
		}
		return rendered
	}
}

func slackMessageSection(message slacksdk.Message, text string) string {
	sender := slackMessageSender(message)
	text = strings.TrimSpace(text)
	if text == "" {
		text = "(no text)"
	}
	ts := strings.TrimSpace(message.Timestamp)
	if ts == "" {
		return sender + ":\n" + text
	}
	return sender + " [" + ts + "]:\n" + text
}

func slackMessageSender(message slacksdk.Message) string {
	if strings.TrimSpace(message.User) != "" {
		return slackUserTag(message.User)
	}
	if strings.TrimSpace(message.Username) != "" {
		return strings.TrimSpace(message.Username)
	}
	if strings.TrimSpace(message.BotID) != "" {
		return "bot:" + strings.TrimSpace(message.BotID)
	}
	return "unknown"
}

func writeAutomationLine(b *strings.Builder, key, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	b.WriteString("- ")
	b.WriteString(key)
	b.WriteString(": ")
	b.WriteString(value)
	b.WriteString("\n")
}

func slackUserTag(userID string) string {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ""
	}
	return "<@" + userID + ">"
}
