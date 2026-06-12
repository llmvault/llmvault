package gateway

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// flakyAdapter wraps the fake-slack adapter but fails SendResponse a configurable number of times
// before succeeding, so we can exercise the failed-delivery retry path.
type flakyAdapter struct {
	provider     string
	mu           sync.Mutex
	failuresLeft int
	sends        int
}

func (a *flakyAdapter) Provider() string { return a.provider }

func (a *flakyAdapter) DecodeInbound(ctx context.Context, envelope WebhookEnvelope) (InboundEnvelope, bool, error) {
	inner := NewFakeSlackAdapter()
	in, ok, err := inner.DecodeInbound(ctx, envelope)
	in.Provider = a.provider
	return in, ok, err
}

func (a *flakyAdapter) FormatAgentRequest(ctx context.Context, inbound InboundEnvelope) (AgentRequest, error) {
	return (&FakeSlackAdapter{}).FormatAgentRequest(ctx, inbound)
}

func (a *flakyAdapter) RenderResponse(ctx context.Context, response AgentResponse) (ProviderResponsePayload, error) {
	return (&FakeSlackAdapter{}).RenderResponse(ctx, response)
}

func (a *flakyAdapter) SendResponse(_ context.Context, payload ProviderResponsePayload) ([]MessageHandle, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.failuresLeft > 0 {
		a.failuresLeft--
		return nil, fmt.Errorf("transient slack error")
	}
	a.sends++
	return []MessageHandle{{
		ProviderMessageID: fmt.Sprintf("flaky-%d", a.sends),
		ChannelID:         payload.ChannelID,
		ThreadID:          payload.ThreadID,
	}}, nil
}

func (a *flakyAdapter) successfulSends() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sends
}

// A transient send failure does not permanently occupy the dedupe key: a later HandleRuntimeFinal
// for the same turn re-sends and the reply is delivered.
func TestHandleRuntimeFinalRetriesFailedDelivery(t *testing.T) {
	db := connectGatewayTestDB(t)
	adapter := &flakyAdapter{provider: "flaky-slack", failuresLeft: 1}
	route := seedFlakyRoute(t, db, adapter.provider)
	runtime := &recordingRuntime{}
	service := NewService(db, runtime, nil, adapter)

	received, err := service.ReceiveWebhook(t.Context(), WebhookEnvelope{
		RouteID: route.ID,
		Body:    fakeSlackBody("400.000", "", "Please investigate"),
	})
	if err != nil {
		t.Fatalf("receive webhook: %v", err)
	}

	final := AgentResponse{
		RuntimeSessionID: received.Runtime.SessionID,
		TurnID:           "turn-retry-1",
		Text:             "All done.",
	}

	firstDelivery, err := service.HandleRuntimeFinal(t.Context(), final)
	if err != nil {
		t.Fatalf("handle runtime final (first): %v", err)
	}
	if firstDelivery == nil || firstDelivery.Status != "failed" {
		t.Fatalf("first delivery should be failed, got %#v", firstDelivery)
	}
	if adapter.successfulSends() != 0 {
		t.Fatalf("no successful send expected yet, got %d", adapter.successfulSends())
	}

	secondDelivery, err := service.HandleRuntimeFinal(t.Context(), final)
	if err != nil {
		t.Fatalf("handle runtime final (retry): %v", err)
	}
	if secondDelivery == nil || secondDelivery.Status != "sent" {
		t.Fatalf("retry delivery should be sent, got %#v", secondDelivery)
	}
	if secondDelivery.ID != firstDelivery.ID {
		t.Fatalf("retry should update the existing dedupe row, not insert a new one: first=%s second=%s", firstDelivery.ID, secondDelivery.ID)
	}
	if adapter.successfulSends() != 1 {
		t.Fatalf("expected exactly one successful send, got %d", adapter.successfulSends())
	}

	thirdDelivery, err := service.HandleRuntimeFinal(t.Context(), final)
	if err != nil {
		t.Fatalf("handle runtime final (dedupe): %v", err)
	}
	if thirdDelivery == nil || thirdDelivery.ID != firstDelivery.ID {
		t.Fatalf("third attempt should return existing sent delivery")
	}
	if adapter.successfulSends() != 1 {
		t.Fatalf("successful send must remain deduped, got %d", adapter.successfulSends())
	}

	var rows int64
	db.Model(&model.EmployeeGatewayDelivery{}).Where("route_id = ? AND dedupe_key = ?", route.ID, firstDelivery.DedupeKey).Count(&rows)
	if rows != 1 {
		t.Fatalf("expected exactly one delivery row for the dedupe key, got %d", rows)
	}
}

func seedFlakyRoute(t *testing.T, db *gorm.DB, provider string) model.EmployeeGatewayRoute {
	t.Helper()
	org := model.Org{Name: "Gateway Flaky " + uuid.NewString()}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	employee := model.Employee{OrgID: &org.ID, Model: "test-model", Status: "active"}
	if err := db.Create(&employee).Error; err != nil {
		t.Fatalf("create employee: %v", err)
	}
	sandbox := model.Sandbox{
		OrgID:                  &org.ID,
		EmployeeID:             &employee.ID,
		ExternalID:             "gateway-flaky-" + uuid.NewString(),
		RuntimeURL:             "http://localhost:1",
		EncryptedRuntimeSecret: []byte("test-key"),
		Status:                 "running",
	}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	route := model.EmployeeGatewayRoute{
		OrgID:      org.ID,
		EmployeeID: employee.ID,
		Provider:   provider,
		Name:       "Flaky Slack",
		Enabled:    true,
		Config:     model.JSON{},
	}
	if err := db.Create(&route).Error; err != nil {
		t.Fatalf("create route: %v", err)
	}
	return route
}
