package tasks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/gateway"
	"github.com/usehivy/hivy/internal/logging"
)

type GatewayExternalCallbackHandler struct {
	db     *gorm.DB
	client *http.Client
}

func NewGatewayExternalCallbackHandler(db *gorm.DB) *GatewayExternalCallbackHandler {
	return &GatewayExternalCallbackHandler{
		db:     db,
		client: &http.Client{Timeout: 20 * time.Second},
	}
}

func (h *GatewayExternalCallbackHandler) Handle(ctx context.Context, t *asynq.Task) error {
	var payload GatewayExternalCallbackPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway external callback: unmarshal payload: %w", err), map[string]any{
			"task_type": t.Type(),
		})
		return err
	}
	fields := map[string]any{
		"route_id":    payload.RouteID,
		"org_id":      payload.OrgID,
		"employee_id": payload.EmployeeID,
		"event_id":    payload.EventID,
		"session_id":  payload.SessionID,
		"trace_id":    payload.TraceID,
		"turn_id":     payload.TurnID,
		"provider":    payload.Provider,
	}
	sink := &ExternalCallbackSink{payload: payload, client: h.client}
	_, err := NewGatewayStreamDeliveryService(h.db).DeliverFromStream(ctx, gatewayStreamPayloadFromExternal(payload), sink, fields)
	return err
}

type ExternalCallbackSink struct {
	payload GatewayExternalCallbackPayload
	client  *http.Client
}

func (s *ExternalCallbackSink) Provider() string {
	return s.payload.Provider
}

func (s *ExternalCallbackSink) BeforeWait(context.Context, GatewayStreamPayload) error {
	return nil
}

func (s *ExternalCallbackSink) SendFinal(ctx context.Context, payload GatewayStreamPayload, text string) ([]gateway.MessageHandle, error) {
	body, err := json.Marshal(map[string]any{
		"route_id":              s.payload.RouteID,
		"event_id":              s.payload.EventID,
		"employee_session_id":   s.payload.SessionID,
		"runtime_session_id":    s.payload.RuntimeConvoID,
		"runtime_trace_id":      s.payload.TraceID,
		"runtime_turn_id":       s.payload.TurnID,
		"thread_id":             s.payload.ThreadID,
		"thread_key":            s.payload.ThreadKey,
		"channel_id":            s.payload.ChannelID,
		"provider":              s.payload.Provider,
		"text":                  text,
		"markdown":              text,
		"response_id":           responseID(s.payload, text),
		"gateway_delivery_type": "final",
	})
	if err != nil {
		return nil, fmt.Errorf("encode external gateway callback: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.payload.CallbackURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build external gateway callback request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hivy-Gateway-Route-ID", s.payload.RouteID)
	req.Header.Set("X-Hivy-Gateway-Event-ID", s.payload.EventID)
	req.Header.Set("X-Hivy-Signature", "sha256="+hmacHex(s.payload.RouteSecret, body))
	client := s.client
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("post external gateway callback: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("external gateway callback returned %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	return []gateway.MessageHandle{{
		ProviderMessageID: responseID(s.payload, text),
		ChannelID:         payload.ChannelID,
		ThreadID:          payload.ThreadID,
		Raw: map[string]any{
			"provider":     s.payload.Provider,
			"callback_url": s.payload.CallbackURL,
		},
	}}, nil
}

func (s *ExternalCallbackSink) AfterSend(ctx context.Context, _ GatewayStreamPayload, result GatewayDeliveryResult) error {
	logging.FromContext(ctx).InfoContext(ctx, "gateway_external_callback_completed",
		"route_id", s.payload.RouteID,
		"org_id", s.payload.OrgID,
		"employee_id", s.payload.EmployeeID,
		"event_id", s.payload.EventID,
		"provider", s.payload.Provider,
		"token_count", result.TokenCount,
		"response_length", len(result.Text),
	)
	return nil
}

func (s *ExternalCallbackSink) OnFailure(context.Context, GatewayStreamPayload, error) error {
	return nil
}

func gatewayStreamPayloadFromExternal(payload GatewayExternalCallbackPayload) GatewayStreamPayload {
	return GatewayStreamPayload{
		RouteID:          payload.RouteID,
		OrgID:            payload.OrgID,
		EmployeeID:       payload.EmployeeID,
		EventID:          payload.EventID,
		SessionID:        payload.SessionID,
		RuntimeSessionID: payload.RuntimeConvoID,
		StreamURL:        payload.StreamURL,
		RuntimeAPIKey:    payload.RuntimeAPIKey,
		TraceID:          payload.TraceID,
		TurnID:           payload.TurnID,
		Provider:         payload.Provider,
		ThreadKey:        payload.ThreadKey,
		ChannelID:        payload.ChannelID,
		ThreadID:         payload.ThreadID,
	}
}

func hmacHex(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func responseID(payload GatewayExternalCallbackPayload, text string) string {
	sum := sha256.Sum256([]byte(payload.RouteID + ":" + payload.EventID + ":" + payload.TurnID + ":" + text))
	return hex.EncodeToString(sum[:])[:32]
}
