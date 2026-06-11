package tasks

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

type GatewaySlackPayload struct {
	RouteID        string `json:"route_id,omitempty"`
	EventID        string `json:"event_id,omitempty"`
	ConnectionID   string `json:"connection_id"`
	OrgID          string `json:"org_id"`
	EmployeeID     string `json:"employee_id"`
	ChannelID      string `json:"channel_id"`
	ThreadTS       string `json:"thread_ts"`
	TeamID         string `json:"team_id,omitempty"`
	StreamURL      string `json:"stream_url"`
	RuntimeURL     string `json:"runtime_url"`
	RuntimeAPIKey  string `json:"runtime_api_key"`
	SessionID      string `json:"session_id"`
	RuntimeConvoID string `json:"runtime_conversation_id"`
	TraceID        string `json:"trace_id"`
	TurnID         string `json:"turn_id"`
	SenderID       string `json:"sender_id"`
	ActionToken    string `json:"action_token,omitempty"`
	NangoConnID    string `json:"nango_connection_id"`
	ProviderKey    string `json:"provider_config_key"`
}

type GatewayExternalCallbackPayload struct {
	RouteID        string `json:"route_id"`
	OrgID          string `json:"org_id"`
	EmployeeID     string `json:"employee_id"`
	EventID        string `json:"event_id"`
	SessionID      string `json:"session_id"`
	RuntimeConvoID string `json:"runtime_conversation_id"`
	StreamURL      string `json:"stream_url"`
	RuntimeURL     string `json:"runtime_url"`
	RuntimeAPIKey  string `json:"runtime_api_key"`
	TraceID        string `json:"trace_id"`
	TurnID         string `json:"turn_id"`
	Provider       string `json:"provider"`
	CallbackURL    string `json:"callback_url"`
	RouteSecret    string `json:"route_secret"`
	ThreadKey      string `json:"thread_key"`
	ChannelID      string `json:"channel_id"`
	ThreadID       string `json:"thread_id"`
}

type GatewaySlackStatusPayload struct {
	ConnectionID string `json:"connection_id"`
	OrgID        string `json:"org_id"`
	EmployeeID   string `json:"employee_id"`
	ChannelID    string `json:"channel_id"`
	ThreadTS     string `json:"thread_ts"`
	TeamID       string `json:"team_id,omitempty"`
	EventID      string `json:"event_id,omitempty"`
	NangoConnID  string `json:"nango_connection_id"`
	ProviderKey  string `json:"provider_config_key"`
}

// NewGatewaySlackTask creates a task that delivers a Slack gateway message.
// Options are returned separately (see WebhookForwardPayload's NewWebhookForwardTask).
func NewGatewaySlackTask(payload GatewaySlackPayload) (*asynq.Task, []asynq.Option, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal gateway slack payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueCritical),
		asynq.MaxRetry(2),
		asynq.Timeout(610 * time.Second),
	}
	return asynq.NewTask(TypeGatewaySlack, encoded), opts, nil
}

// NewGatewayExternalCallbackTask creates a task that delivers an external
// callback. Options are returned separately (see WebhookForwardPayload's NewWebhookForwardTask).
func NewGatewayExternalCallbackTask(payload GatewayExternalCallbackPayload) (*asynq.Task, []asynq.Option, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal gateway external callback payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueCritical),
		asynq.MaxRetry(2),
		asynq.Timeout(610 * time.Second),
	}
	return asynq.NewTask(TypeGatewayExternalCallback, encoded), opts, nil
}

// NewGatewaySlackStatusTask creates a task that sends a Slack status update.
// Options are returned separately (see WebhookForwardPayload's NewWebhookForwardTask).
func NewGatewaySlackStatusTask(payload GatewaySlackStatusPayload) (*asynq.Task, []asynq.Option, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal gateway slack status payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueCritical),
		asynq.MaxRetry(1),
		asynq.Timeout(30 * time.Second),
	}
	return asynq.NewTask(TypeGatewaySlackStatus, encoded), opts, nil
}
