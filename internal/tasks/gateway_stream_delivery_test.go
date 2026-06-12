package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/usehivy/hivy/internal/gateway"
)

type recordingGatewaySink struct {
	sentText string
	handles  []gateway.MessageHandle
	failSend error
}

func (s *recordingGatewaySink) Provider() string { return "test" }
func (s *recordingGatewaySink) BeforeWait(context.Context, GatewayStreamPayload) error {
	return nil
}
func (s *recordingGatewaySink) SendFinal(_ context.Context, payload GatewayStreamPayload, text string) ([]gateway.MessageHandle, error) {
	s.sentText = text
	if s.failSend != nil {
		return nil, s.failSend
	}
	s.handles = []gateway.MessageHandle{{ProviderMessageID: "provider-1", ChannelID: payload.ChannelID, ThreadID: payload.ThreadID}}
	return s.handles, nil
}
func (s *recordingGatewaySink) AfterSend(context.Context, GatewayStreamPayload, GatewayDeliveryResult) error {
	return nil
}
func (s *recordingGatewaySink) OnFailure(context.Context, GatewayStreamPayload, error) error {
	return nil
}

// The runtime's final event is authoritative: it must win over the streamed token
// accumulation (which can drop/duplicate tokens), falling back to streamed text
// only when the final event is empty.
func TestGatewayStreamDeliveryPrefersFinalTextOverAccumulatedTokens(t *testing.T) {
	sink := &recordingGatewaySink{}
	result, err := NewGatewayStreamDeliveryService(nil).DeliverEvents(
		context.Background(),
		GatewayStreamPayload{Provider: "test", ChannelID: "C1", ThreadID: "T1"},
		sink,
		gatewaySlackEvents(
			gateway.SSEEvent{Type: "token", Data: json.RawMessage(`{"text":"not"}`)},
			gateway.SSEEvent{Type: "token", Data: json.RawMessage(`{"text":"ion"}`)},
			gateway.SSEEvent{Type: "final", Data: json.RawMessage(`{"text":"the full answer"}`)},
		),
		map[string]any{},
	)
	if err != nil {
		t.Fatalf("deliver events: %v", err)
	}
	if !result.Delivered || sink.sentText != "the full answer" {
		t.Fatalf("delivered=%v text=%q", result.Delivered, sink.sentText)
	}
}

func TestGatewayStreamDeliveryFallsBackToTokensWhenFinalEmpty(t *testing.T) {
	sink := &recordingGatewaySink{}
	result, err := NewGatewayStreamDeliveryService(nil).DeliverEvents(
		context.Background(),
		GatewayStreamPayload{Provider: "test", ChannelID: "C1", ThreadID: "T1"},
		sink,
		gatewaySlackEvents(
			gateway.SSEEvent{Type: "token", Data: json.RawMessage(`{"text":"not"}`)},
			gateway.SSEEvent{Type: "token", Data: json.RawMessage(`{"text":"ion"}`)},
			gateway.SSEEvent{Type: "final", Data: json.RawMessage(`{"text":""}`)},
		),
		map[string]any{},
	)
	if err != nil {
		t.Fatalf("deliver events: %v", err)
	}
	if !result.Delivered || sink.sentText != "notion" {
		t.Fatalf("delivered=%v text=%q", result.Delivered, sink.sentText)
	}
}

func TestGatewayStreamDeliverySendsFriendlyTextOnStreamError(t *testing.T) {
	sink := &recordingGatewaySink{}
	result, err := NewGatewayStreamDeliveryService(nil).DeliverEvents(
		context.Background(),
		GatewayStreamPayload{Provider: "test", ChannelID: "C1", ThreadID: "T1"},
		sink,
		gatewaySlackEvents(gateway.SSEEvent{Type: "error", Data: json.RawMessage(`{"message":"internal"}`)}),
		map[string]any{},
	)
	if err != nil {
		t.Fatalf("deliver events: %v", err)
	}
	if !result.Delivered || sink.sentText != gatewayFriendlyStreamError {
		t.Fatalf("delivered=%v text=%q", result.Delivered, sink.sentText)
	}
}

// A transport-level stream failure (EventStreamError) must trigger a subscription
// retry, not be delivered as a truncated final response. The broker replays
// history, so the retry observes the real final event.
func TestGatewayStreamDeliveryRetriesOnTransportError(t *testing.T) {
	sink := &recordingGatewaySink{}
	var attempts int
	svc := NewGatewayStreamDeliveryService(nil)
	svc.subscriber = func(context.Context, string, string) (<-chan gateway.SSEEvent, error) {
		attempts++
		if attempts == 1 {
			return gatewaySlackEvents(
				gateway.SSEEvent{Type: "token", Data: json.RawMessage(`{"text":"par"}`)},
				gateway.SSEEvent{Type: gateway.EventStreamError, Err: errors.New("connection reset")},
			), nil
		}
		return gatewaySlackEvents(
			gateway.SSEEvent{Type: "token", Data: json.RawMessage(`{"text":"par"}`)},
			gateway.SSEEvent{Type: "token", Data: json.RawMessage(`{"text":"tial"}`)},
			gateway.SSEEvent{Type: "final", Data: json.RawMessage(`{"text":"the complete answer"}`)},
		), nil
	}

	result, err := svc.DeliverFromStream(
		context.Background(),
		GatewayStreamPayload{Provider: "test", ChannelID: "C1", ThreadID: "T1"},
		sink,
		map[string]any{},
	)
	if err != nil {
		t.Fatalf("deliver from stream: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 subscribe attempts, got %d", attempts)
	}
	if !result.Delivered || sink.sentText != "the complete answer" {
		t.Fatalf("delivered=%v text=%q", result.Delivered, sink.sentText)
	}
}

// A clean EOF without a terminal event is genuine truncation, not a transport
// blip: it must NOT trigger a retry; the partial text is delivered as-is.
func TestGatewayStreamDeliveryDoesNotRetryOnCleanEOF(t *testing.T) {
	sink := &recordingGatewaySink{}
	var attempts int
	svc := NewGatewayStreamDeliveryService(nil)
	svc.subscriber = func(context.Context, string, string) (<-chan gateway.SSEEvent, error) {
		attempts++
		return gatewaySlackEvents(
			gateway.SSEEvent{Type: "token", Data: json.RawMessage(`{"text":"partial"}`)},
			gateway.SSEEvent{Type: gateway.EventStreamEOF},
		), nil
	}

	result, err := svc.DeliverFromStream(
		context.Background(),
		GatewayStreamPayload{Provider: "test", ChannelID: "C1", ThreadID: "T1"},
		sink,
		map[string]any{},
	)
	if err != nil {
		t.Fatalf("deliver from stream: %v", err)
	}
	if attempts != 1 {
		t.Fatalf("expected exactly 1 subscribe attempt on clean EOF, got %d", attempts)
	}
	if !result.Delivered || sink.sentText != "partial" {
		t.Fatalf("delivered=%v text=%q", result.Delivered, sink.sentText)
	}
}

func TestGatewayStreamDeliveryReturnsProviderSendError(t *testing.T) {
	sink := &recordingGatewaySink{failSend: errors.New("provider down")}
	_, err := NewGatewayStreamDeliveryService(nil).DeliverEvents(
		context.Background(),
		GatewayStreamPayload{Provider: "test", ChannelID: "C1", ThreadID: "T1"},
		sink,
		gatewaySlackEvents(gateway.SSEEvent{Type: "final", Data: json.RawMessage(`{"text":"answer"}`)}),
		map[string]any{},
	)
	if err == nil {
		t.Fatal("expected provider send error")
	}
}
