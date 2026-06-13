package tasks

import (
	"context"

	"github.com/usehivy/hivy/internal/gateway"
	"github.com/usehivy/hivy/internal/logging"
)

type GatewaySlackSink struct {
	handler           *GatewaySlackHandler
	client            slackGatewayClient
	payload           GatewaySlackPayload
	fields            map[string]any
	statusCleared     bool
	providerMessageID string
}

func (s *GatewaySlackSink) Provider() string {
	return gateway.SlackProvider
}

func (s *GatewaySlackSink) BeforeWait(ctx context.Context, _ GatewayStreamPayload) error {
	s.handler.setStatus(ctx, s.client, s.payload.ChannelID, s.payload.ThreadTS, s.fields)
	return nil
}

func (s *GatewaySlackSink) SendFinal(ctx context.Context, _ GatewayStreamPayload, text string) ([]gateway.MessageHandle, error) {
	s.clearStatus(ctx)
	_, providerMessageID, err := s.handler.finishSlackResponse(ctx, s.client, s.payload, text, s.fields)
	if err != nil {
		return nil, err
	}
	s.providerMessageID = providerMessageID
	return []gateway.MessageHandle{{
		ProviderMessageID: providerMessageID,
		ChannelID:         s.payload.ChannelID,
		ThreadID:          s.payload.ThreadTS,
		Raw: map[string]any{
			"provider": gateway.SlackProvider,
			"slack_ts": providerMessageID,
		},
	}}, nil
}

func (s *GatewaySlackSink) AfterSend(ctx context.Context, payload GatewayStreamPayload, result GatewayDeliveryResult) error {
	logging.FromContext(ctx).InfoContext(ctx, "gateway_slack_completed",
		"connection_id", s.payload.ConnectionID,
		"org_id", s.payload.OrgID,
		"channel_id", s.payload.ChannelID,
		"thread_ts", s.payload.ThreadTS,
		"token_count", result.TokenCount,
		"response_length", len(result.Text),
		"trace_id", payload.TraceID,
		"turn_id", payload.TurnID,
	)
	return nil
}

func (s *GatewaySlackSink) OnFailure(ctx context.Context, _ GatewayStreamPayload, _ error) error {
	s.clearStatus(ctx)
	return nil
}

func (s *GatewaySlackSink) clearStatus(ctx context.Context) {
	if s.statusCleared {
		return
	}
	s.handler.clearStatus(ctx, s.client, s.payload.ChannelID, s.payload.ThreadTS, s.fields)
	s.statusCleared = true
}

func gatewayStreamPayloadFromSlack(payload GatewaySlackPayload) GatewayStreamPayload {
	return GatewayStreamPayload{
		RouteID:          payload.RouteID,
		OrgID:            payload.OrgID,
		AgentID:          payload.AgentID,
		EventID:          payload.EventID,
		SessionID:        payload.SessionID,
		RuntimeSessionID: payload.RuntimeConvoID,
		StreamURL:        payload.StreamURL,
		RuntimeAPIKey:    payload.RuntimeAPIKey,
		TraceID:          payload.TraceID,
		TurnID:           payload.TurnID,
		Provider:         gateway.SlackProvider,
		ThreadKey:        payload.ChannelID + ":" + payload.ThreadTS,
		ChannelID:        payload.ChannelID,
		ThreadID:         payload.ThreadTS,
	}
}
