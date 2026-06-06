package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/model"
)

const (
	ExternalProvider    = "external"
	ExternalAdapterName = "external"
)

type ExternalAdapterConfig struct {
	CallbackURL     string `json:"callback_url,omitempty"`
	SecretHash      string `json:"secret_hash,omitempty"`
	EncryptedSecret string `json:"encrypted_secret,omitempty"`
	SecretPrefix    string `json:"secret_prefix,omitempty"`
	SecretRotatedAt string `json:"secret_rotated_at,omitempty"`
	Adapter         string `json:"adapter,omitempty"`
}

type ExternalAdapter struct{}

type externalInboundPayload struct {
	MessageID string         `json:"message_id"`
	ThreadID  string         `json:"thread_id"`
	ChannelID string         `json:"channel_id"`
	Sender    externalSender `json:"sender"`
	Text      string         `json:"text"`
	Markdown  string         `json:"markdown"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type externalSender struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func NewExternalAdapter() *ExternalAdapter {
	return &ExternalAdapter{}
}

func (a *ExternalAdapter) Provider() string {
	return ExternalProvider
}

func (a *ExternalAdapter) DecodeInbound(_ context.Context, envelope WebhookEnvelope) (InboundEnvelope, bool, error) {
	var payload externalInboundPayload
	if err := json.Unmarshal(envelope.Body, &payload); err != nil {
		return InboundEnvelope{}, false, fmt.Errorf("decode external gateway message: invalid JSON payload: %w", err)
	}
	text := firstNonEmpty(payload.Markdown, payload.Text)
	if strings.TrimSpace(text) == "" {
		return InboundEnvelope{}, false, fmt.Errorf("decode external gateway message: text is required")
	}
	messageID := strings.TrimSpace(payload.MessageID)
	if messageID == "" {
		return InboundEnvelope{}, false, fmt.Errorf("decode external gateway message: message_id is required")
	}
	threadID := strings.TrimSpace(payload.ThreadID)
	if threadID == "" {
		return InboundEnvelope{}, false, fmt.Errorf("decode external gateway message: thread_id is required")
	}
	raw := map[string]any{
		"message_id": messageID,
		"thread_id":  threadID,
		"channel_id": strings.TrimSpace(payload.ChannelID),
		"sender": map[string]any{
			"id":   strings.TrimSpace(payload.Sender.ID),
			"name": strings.TrimSpace(payload.Sender.Name),
		},
		"text":     text,
		"metadata": payload.Metadata,
	}
	return InboundEnvelope{
		RouteID:           envelope.RouteID,
		ExternalMessageID: messageID,
		DedupeKey:         messageID,
		ThreadKey:         threadID,
		ChannelID:         firstNonEmpty(payload.ChannelID, threadID),
		ThreadID:          threadID,
		SenderID:          firstNonEmpty(payload.Sender.ID, "external"),
		SenderName:        strings.TrimSpace(payload.Sender.Name),
		Text:              text,
		Raw:               raw,
		ReceivedAt:        time.Now().UTC(),
	}, true, nil
}

func (a *ExternalAdapter) FormatAgentRequest(_ context.Context, inbound InboundEnvelope) (AgentRequest, error) {
	if strings.TrimSpace(inbound.Text) == "" {
		return AgentRequest{}, fmt.Errorf("format external gateway message: text is required")
	}
	return AgentRequest{Markdown: inbound.Text, Metadata: inbound.Raw}, nil
}

func (a *ExternalAdapter) RenderResponse(_ context.Context, response AgentResponse) (ProviderResponsePayload, error) {
	if strings.TrimSpace(response.Text) == "" {
		return ProviderResponsePayload{}, fmt.Errorf("render external gateway response: response text is required")
	}
	return ProviderResponsePayload{
		Route:     response.Route,
		Session:   response.EmployeeSession,
		ChannelID: response.ChannelID,
		ThreadID:  response.ThreadID,
		Text:      response.Text,
		Raw:       response.Raw,
	}, nil
}

func (a *ExternalAdapter) SendResponse(context.Context, ProviderResponsePayload) ([]MessageHandle, error) {
	return nil, fmt.Errorf("external gateway responses are delivered by gateway:external_callback task")
}

func RouteUsesExternalAdapter(route model.EmployeeGatewayRoute) bool {
	adapter, _ := route.Config["adapter"].(string)
	return strings.EqualFold(strings.TrimSpace(adapter), ExternalAdapterName)
}
