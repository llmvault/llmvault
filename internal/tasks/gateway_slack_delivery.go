package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	slacksdk "github.com/slack-go/slack"

	"github.com/usehivy/hivy/internal/gateway"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
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
	messageTS, err := h.postThreadReply(ctx, client, payload.ChannelID, payload.ThreadTS, text, fields)
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

func (h *GatewaySlackHandler) postThreadReply(ctx context.Context, client slackGatewayClient, channelID, threadTS, text string, fields map[string]any) (string, error) {
	_, messageTS, err := client.PostMessageContext(ctx, channelID,
		slacksdk.MsgOptionText(text, false),
		slacksdk.MsgOptionBlocks(slacksdk.NewMarkdownBlock("", text)),
		slacksdk.MsgOptionTS(threadTS),
	)
	if err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack: post thread reply: %w", err), fields)
		logging.FromContext(ctx).WarnContext(ctx, "gateway slack: post thread reply failed",
			"channel_id", channelID,
			"thread_ts", threadTS,
			"error", err,
		)
		return "", err
	}
	logging.FromContext(ctx).InfoContext(ctx, "gateway slack: post thread reply sent",
		"channel_id", channelID,
		"thread_ts", threadTS,
		"message_ts", messageTS,
	)
	return messageTS, nil
}

func (h *GatewaySlackHandler) recordDelivery(ctx context.Context, payload GatewaySlackPayload, text, providerMessageID string) {
	orgID, _ := parseUUID(payload.OrgID)
	employeeID, _ := parseUUID(payload.EmployeeID)
	sessionID, _ := parseUUID(payload.SessionID)

	delivery := model.EmployeeGatewayDelivery{
		OrgID:             orgID,
		EmployeeID:        employeeID,
		Provider:          gateway.SlackProvider,
		DedupeKey:         payload.TraceID + ":" + payload.TurnID,
		RuntimeSessionID:  payload.SessionID,
		RuntimeTraceID:    payload.TraceID,
		RuntimeTurnID:     payload.TurnID,
		ThreadKey:         payload.ChannelID + ":" + payload.ThreadTS,
		ChannelID:         payload.ChannelID,
		ThreadID:          payload.ThreadTS,
		ResponseText:      text,
		Status:            "sent",
		EmployeeSessionID: sessionID,
	}
	if strings.TrimSpace(providerMessageID) != "" {
		handles, _ := json.Marshal([]gateway.MessageHandle{{
			ProviderMessageID: providerMessageID,
			ChannelID:         payload.ChannelID,
			ThreadID:          payload.ThreadTS,
			Raw: map[string]any{
				"slack_ts": providerMessageID,
			},
		}})
		delivery.ProviderHandles = model.RawJSON(handles)
	}

	if err := h.db.WithContext(ctx).Create(&delivery).Error; err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack: record delivery: %w", err), map[string]any{
			"connection_id": payload.ConnectionID,
			"org_id":        payload.OrgID,
			"channel_id":    payload.ChannelID,
			"thread_ts":     payload.ThreadTS,
		})
	}
}

func parseUUID(s string) (uuid.UUID, error) {
	if s == "" {
		return uuid.Nil, nil
	}
	return uuid.Parse(s)
}
