package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	slacksdk "github.com/slack-go/slack"

	"github.com/usehivy/hivy/internal/gateway"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/nango"
)

type GatewaySlackHandler struct {
	db                 *gorm.DB
	nangoClient        *nango.Client
	slackClientFactory func(string) slackGatewayClient
}

func NewGatewaySlackHandler(db *gorm.DB, nangoClient *nango.Client) *GatewaySlackHandler {
	return &GatewaySlackHandler{db: db, nangoClient: nangoClient}
}

type slackGatewayClient interface {
	SetAssistantThreadsStatusContext(context.Context, slacksdk.AssistantThreadsSetStatusParameters) error
	PostMessageContext(context.Context, string, ...slacksdk.MsgOption) (string, string, error)
}

const (
	slackAssistantStatus = "is thinking..."
)

func (h *GatewaySlackHandler) Handle(ctx context.Context, t *asynq.Task) error {
	var payload GatewaySlackPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack: unmarshal payload: %w", err), map[string]any{
			"task_type": t.Type(),
		})
		return err
	}

	fields := map[string]any{
		"connection_id": payload.ConnectionID,
		"org_id":        payload.OrgID,
		"employee_id":   payload.EmployeeID,
		"channel_id":    payload.ChannelID,
		"thread_ts":     payload.ThreadTS,
		"session_id":    payload.SessionID,
		"trace_id":      payload.TraceID,
		"turn_id":       payload.TurnID,
	}

	botToken, err := h.loadBotToken(ctx, payload.NangoConnID, payload.ProviderKey)
	if err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack: load bot token: %w", err), fields)
		return err
	}

	client := h.newSlackClient(botToken)

	streamURL := payload.StreamURL
	subscriber := gateway.NewSSESubscriber(&http.Client{Timeout: 610 * time.Second})
	events, err := subscriber.Subscribe(ctx, streamURL, payload.RuntimeAPIKey)
	if err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack: subscribe to stream: %w", err), fields)
		return err
	}

	text, delivered, providerMessageID, err := h.deliverSlackResponse(ctx, payload, client, events, fields)
	if delivered {
		h.recordDelivery(ctx, payload, text, providerMessageID)
	}
	return err
}

func (h *GatewaySlackHandler) HandleStatus(ctx context.Context, t *asynq.Task) error {
	var payload GatewaySlackStatusPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack status: unmarshal payload: %w", err), map[string]any{
			"task_type": t.Type(),
		})
		return err
	}
	fields := map[string]any{
		"connection_id": payload.ConnectionID,
		"org_id":        payload.OrgID,
		"employee_id":   payload.EmployeeID,
		"channel_id":    payload.ChannelID,
		"thread_ts":     payload.ThreadTS,
		"event_id":      payload.EventID,
	}

	botToken, err := h.loadBotToken(ctx, payload.NangoConnID, payload.ProviderKey)
	if err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack status: load bot token: %w", err), fields)
		return err
	}

	h.setStatus(ctx, h.newSlackClient(botToken), payload.ChannelID, payload.ThreadTS, fields)
	return nil
}

func (h *GatewaySlackHandler) newSlackClient(botToken string) slackGatewayClient {
	if h.slackClientFactory != nil {
		return h.slackClientFactory(botToken)
	}
	return slacksdk.New(botToken)
}

func (h *GatewaySlackHandler) deliverSlackResponse(ctx context.Context, payload GatewaySlackPayload, client slackGatewayClient, events <-chan gateway.SSEEvent, fields map[string]any) (string, bool, string, error) {
	statusCleared := false
	tokenCount := 0
	var streamedText strings.Builder

	h.setStatus(ctx, client, payload.ChannelID, payload.ThreadTS, fields)

	for event := range events {
		if ctx.Err() != nil {
			logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack: context cancelled"), fields)
			return "", false, "", ctx.Err()
		}

		switch event.Type {
		case "token":
			var data struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(event.Data, &data); err != nil || data.Text == "" {
				continue
			}
			streamedText.WriteString(data.Text)
			tokenCount++

		case "final":
			var data struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal(event.Data, &data)

			if !statusCleared {
				h.clearStatus(ctx, client, payload.ChannelID, payload.ThreadTS, fields)
			}

			text := firstNonEmpty(streamedText.String(), data.Text)
			if text == "" {
				text = "No response generated."
			}
			method, providerMessageID, err := h.finishSlackResponse(ctx, client, payload, text, fields)
			if err != nil {
				return "", false, "", err
			}

			logging.FromContext(ctx).InfoContext(ctx, "gateway_slack_completed",
				"connection_id", payload.ConnectionID,
				"org_id", payload.OrgID,
				"channel_id", payload.ChannelID,
				"thread_ts", payload.ThreadTS,
				"delivery_method", method,
				"token_count", tokenCount,
				"response_length", len(text),
			)
			return text, true, providerMessageID, nil

		case "done":
			if !statusCleared {
				h.clearStatus(ctx, client, payload.ChannelID, payload.ThreadTS, fields)
			}
			text := streamedText.String()
			if strings.TrimSpace(text) == "" {
				logging.FromContext(ctx).InfoContext(ctx, "gateway slack: stream done without visible response",
					"connection_id", payload.ConnectionID,
					"org_id", payload.OrgID,
					"channel_id", payload.ChannelID,
					"thread_ts", payload.ThreadTS,
				)
				return "", false, "", nil
			}
			method, providerMessageID, err := h.finishSlackResponse(ctx, client, payload, text, fields)
			if err != nil {
				return "", false, "", err
			}
			logging.FromContext(ctx).InfoContext(ctx, "gateway_slack_stream_done",
				"connection_id", payload.ConnectionID,
				"token_count", tokenCount,
				"delivery_method", method,
			)
			return text, true, providerMessageID, nil

		case "error":
			if !statusCleared {
				h.clearStatus(ctx, client, payload.ChannelID, payload.ThreadTS, fields)
			}
			text := "Something went wrong. Please try again."
			_, providerMessageID, err := h.finishSlackResponse(ctx, client, payload, text, fields)
			if err != nil {
				return "", false, "", err
			}
			logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack: agent error in stream"), fields)
			return text, true, providerMessageID, nil

		case "thinking", "tool_call", "tool_result", "model_usage", "turn_started", "turn_completed":
			continue
		}
	}

	if !statusCleared {
		h.clearStatus(ctx, client, payload.ChannelID, payload.ThreadTS, fields)
	}
	if strings.TrimSpace(streamedText.String()) != "" {
		text := streamedText.String()
		method, providerMessageID, err := h.finishSlackResponse(ctx, client, payload, text, fields)
		if err != nil {
			return "", false, "", err
		}
		logging.FromContext(ctx).WarnContext(ctx, "gateway slack: stream ended without final/done; posted accumulated tokens",
			"connection_id", payload.ConnectionID,
			"org_id", payload.OrgID,
			"channel_id", payload.ChannelID,
			"thread_ts", payload.ThreadTS,
			"delivery_method", method,
			"token_count", tokenCount,
			"response_length", len(text),
		)
		return text, true, providerMessageID, nil
	}
	logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack: stream ended without final/done"), fields)
	return "", false, "", fmt.Errorf("stream ended without final/done")
}

func (h *GatewaySlackHandler) loadBotToken(ctx context.Context, nangoConnID, providerKey string) (string, error) {
	nangoConn, err := h.nangoClient.GetConnection(ctx, nangoConnID, providerKey)
	if err != nil {
		return "", fmt.Errorf("load nango connection: %w", err)
	}
	creds, _ := nangoConn["credentials"].(map[string]any)
	for _, key := range []string{"bot_token", "access_token"} {
		if token, ok := creds[key].(string); ok && strings.TrimSpace(token) != "" {
			return strings.TrimSpace(token), nil
		}
	}
	return "", fmt.Errorf("no bot token in nango credentials")
}
