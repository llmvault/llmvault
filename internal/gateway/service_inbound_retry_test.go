package gateway

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

// flakyRuntime fails Send a configurable number of times before succeeding.
type flakyRuntime struct {
	mu           sync.Mutex
	failuresLeft int
	sends        int
}

func (r *flakyRuntime) Send(_ context.Context, message RuntimeMessage) (*RuntimeDelivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failuresLeft > 0 {
		r.failuresLeft--
		return nil, fmt.Errorf("runtime waking")
	}
	r.sends++
	return &RuntimeDelivery{
		SessionID: runtimeSessionID(message.ConversationID),
		StreamID:  "stream-" + message.GatewayExternalMsgID,
		TraceID:   "trace-" + message.GatewayExternalMsgID,
		TurnID:    "turn-" + message.GatewayExternalMsgID,
	}, nil
}

func (r *flakyRuntime) successfulSends() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sends
}

// An inbound message whose first delivery to the runtime failed is NOT permanently dropped as a
// duplicate: a redelivery (same dedupe key) reclaims the failed event row and re-runs Send.
func TestReceiveRetriesFailedInboundEvent(t *testing.T) {
	db := connectGatewayTestDB(t)
	route := seedGatewayRoute(t, db)
	runtime := &flakyRuntime{failuresLeft: 1}
	service := NewService(db, runtime, nil, NewFakeSlackAdapter())

	body := fakeSlackBody("500.000", "", "Please handle this")

	if _, err := service.ReceiveWebhook(t.Context(), WebhookEnvelope{RouteID: route.ID, Body: body}); err == nil {
		t.Fatalf("expected error from failed runtime send")
	}
	if runtime.successfulSends() != 0 {
		t.Fatalf("no successful send expected yet, got %d", runtime.successfulSends())
	}

	var status string
	db.Model(&model.EmployeeGatewayEvent{}).
		Where("route_id = ?", route.ID).
		Select("status").Scan(&status)
	if status != "failed" {
		t.Fatalf("event status = %q, want failed", status)
	}

	result, err := service.ReceiveWebhook(t.Context(), WebhookEnvelope{RouteID: route.ID, Body: body})
	if err != nil {
		t.Fatalf("retry receive webhook: %v", err)
	}
	if result.Duplicate {
		t.Fatalf("retry of a failed inbound must not be treated as a duplicate")
	}
	if runtime.successfulSends() != 1 {
		t.Fatalf("retry should have re-run Send, successful sends = %d", runtime.successfulSends())
	}

	dup, err := service.ReceiveWebhook(t.Context(), WebhookEnvelope{RouteID: route.ID, Body: body})
	if err != nil {
		t.Fatalf("duplicate receive webhook: %v", err)
	}
	if !dup.Duplicate {
		t.Fatalf("delivered inbound should now be deduped")
	}
	if runtime.successfulSends() != 1 {
		t.Fatalf("delivered inbound must not re-send, got %d", runtime.successfulSends())
	}

	var rows int64
	db.Model(&model.EmployeeGatewayEvent{}).Where("route_id = ?", route.ID).Count(&rows)
	if rows != 1 {
		t.Fatalf("expected a single inbound event row, got %d", rows)
	}
}
