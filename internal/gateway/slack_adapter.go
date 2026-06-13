package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	slacksdk "github.com/slack-go/slack"

	"github.com/usehivy/hivy/internal/slackgateway"
)

const SlackProvider = "slack"

var slackMentionRE = regexp.MustCompile(`<@[^>]+>`)

type SlackAdapter struct {
	sender SlackResponseSender
}

type SlackAdapterOption func(*SlackAdapter)

type SlackResponseSender interface {
	SendSlackResponse(context.Context, ProviderResponsePayload) ([]MessageHandle, error)
}

func NewSlackAdapter(opts ...SlackAdapterOption) *SlackAdapter {
	adapter := &SlackAdapter{}
	for _, opt := range opts {
		if opt != nil {
			opt(adapter)
		}
	}
	return adapter
}

func WithSlackResponseSender(sender SlackResponseSender) SlackAdapterOption {
	return func(adapter *SlackAdapter) {
		adapter.sender = sender
	}
}

func (a *SlackAdapter) Provider() string {
	return SlackProvider
}

type slackEventCallback struct {
	Type    string     `json:"type"`
	TeamID  string     `json:"team_id"`
	EventID string     `json:"event_id"`
	Event   slackEvent `json:"event"`
}

type slackEvent struct {
	Type            string                `json:"type"`
	Team            string                `json:"team"`
	Channel         string                `json:"channel"`
	User            string                `json:"user"`
	Text            string                `json:"text"`
	TS              string                `json:"ts"`
	ThreadTS        string                `json:"thread_ts"`
	BotID           string                `json:"bot_id"`
	Subtype         string                `json:"subtype"`
	ChannelType     string                `json:"channel_type"`
	Hidden          bool                  `json:"hidden"`
	AssistantThread *slackAssistantThread `json:"assistant_thread,omitempty"`
}

type slackAssistantThread struct {
	ActionToken string `json:"action_token"`
}

func (a *SlackAdapter) DecodeInbound(_ context.Context, envelope WebhookEnvelope) (InboundEnvelope, bool, error) {
	var callback slackEventCallback
	if err := json.Unmarshal(envelope.Body, &callback); err != nil {
		return InboundEnvelope{}, false, fmt.Errorf("decode slack event: %w", err)
	}

	if callback.Type == "url_verification" {
		return InboundEnvelope{}, false, nil
	}

	if callback.Type != "event_callback" {
		return InboundEnvelope{}, false, nil
	}

	event := callback.Event

	if event.Type != "app_mention" && event.Type != "message" {
		return InboundEnvelope{}, false, nil
	}

	if event.BotID != "" || event.Subtype == "bot_message" {
		return InboundEnvelope{}, false, nil
	}
	if event.Hidden {
		return InboundEnvelope{}, false, nil
	}
	if event.Type == "message" {
		if event.Subtype != "" {
			return InboundEnvelope{}, false, nil
		}
		if strings.TrimSpace(event.ThreadTS) == "" {
			return InboundEnvelope{}, false, nil
		}
	}

	if strings.TrimSpace(event.Channel) == "" {
		return InboundEnvelope{}, false, fmt.Errorf("slack event missing channel")
	}

	if strings.TrimSpace(event.TS) == "" {
		return InboundEnvelope{}, false, fmt.Errorf("slack event missing ts")
	}

	threadTS := event.ThreadTS
	if threadTS == "" {
		threadTS = event.TS
	}

	threadKey := event.Channel + ":" + threadTS

	teamID := callback.TeamID
	if teamID == "" {
		teamID = event.Team
	}
	raw := map[string]any{
		"team_id":       teamID,
		"event_id":      callback.EventID,
		"ts":            event.TS,
		"thread_ts":     threadTS,
		"event_type":    event.Type,
		"event_subtype": event.Subtype,
		"channel_type":  event.ChannelType,
		"is_thread_reply": event.ThreadTS != "" &&
			event.ThreadTS != event.TS,
	}
	if event.AssistantThread != nil && event.AssistantThread.ActionToken != "" {
		raw["action_token"] = event.AssistantThread.ActionToken
	}

	return InboundEnvelope{
		Provider:          SlackProvider,
		ExternalMessageID: event.TS,
		DedupeKey:         callback.EventID,
		ThreadKey:         threadKey,
		ChannelID:         event.Channel,
		ThreadID:          threadTS,
		SenderID:          event.User,
		Text:              strings.TrimSpace(event.Text),
		Raw:               raw,
		ReceivedAt:        time.Now().UTC(),
	}, true, nil
}

func (a *SlackAdapter) FormatAgentRequest(_ context.Context, inbound InboundEnvelope) (AgentRequest, error) {
	text := cleanSlackPromptText(inbound.Text)
	if text == "" {
		return AgentRequest{}, fmt.Errorf("format slack message: text is required")
	}
	return AgentRequest{
		Markdown: fmt.Sprintf("Slack message:\n\n%s", text),
		Metadata: map[string]any{
			"sender_id":  inbound.SenderID,
			"channel_id": inbound.ChannelID,
			"thread_ts":  inbound.ThreadID,
			"provider":   SlackProvider,
		},
	}, nil
}

func cleanSlackPromptText(text string) string {
	text = slackMentionRE.ReplaceAllString(text, "")
	return strings.Join(strings.Fields(text), " ")
}

func (a *SlackAdapter) RenderResponse(_ context.Context, response AgentResponse) (ProviderResponsePayload, error) {
	text := strings.TrimSpace(response.Text)
	if text == "" {
		return ProviderResponsePayload{}, fmt.Errorf("render slack response: text is required")
	}
	return ProviderResponsePayload{
		Route:     response.Route,
		Session:   response.AgentSession,
		ChannelID: response.ChannelID,
		ThreadID:  response.ThreadID,
		Text:      text,
	}, nil
}

func (a *SlackAdapter) SendResponse(ctx context.Context, payload ProviderResponsePayload) ([]MessageHandle, error) {
	if a.sender == nil {
		return nil, fmt.Errorf("slack adapter SendResponse not configured")
	}
	return a.sender.SendSlackResponse(ctx, payload)
}

func (a *SlackAdapter) ActionToken(raw map[string]any) string {
	if raw == nil {
		return ""
	}
	if token, ok := raw["action_token"].(string); ok {
		return token
	}
	return ""
}

type slackNangoClient interface {
	GetConnection(context.Context, string, string) (map[string]any, error)
}

type SlackNangoResponseSender struct {
	nangoClient        slackNangoClient
	slackClientFactory func(string) slackgateway.ThreadReplyClient
}

func NewSlackNangoResponseSender(nangoClient slackNangoClient) *SlackNangoResponseSender {
	return &SlackNangoResponseSender{nangoClient: nangoClient}
}

func (s *SlackNangoResponseSender) SetSlackClientFactory(factory func(string) slackgateway.ThreadReplyClient) {
	s.slackClientFactory = factory
}

func (s *SlackNangoResponseSender) SendSlackResponse(ctx context.Context, payload ProviderResponsePayload) ([]MessageHandle, error) {
	if s == nil || s.nangoClient == nil {
		return nil, fmt.Errorf("slack response sender not configured")
	}
	if strings.TrimSpace(payload.ChannelID) == "" {
		return nil, fmt.Errorf("send slack response: channel_id is required")
	}
	if strings.TrimSpace(payload.ThreadID) == "" {
		return nil, fmt.Errorf("send slack response: thread_id is required")
	}
	if strings.TrimSpace(payload.Text) == "" {
		return nil, fmt.Errorf("send slack response: text is required")
	}

	connection := payload.Route.Connection
	if connection == nil || strings.TrimSpace(connection.NangoConnectionID) == "" {
		return nil, fmt.Errorf("send slack response: route connection is required")
	}
	providerKey := strings.TrimSpace(connection.Integration.UniqueKey)
	if providerKey == "" {
		providerKey = SlackProvider
	}
	botToken, err := s.loadBotToken(ctx, connection.NangoConnectionID, providerKey)
	if err != nil {
		return nil, err
	}

	messageTS, err := slackgateway.PostThreadReply(ctx, s.newSlackClient(botToken), payload.ChannelID, payload.ThreadID, payload.Text, map[string]any{
		"connection_id": connection.ID.String(),
		"org_id":        payload.Session.OrgID.String(),
		"agent_id":      payload.Session.AgentID.String(),
		"channel_id":    payload.ChannelID,
		"thread_ts":     payload.ThreadID,
	})
	if err != nil {
		return nil, err
	}
	return []MessageHandle{{
		ProviderMessageID: messageTS,
		ChannelID:         payload.ChannelID,
		ThreadID:          payload.ThreadID,
		Raw: map[string]any{
			"provider": SlackProvider,
			"slack_ts": messageTS,
		},
	}}, nil
}

func (s *SlackNangoResponseSender) newSlackClient(botToken string) slackgateway.ThreadReplyClient {
	if s.slackClientFactory != nil {
		return s.slackClientFactory(botToken)
	}
	return slacksdk.New(botToken)
}

func (s *SlackNangoResponseSender) loadBotToken(ctx context.Context, nangoConnID, providerKey string) (string, error) {
	nangoConn, err := s.nangoClient.GetConnection(ctx, nangoConnID, providerKey)
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
