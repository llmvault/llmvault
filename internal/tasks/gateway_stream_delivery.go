package tasks

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/gateway"
	"github.com/usehivy/hivy/internal/logging"
)

const gatewayFriendlyStreamError = "Something went wrong. Please try again."

// maxStreamSubscribeAttempts bounds re-subscribes after a transport failure
// before delivering whatever partial text we have. The broker replays history on
// each subscribe (so a reconnect can still see the terminal event).
const maxStreamSubscribeAttempts = 5

// errStreamTransport signals that the SSE stream ended due to a transport
// failure (dropped connection / read error) rather than a clean EOF. The
// caller should retry the subscription before falling back to partial text.
type errStreamTransport struct{ err error }

func (e errStreamTransport) Error() string {
	return "gateway stream transport failure: " + e.err.Error()
}
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
		subscriber := gateway.NewSSESubscriber(&http.Client{Timeout: 660 * time.Second})
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
// transport-level stream failure (EventStreamError) is treated as a non-retryable
// end here — direct callers get the accumulated partial text. DeliverFromStream
// wraps this with subscription retries.
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

// deliverEventsOnce runs one pass over an event stream, returning
// errStreamTransport on a transport failure (caller may re-subscribe); the
// result carries the accumulated partial text so the caller can fall back.
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
	// Pre-send dedupe: an asynq retry re-runs the whole stream, and SendFinal
	// happens before the row is written, so the post-send row alone can't prevent
	// a duplicate provider message. Skip the send if a prior attempt delivered.
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
