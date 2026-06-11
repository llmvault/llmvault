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

// slackPostHTTPClient is a shared, bounded-timeout client for slack-go SDK calls; the SDK's
// default http.DefaultClient has no timeout and would block a worker goroutine if Slack stalls.
var slackPostHTTPClient = &http.Client{Timeout: 30 * time.Second}

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

	_, _, _, err = h.deliverSlackResponse(ctx, payload, client, events, fields)
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
	return slacksdk.New(botToken, slacksdk.OptionHTTPClient(slackPostHTTPClient))
}

func (h *GatewaySlackHandler) deliverSlackResponse(ctx context.Context, payload GatewaySlackPayload, client slackGatewayClient, events <-chan gateway.SSEEvent, fields map[string]any) (string, bool, string, error) {
	sink := &GatewaySlackSink{handler: h, client: client, payload: payload, fields: fields}
	result, err := NewGatewayStreamDeliveryService(h.db).DeliverEvents(ctx, gatewayStreamPayloadFromSlack(payload), sink, events, fields)
	if !sink.statusCleared {
		h.clearStatus(ctx, client, payload.ChannelID, payload.ThreadTS, fields)
	}
	if err != nil {
		return result.Text, result.Delivered, sink.providerMessageID, err
	}
	return result.Text, result.Delivered, sink.providerMessageID, nil
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
