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
	if err := validateSlackMessageLookup(channelID, messageTS); err != nil {
		return ReactionMessageContext{}, err
	}
	reacted, err := client.GetReactionsContext(ctx, slacksdk.NewRefToMessage(channelID, messageTS), slacksdk.GetReactionsParameters{
		Full: true,
	})
	if err == nil && reacted.Message != nil {
		message := *reacted.Message
		if strings.TrimSpace(message.Timestamp) == "" {
			message.Timestamp = messageTS
		}
		return slackMessageContext(ctx, client, channelID, message, messageTS)
	}
	return fetchHistoryMessageContext(ctx, client, channelID, messageTS)
}

func FetchInboundMessageContext(ctx context.Context, client Client, channelID, threadTS, messageTS string) (ReactionMessageContext, error) {
	channelID = strings.TrimSpace(channelID)
	threadTS = strings.TrimSpace(threadTS)
	messageTS = strings.TrimSpace(messageTS)
	if err := validateSlackMessageLookup(channelID, messageTS); err != nil {
		return ReactionMessageContext{}, err
	}
	if threadTS != "" && threadTS != messageTS {
		return fetchThreadMessageContext(ctx, client, channelID, threadTS, messageTS)
	}
	return fetchHistoryMessageContext(ctx, client, channelID, messageTS)
}

func fetchHistoryMessageContext(ctx context.Context, client Client, channelID, messageTS string) (ReactionMessageContext, error) {
	resp, err := client.GetConversationHistoryContext(ctx, &slacksdk.GetConversationHistoryParameters{
		ChannelID: channelID,
		Latest:    messageTS,
		Inclusive: true,
		Limit:     1,
	})
	if err != nil {
		return ReactionMessageContext{}, fmt.Errorf("fetch slack message: %w", err)
	}
	if resp == nil || len(resp.Messages) == 0 {
		return ReactionMessageContext{}, fmt.Errorf("slack message not found")
	}
	message := resp.Messages[0]
	if strings.TrimSpace(message.Timestamp) != messageTS {
		return ReactionMessageContext{}, fmt.Errorf("slack message timestamp mismatch")
	}
	return slackMessageContext(ctx, client, channelID, message, messageTS)
}

func slackMessageContext(ctx context.Context, client Client, channelID string, message slacksdk.Message, messageTS string) (ReactionMessageContext, error) {
	threadTS := reactionThreadTS(message, messageTS)
	result := ReactionMessageContext{Message: message, ThreadTS: threadTS}
	if !reactionMessageHasThread(message, threadTS, messageTS) {
		return result, nil
	}
	replies, err := fetchSlackThreadMessages(ctx, client, channelID, threadTS)
	if err != nil {
		return ReactionMessageContext{}, fmt.Errorf("fetch slack thread replies: %w", err)
	}
	result.ThreadMessages = replies
	return result, nil
}

func fetchThreadMessageContext(ctx context.Context, client Client, channelID, threadTS, messageTS string) (ReactionMessageContext, error) {
	replies, err := fetchSlackThreadMessages(ctx, client, channelID, threadTS)
	if err != nil {
		return ReactionMessageContext{}, fmt.Errorf("fetch slack thread replies: %w", err)
	}
	message, ok := findSlackMessageByTS(replies, messageTS)
	if !ok {
		return ReactionMessageContext{}, fmt.Errorf("slack thread message not found")
	}
	return ReactionMessageContext{
		Message:        message,
		ThreadMessages: replies,
		ThreadTS:       threadTS,
	}, nil
}

func fetchSlackThreadMessages(ctx context.Context, client Client, channelID, threadTS string) ([]slacksdk.Message, error) {
	var messages []slacksdk.Message
	cursor := ""
	for {
		replies, hasMore, nextCursor, err := client.GetConversationRepliesContext(ctx, &slacksdk.GetConversationRepliesParameters{
			ChannelID: channelID,
			Timestamp: threadTS,
			Cursor:    cursor,
			Inclusive: true,
			Limit:     200,
		})
		if err != nil {
			return nil, err
		}
		messages = append(messages, replies...)
		if !hasMore || strings.TrimSpace(nextCursor) == "" {
			return messages, nil
		}
		cursor = nextCursor
	}
}

func findSlackMessageByTS(messages []slacksdk.Message, ts string) (slacksdk.Message, bool) {
	ts = strings.TrimSpace(ts)
	for _, message := range messages {
		if strings.TrimSpace(message.Timestamp) == ts {
			return message, true
		}
	}
	return slacksdk.Message{}, false
}

func validateSlackMessageLookup(channelID, messageTS string) error {
	if channelID == "" {
		return fmt.Errorf("slack channel id is required")
	}
	if messageTS == "" {
		return fmt.Errorf("slack message timestamp is required")
	}
	return nil
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
