package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/gateway"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

const gatewayFriendlyStreamError = "Something went wrong. Please try again."

type GatewayStreamPayload struct {
	RouteID          string
	OrgID            string
	EmployeeID       string
	EventID          string
	SessionID        string
	RuntimeSessionID string
	StreamURL        string
	RuntimeAPIKey    string
	TraceID          string
	TurnID           string
	Provider         string
	ThreadKey        string
	ChannelID        string
	ThreadID         string
}

type GatewayDeliveryResult struct {
	Text       string
	Delivered  bool
	Handles    []gateway.MessageHandle
	TokenCount int
}

type GatewayResponseSink interface {
	Provider() string
	BeforeWait(context.Context, GatewayStreamPayload) error
	SendFinal(context.Context, GatewayStreamPayload, string) ([]gateway.MessageHandle, error)
	AfterSend(context.Context, GatewayStreamPayload, GatewayDeliveryResult) error
	OnFailure(context.Context, GatewayStreamPayload, error) error
}

type GatewayStreamDeliveryService struct {
	db         *gorm.DB
	subscriber func(context.Context, string, string) (<-chan gateway.SSEEvent, error)
}

func NewGatewayStreamDeliveryService(db *gorm.DB) *GatewayStreamDeliveryService {
	return &GatewayStreamDeliveryService{db: db}
}

func (s *GatewayStreamDeliveryService) DeliverFromStream(ctx context.Context, payload GatewayStreamPayload, sink GatewayResponseSink, fields map[string]any) (GatewayDeliveryResult, error) {
	subscribe := s.subscriber
	if subscribe == nil {
		subscriber := gateway.NewSSESubscriber(&http.Client{Timeout: 610 * time.Second})
		subscribe = subscriber.Subscribe
	}
	events, err := subscribe(ctx, payload.StreamURL, payload.RuntimeAPIKey)
	if err != nil {
		err = fmt.Errorf("subscribe to gateway response stream: %w", err)
		logging.CaptureWithFields(ctx, err, fields)
		return GatewayDeliveryResult{}, err
	}
	return s.DeliverEvents(ctx, payload, sink, events, fields)
}

func (s *GatewayStreamDeliveryService) DeliverEvents(ctx context.Context, payload GatewayStreamPayload, sink GatewayResponseSink, events <-chan gateway.SSEEvent, fields map[string]any) (GatewayDeliveryResult, error) {
	if sink == nil {
		return GatewayDeliveryResult{}, fmt.Errorf("gateway response sink is required")
	}
	if err := sink.BeforeWait(ctx, payload); err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway delivery before wait: %w", err), fields)
	}

	tokenCount := 0
	var streamedText strings.Builder
	for event := range events {
		if ctx.Err() != nil {
			err := ctx.Err()
			_ = sink.OnFailure(ctx, payload, err)
			logging.CaptureWithFields(ctx, fmt.Errorf("gateway stream context cancelled: %w", err), fields)
			return GatewayDeliveryResult{}, err
		}
		switch event.Type {
		case "token":
			text := eventText(event)
			if text == "" {
				continue
			}
			streamedText.WriteString(text)
			tokenCount++
		case "final":
			text := firstNonEmpty(streamedText.String(), eventText(event), "No response generated.")
			return s.sendFinal(ctx, payload, sink, text, tokenCount, fields)
		case "done":
			if strings.TrimSpace(streamedText.String()) == "" {
				logging.FromContext(ctx).InfoContext(ctx, "gateway stream done without visible response",
					"provider", payload.Provider, "route_id", payload.RouteID, "session_id", payload.SessionID)
				return GatewayDeliveryResult{TokenCount: tokenCount}, nil
			}
			return s.sendFinal(ctx, payload, sink, streamedText.String(), tokenCount, fields)
		case "error":
			err := fmt.Errorf("gateway stream emitted error")
			logging.CaptureWithFields(ctx, err, fields)
			return s.sendFinal(ctx, payload, sink, gatewayFriendlyStreamError, tokenCount, fields)
		case "thinking", "tool_call", "tool_result", "model_usage", "turn_started", "turn_completed":
			continue
		}
	}
	if strings.TrimSpace(streamedText.String()) != "" {
		result, err := s.sendFinal(ctx, payload, sink, streamedText.String(), tokenCount, fields)
		if err == nil {
			logging.FromContext(ctx).WarnContext(ctx, "gateway stream ended without terminal event; delivered accumulated tokens",
				"provider", payload.Provider, "route_id", payload.RouteID, "session_id", payload.SessionID)
		}
		return result, err
	}
	err := fmt.Errorf("gateway stream ended without final/done")
	_ = sink.OnFailure(ctx, payload, err)
	logging.CaptureWithFields(ctx, err, fields)
	return GatewayDeliveryResult{TokenCount: tokenCount}, err
}

func (s *GatewayStreamDeliveryService) sendFinal(ctx context.Context, payload GatewayStreamPayload, sink GatewayResponseSink, text string, tokenCount int, fields map[string]any) (GatewayDeliveryResult, error) {
	handles, err := sink.SendFinal(ctx, payload, text)
	if err != nil {
		_ = sink.OnFailure(ctx, payload, err)
		_ = s.recordDelivery(ctx, payload, text, handles, "failed", err.Error())
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway delivery send final: %w", err), fields)
		return GatewayDeliveryResult{Text: text, Handles: handles, TokenCount: tokenCount}, err
	}
	result := GatewayDeliveryResult{Text: text, Delivered: true, Handles: handles, TokenCount: tokenCount}
	if err := s.recordDelivery(ctx, payload, text, handles, "sent", ""); err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway delivery record: %w", err), fields)
	}
	if err := sink.AfterSend(ctx, payload, result); err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway delivery after send: %w", err), fields)
	}
	return result, nil
}

func (s *GatewayStreamDeliveryService) recordDelivery(ctx context.Context, payload GatewayStreamPayload, text string, handles []gateway.MessageHandle, status, errText string) error {
	if s == nil || s.db == nil {
		return nil
	}
	orgID, _ := parseUUID(payload.OrgID)
	employeeID, _ := parseUUID(payload.EmployeeID)
	sessionID, _ := parseUUID(payload.SessionID)
	routeID, _ := parseUUID(payload.RouteID)
	dedupe := payload.TraceID + ":" + payload.TurnID
	if strings.Trim(dedupe, ":") == "" {
		dedupe = payload.RuntimeSessionID + ":" + hashDeliveryText(text)
	}
	row := model.EmployeeGatewayDelivery{
		OrgID:             orgID,
		EmployeeID:        employeeID,
		RouteID:           routeIDPtr(routeID),
		EmployeeSessionID: sessionID,
		Provider:          firstNonEmpty(payload.Provider, sinkProvider(handles)),
		DedupeKey:         dedupe,
		RuntimeSessionID:  payload.RuntimeSessionID,
		RuntimeTraceID:    payload.TraceID,
		RuntimeTurnID:     payload.TurnID,
		ThreadKey:         payload.ThreadKey,
		ChannelID:         payload.ChannelID,
		ThreadID:          payload.ThreadID,
		ResponseText:      text,
		ProviderHandles:   handlesJSON(handles),
		Status:            status,
		Error:             errText,
	}
	if len(handles) > 0 {
		row.ChannelID = handles[0].ChannelID
		row.ThreadID = handles[0].ThreadID
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
}

func eventText(event gateway.SSEEvent) string {
	var data struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return ""
	}
	return data.Text
}

func routeIDPtr(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

func handlesJSON(handles []gateway.MessageHandle) model.RawJSON {
	if handles == nil {
		handles = []gateway.MessageHandle{}
	}
	encoded, err := json.Marshal(handles)
	if err != nil {
		return model.RawJSON("[]")
	}
	return model.RawJSON(encoded)
}

func hashDeliveryText(text string) string {
	return gateway.HashExternalGatewaySecret(text)[:16]
}

func sinkProvider(handles []gateway.MessageHandle) string {
	if len(handles) == 0 {
		return ""
	}
	if provider, ok := handles[0].Raw["provider"].(string); ok {
		return provider
	}
	return ""
}
