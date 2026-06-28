package slackapp

import (
	"context"
	"fmt"
	"strings"

	slacksdk "github.com/slack-go/slack"
)

type ReactionMessageContext struct {
	Message        slacksdk.Message
	ThreadMessages []slacksdk.Message
	ThreadTS       string
}

func FetchReactionMessageContext(ctx context.Context, client Client, channelID, messageTS string) (ReactionMessageContext, error) {
	channelID = strings.TrimSpace(channelID)
	messageTS = strings.TrimSpace(messageTS)
	if channelID == "" {
		return ReactionMessageContext{}, fmt.Errorf("slack channel id is required")
	}
	if messageTS == "" {
		return ReactionMessageContext{}, fmt.Errorf("slack message timestamp is required")
	}
	resp, err := client.GetConversationHistoryContext(ctx, &slacksdk.GetConversationHistoryParameters{
		ChannelID: channelID,
		Latest:    messageTS,
		Inclusive: true,
		Limit:     1,
	})
	if err != nil {
		return ReactionMessageContext{}, fmt.Errorf("fetch reacted slack message: %w", err)
	}
	if resp == nil || len(resp.Messages) == 0 {
		return ReactionMessageContext{}, fmt.Errorf("reacted slack message not found")
	}
	message := resp.Messages[0]
	if strings.TrimSpace(message.Timestamp) != messageTS {
		return ReactionMessageContext{}, fmt.Errorf("reacted slack message timestamp mismatch")
	}
	threadTS := reactionThreadTS(message, messageTS)
	result := ReactionMessageContext{Message: message, ThreadTS: threadTS}
	if !reactionMessageHasThread(message, threadTS, messageTS) {
		return result, nil
	}
	replies, _, _, err := client.GetConversationRepliesContext(ctx, &slacksdk.GetConversationRepliesParameters{
		ChannelID: channelID,
		Timestamp: threadTS,
		Inclusive: true,
		Limit:     100,
	})
	if err != nil {
		return ReactionMessageContext{}, fmt.Errorf("fetch slack thread replies: %w", err)
	}
	result.ThreadMessages = replies
	return result, nil
}

func reactionThreadTS(message slacksdk.Message, fallback string) string {
	threadTS := strings.TrimSpace(message.ThreadTimestamp)
	if threadTS == "" {
		return strings.TrimSpace(fallback)
	}
	return threadTS
}

func reactionMessageHasThread(message slacksdk.Message, threadTS, messageTS string) bool {
	if message.ReplyCount > 0 {
		return true
	}
	if strings.TrimSpace(threadTS) != "" && strings.TrimSpace(threadTS) != strings.TrimSpace(messageTS) {
		return true
	}
	return strings.TrimSpace(message.ThreadTimestamp) == strings.TrimSpace(message.Timestamp) && strings.TrimSpace(message.ThreadTimestamp) != ""
}
