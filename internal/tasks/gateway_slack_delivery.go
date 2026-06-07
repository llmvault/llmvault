package tasks

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	slacksdk "github.com/slack-go/slack"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/slackgateway"
)

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (h *GatewaySlackHandler) finishSlackResponse(ctx context.Context, client slackGatewayClient, payload GatewaySlackPayload, text string, fields map[string]any) (string, string, error) {
	messageTS, err := slackgateway.PostThreadReply(ctx, client, payload.ChannelID, payload.ThreadTS, text, fields)
	if err != nil {
		return "", "", err
	}
	return "post_message", messageTS, nil
}

func (h *GatewaySlackHandler) setStatus(ctx context.Context, client slackGatewayClient, channelID, threadTS string, fields map[string]any) {
	if err := client.SetAssistantThreadsStatusContext(ctx, slacksdk.AssistantThreadsSetStatusParameters{
		ChannelID:       channelID,
		ThreadTS:        threadTS,
		Status:          slackAssistantStatus,
		LoadingMessages: []string{"is thinking...", "is working on your request..."},
	}); err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack: set status: %w", err), fields)
		logging.FromContext(ctx).WarnContext(ctx, "gateway slack: set status failed",
			"channel_id", channelID,
			"thread_ts", threadTS,
			"error", err,
		)
		return
	}
	logging.FromContext(ctx).InfoContext(ctx, "gateway slack: status set",
		"channel_id", channelID,
		"thread_ts", threadTS,
		"status", slackAssistantStatus,
	)
}

func (h *GatewaySlackHandler) clearStatus(ctx context.Context, client slackGatewayClient, channelID, threadTS string, fields map[string]any) {
	if err := client.SetAssistantThreadsStatusContext(ctx, slacksdk.AssistantThreadsSetStatusParameters{
		ChannelID: channelID,
		ThreadTS:  threadTS,
		Status:    "",
	}); err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack: clear status: %w", err), fields)
		logging.FromContext(ctx).WarnContext(ctx, "gateway slack: clear status failed",
			"channel_id", channelID,
			"thread_ts", threadTS,
			"error", err,
		)
		return
	}
	logging.FromContext(ctx).InfoContext(ctx, "gateway slack: status cleared",
		"channel_id", channelID,
		"thread_ts", threadTS,
	)
}

func parseUUID(s string) (uuid.UUID, error) {
	if s == "" {
		return uuid.Nil, nil
	}
	return uuid.Parse(s)
}
