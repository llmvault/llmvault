package tasks

import (
	"context"
	"encoding/json"
	"errors"
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

// maxStreamSubscribeAttempts bounds how many times DeliverFromStream
// re-subscribes after a transport-level stream failure before giving up and
// delivering whatever partial text it has. The broker replays history on
// each subscribe (so a reconnect can still observe the terminal event), but
// we must not retry forever.
const maxStreamSubscribeAttempts = 3

// errStreamTransport signals that the SSE stream ended due to a transport
// failure (dropped connection / read error) rather than a clean EOF. The
// caller should retry the subscription before falling back to partial text.
type errStreamTransport struct{ err error }

func (e errStreamTransport) Error() string { return "gateway stream transport failure: " + e.err.Error() }
func (e errStreamTransport) Unwrap() error { return e.err }

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

	var lastResult GatewayDeliveryResult
	var lastErr error
	for attempt := 1; attempt <= maxStreamSubscribeAttempts; attempt++ {
		if ctx.Err() != nil {
			return GatewayDeliveryResult{}, ctx.Err()
		}
		events, err := subscribe(ctx, payload.StreamURL, payload.RuntimeAPIKey)
		if err != nil {
			lastErr = fmt.Errorf("subscribe to gateway response stream: %w", err)
			logging.CaptureWithFields(ctx, lastErr, fields)
			// A connection-level subscribe failure is itself retryable.
			continue
		}
		result, err := s.deliverEventsOnce(ctx, payload, sink, events, fields)
		var transport errStreamTransport
		if errors.As(err, &transport) {
			// The stream dropped mid-flight; the broker replays history, so
			// re-subscribe and try to observe the terminal event again.
			lastResult, lastErr = result, err
			logging.FromContext(ctx).WarnContext(ctx, "gateway stream transport failure, retrying subscription",
				"attempt", attempt, "provider", payload.Provider, "route_id", payload.RouteID, "session_id", payload.SessionID, "error", err)
			continue
		}
		return result, err
	}

	// Retries exhausted on a transport failure: fall back to delivering the
	// partial text we accumulated on the last attempt, if any.
	if strings.TrimSpace(lastResult.Text) != "" && !lastResult.Delivered {
		return s.sendFinal(ctx, payload, sink, lastResult.Text, lastResult.TokenCount, fields)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("gateway stream failed after %d attempts", maxStreamSubscribeAttempts)
	}
	_ = sink.OnFailure(ctx, payload, lastErr)
	logging.CaptureWithFields(ctx, lastErr, fields)
	return lastResult, lastErr
}

// DeliverEvents consumes a single SSE event stream to completion. A
// transport-level stream failure (EventStreamError) is treated as a
// non-retryable end here — direct callers get the accumulated partial text.
// DeliverFromStream wraps this with subscription retries.
func (s *GatewayStreamDeliveryService) DeliverEvents(ctx context.Context, payload GatewayStreamPayload, sink GatewayResponseSink, events <-chan gateway.SSEEvent, fields map[string]any) (GatewayDeliveryResult, error) {
	result, err := s.deliverEventsOnce(ctx, payload, sink, events, fields)
	var transport errStreamTransport
	if errors.As(err, &transport) {
		// No retry channel available here: deliver partial text if we have any.
		if strings.TrimSpace(result.Text) != "" && !result.Delivered {
			return s.sendFinal(ctx, payload, sink, result.Text, result.TokenCount, fields)
		}
		_ = sink.OnFailure(ctx, payload, err)
		logging.CaptureWithFields(ctx, err, fields)
		return result, err
	}
	return result, err
}

// deliverEventsOnce runs one pass over an event stream. It returns an
// errStreamTransport when the stream ended on a transport failure (the
// caller may re-subscribe); the returned result carries the accumulated
// partial text/token count so the caller can fall back to it.
func (s *GatewayStreamDeliveryService) deliverEventsOnce(ctx context.Context, payload GatewayStreamPayload, sink GatewayResponseSink, events <-chan gateway.SSEEvent, fields map[string]any) (GatewayDeliveryResult, error) {
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
			// Prefer the runtime's authoritative final text; fall back to the
			// streamed token accumulation only when the final event is empty.
			text := firstNonEmpty(eventText(event), streamedText.String(), "No response generated.")
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
		case gateway.EventStreamError:
			// Transport failure: surface as retryable, carrying partial text.
			return GatewayDeliveryResult{Text: streamedText.String(), TokenCount: tokenCount},
				errStreamTransport{err: firstNonNilErr(event.Err, fmt.Errorf("gateway stream transport closed"))}
		case gateway.EventStreamEOF:
			// Clean EOF without a terminal event: genuine truncation, not
			// retryable. Fall through to the post-loop partial handling.
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

func firstNonNilErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

func (s *GatewayStreamDeliveryService) sendFinal(ctx context.Context, payload GatewayStreamPayload, sink GatewayResponseSink, text string, tokenCount int, fields map[string]any) (GatewayDeliveryResult, error) {
	// Pre-send dedupe: an asynq retry of this task re-runs the whole stream.
	// The post-send dedupe row alone can't prevent a duplicate provider
	// message because SendFinal happens before the row is written. If a prior
	// attempt already delivered this turn, skip the send entirely.
	if existing, ok := s.alreadyDelivered(ctx, payload, text); ok {
		logging.FromContext(ctx).InfoContext(ctx, "gateway delivery already sent; skipping duplicate",
			"provider", payload.Provider, "route_id", payload.RouteID, "session_id", payload.SessionID)
		handles := decodeHandles(existing.ProviderHandles)
		return GatewayDeliveryResult{Text: existing.ResponseText, Delivered: true, Handles: handles, TokenCount: tokenCount}, nil
	}

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
	dedupe := deliveryDedupeKey(payload, text)
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

// deliveryDedupeKey derives the stable dedupe identity for a delivery.
// Prefer the runtime trace+turn (stable across asynq retries and independent
// of text); fall back to runtime session + text hash only when those are
// absent.
func deliveryDedupeKey(payload GatewayStreamPayload, text string) string {
	dedupe := payload.TraceID + ":" + payload.TurnID
	if strings.Trim(dedupe, ":") == "" {
		dedupe = payload.RuntimeSessionID + ":" + hashDeliveryText(text)
	}
	return dedupe
}

// alreadyDelivered reports whether a prior attempt already sent this turn,
// returning the existing row so the caller can reuse its handles/text. Only
// rows with status "sent" count — a prior "failed" row must be retried.
func (s *GatewayStreamDeliveryService) alreadyDelivered(ctx context.Context, payload GatewayStreamPayload, text string) (model.EmployeeGatewayDelivery, bool) {
	if s == nil || s.db == nil {
		return model.EmployeeGatewayDelivery{}, false
	}
	dedupe := deliveryDedupeKey(payload, text)
	if strings.Trim(dedupe, ":") == "" {
		return model.EmployeeGatewayDelivery{}, false
	}
	var row model.EmployeeGatewayDelivery
	err := s.db.WithContext(ctx).
		Where("dedupe_key = ? AND status = ?", dedupe, "sent").
		Take(&row).Error
	if err != nil {
		return model.EmployeeGatewayDelivery{}, false
	}
	return row, true
}

func decodeHandles(raw model.RawJSON) []gateway.MessageHandle {
	if len(raw) == 0 {
		return nil
	}
	var handles []gateway.MessageHandle
	if err := json.Unmarshal(raw, &handles); err != nil {
		return nil
	}
	return handles
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
