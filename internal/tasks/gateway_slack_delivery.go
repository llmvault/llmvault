package tasks

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	slacksdk "github.com/slack-go/slack"

	"github.com/usehivy/hivy/internal/gateway"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func shouldFlushSlackStream(pendingBytes int, lastFlush time.Time) bool {
	return pendingBytes >= slackStreamFlushBytes || time.Since(lastFlush) >= slackStreamFlushWindow
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (h *GatewaySlackHandler) startStream(ctx context.Context, client slackGatewayClient, payload GatewaySlackPayload, fields map[string]any) (string, error) {
	opts := []slacksdk.MsgOption{slacksdk.MsgOptionTS(payload.ThreadTS)}
	if strings.TrimSpace(payload.TeamID) != "" {
		opts = append(opts, slacksdk.MsgOptionRecipientTeamID(strings.TrimSpace(payload.TeamID)))
	}
	if strings.TrimSpace(payload.SenderID) != "" {
		opts = append(opts, slacksdk.MsgOptionRecipientUserID(strings.TrimSpace(payload.SenderID)))
	}
	_, streamTS, err := client.StartStreamContext(ctx, payload.ChannelID, opts...)
	if err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack: start stream: %w", err), fields)
		logging.FromContext(ctx).WarnContext(ctx, "gateway slack: start stream failed, falling back to chat.postMessage",
			"channel_id", payload.ChannelID,
			"thread_ts", payload.ThreadTS,
			"error", err,
		)
		return "", err
	}
	return streamTS, nil
}

func (h *GatewaySlackHandler) finishSlackResponse(ctx context.Context, client slackGatewayClient, payload GatewaySlackPayload, slackStreamTS, text string, streamedText, pendingText *strings.Builder, fields map[string]any) (string, error) {
	if slackStreamTS != "" {
		if err := h.appendFinalSuffix(ctx, client, payload.ChannelID, slackStreamTS, text, streamedText, pendingText, fields); err != nil {
			return "", err
		}
		if err := h.stopStream(ctx, client, payload.ChannelID, slackStreamTS, fields); err == nil {
			return "stream", nil
		}
		if strings.TrimSpace(streamedText.String()) != "" {
			return "stream_unconfirmed", nil
		}
	}
	if err := h.postThreadReply(ctx, client, payload.ChannelID, payload.ThreadTS, text, fields); err != nil {
		return "", err
	}
	return "post_message", nil
}

func (h *GatewaySlackHandler) appendFinalSuffix(ctx context.Context, client slackGatewayClient, channelID, streamTS, finalText string, streamedText, pendingText *strings.Builder, fields map[string]any) error {
	streamed := streamedText.String()
	if strings.HasPrefix(finalText, streamed) && len(finalText) > len(streamed) {
		pendingText.WriteString(finalText[len(streamed):])
		streamedText.WriteString(finalText[len(streamed):])
	}
	return h.flushPendingStream(ctx, client, channelID, streamTS, pendingText, fields)
}

func (h *GatewaySlackHandler) flushPendingStream(ctx context.Context, client slackGatewayClient, channelID, streamTS string, pendingText *strings.Builder, fields map[string]any) error {
	text := pendingText.String()
	if text == "" {
		return nil
	}
	if err := h.appendStream(ctx, client, channelID, streamTS, text, fields); err != nil {
		return err
	}
	pendingText.Reset()
	return nil
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
	}
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
	}
}

func (h *GatewaySlackHandler) appendStream(ctx context.Context, client slackGatewayClient, channelID, streamTS, text string, fields map[string]any) error {
	if _, _, err := client.AppendStreamContext(ctx, channelID, streamTS,
		slacksdk.MsgOptionMarkdownText(text),
	); err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack: append stream: %w", err), fields)
		logging.FromContext(ctx).WarnContext(ctx, "gateway slack: append stream failed",
			"channel_id", channelID,
			"slack_stream_ts", streamTS,
			"error", err,
		)
		return err
	}
	return nil
}

func (h *GatewaySlackHandler) stopStream(ctx context.Context, client slackGatewayClient, channelID, streamTS string, fields map[string]any) error {
	if _, _, err := client.StopStreamContext(ctx, channelID, streamTS); err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack: stop stream: %w", err), fields)
		logging.FromContext(ctx).WarnContext(ctx, "gateway slack: stop stream failed",
			"channel_id", channelID,
			"slack_stream_ts", streamTS,
			"error", err,
		)
		return err
	}
	return nil
}

func (h *GatewaySlackHandler) postThreadReply(ctx context.Context, client slackGatewayClient, channelID, threadTS, text string, fields map[string]any) error {
	if _, _, err := client.PostMessageContext(ctx, channelID,
		slacksdk.MsgOptionText(text, false),
		slacksdk.MsgOptionTS(threadTS),
	); err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack: post thread reply: %w", err), fields)
		logging.FromContext(ctx).WarnContext(ctx, "gateway slack: post thread reply failed",
			"channel_id", channelID,
			"thread_ts", threadTS,
			"error", err,
		)
		return err
	}
	return nil
}

func (h *GatewaySlackHandler) recordDelivery(ctx context.Context, payload GatewaySlackPayload, text string) {
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
