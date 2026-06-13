package gateway

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/model"
)

type RuntimeClientProvider interface {
	GetRuntimeClient(context.Context, *model.Sandbox) (*agentruntime.Client, error)
}

type OrchestratedRuntimeMessenger struct {
	db       *gorm.DB
	provider RuntimeClientProvider
}

func NewOrchestratedRuntimeMessenger(db *gorm.DB, provider RuntimeClientProvider) *OrchestratedRuntimeMessenger {
	return &OrchestratedRuntimeMessenger{db: db, provider: provider}
}

func (m *OrchestratedRuntimeMessenger) Send(ctx context.Context, message RuntimeMessage) (*RuntimeDelivery, error) {
	if m == nil || m.db == nil || m.provider == nil {
		return nil, fmt.Errorf("gateway runtime messenger is not configured")
	}
	if message.Session.SandboxID == uuid.Nil {
		return nil, fmt.Errorf("gateway runtime message is missing agent session sandbox_id")
	}
	var sandbox model.Sandbox
	if err := m.db.WithContext(ctx).Where("id = ?", message.Session.SandboxID).First(&sandbox).Error; err != nil {
		return nil, fmt.Errorf("load gateway runtime sandbox: %w", err)
	}
	client, err := m.provider.GetRuntimeClient(ctx, &sandbox)
	if err != nil {
		return nil, fmt.Errorf("get gateway runtime client: %w", err)
	}
	resp, err := client.PostHTTPMessage(ctx, agentruntime.HTTPMessageRequest{
		Text:            message.Text,
		ConversationID:  message.ConversationID,
		User:            message.User,
		UserDisplayName: message.UserDisplayName,
		DynamicContext:  message.DynamicContext,
		Raw:             runtimeRaw(message),
	})
	if err != nil {
		return nil, fmt.Errorf("send gateway message to runtime: %w", err)
	}
	return &RuntimeDelivery{
		SessionID:         resp.SessionID,
		StreamID:          resp.StreamID,
		ResponseStreamID:  resp.ResponseStreamID,
		ResponseStreamURL: resp.ResponseStreamURL,
		TraceID:           resp.TraceID,
		TurnID:            resp.TurnID,
	}, nil
}

func runtimeRaw(message RuntimeMessage) map[string]any {
	raw := map[string]any{
		"source":              Source,
		"provider":            message.GatewayProvider,
		"route_id":            message.Route.ID.String(),
		"agent_session_id":    message.Session.ID.String(),
		"gateway_event_id":    message.GatewayEventID.String(),
		"dedupe_key":          message.GatewayDedupeKey,
		"thread_key":          message.GatewayThreadKey,
		"channel_id":          message.GatewayChannelID,
		"thread_id":           message.GatewayThreadID,
		"external_message_id": message.GatewayExternalMsgID,
	}
	for key, value := range message.Metadata {
		raw[key] = value
	}
	return raw
}
