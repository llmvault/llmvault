package tasks

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/gateway"
	"github.com/usehivy/hivy/internal/model"
)

// countingGatewaySink records how many times SendFinal is invoked so the
// dedupe test can assert that an asynq retry does not re-post to the provider.
type countingGatewaySink struct {
	sends int
}

func (s *countingGatewaySink) Provider() string                                       { return "test" }
func (s *countingGatewaySink) BeforeWait(context.Context, GatewayStreamPayload) error { return nil }
func (s *countingGatewaySink) SendFinal(_ context.Context, payload GatewayStreamPayload, _ string) ([]gateway.MessageHandle, error) {
	s.sends++
	return []gateway.MessageHandle{{ProviderMessageID: "provider-1", ChannelID: payload.ChannelID, ThreadID: payload.ThreadID}}, nil
}
func (s *countingGatewaySink) AfterSend(context.Context, GatewayStreamPayload, GatewayDeliveryResult) error {
	return nil
}
func (s *countingGatewaySink) OnFailure(context.Context, GatewayStreamPayload, error) error {
	return nil
}

func seedGatewayDeliveryParents(t *testing.T, db *gorm.DB) (orgID, employeeID, sessionID uuid.UUID) {
	t.Helper()
	orgID = uuid.New()
	if err := db.Create(&model.Org{ID: orgID, Name: "gw-dedupe-" + uuid.NewString()[:8], Active: true}).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Employee{ID: uuid.New(), OrgID: &orgID, Model: "test-model", Status: "active"}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create employee: %v", err)
	}
	sb := model.Sandbox{ID: uuid.New(), OrgID: &orgID, EmployeeID: &agent.ID, EncryptedRuntimeSecret: []byte("secret"), Status: "running"}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	session := model.EmployeeSession{
		ID:                    uuid.New(),
		OrgID:                 orgID,
		EmployeeID:            agent.ID,
		SandboxID:             sb.ID,
		RuntimeConversationID: "gw-dedupe-conv-" + uuid.NewString()[:8],
		Status:                "active",
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	return orgID, agent.ID, session.ID
}

// An asynq retry of the same turn must NOT re-post: the pre-send dedupe check
// finds the prior "sent" row and short-circuits.
func TestGatewayStreamDeliverySkipsDuplicateSendOnRetry(t *testing.T) {
	db := openTasksMemoryTestDB(t)
	orgID, employeeID, sessionID := seedGatewayDeliveryParents(t, db)
	svc := NewGatewayStreamDeliveryService(db)

	payload := GatewayStreamPayload{
		Provider:         "test",
		OrgID:            orgID.String(),
		EmployeeID:       employeeID.String(),
		SessionID:        sessionID.String(),
		RuntimeSessionID: "rt-session-1",
		TraceID:          "trace-dedupe-" + uuid.NewString(),
		TurnID:           "turn-1",
		ChannelID:        "C1",
		ThreadID:         "T1",
	}
	events := func() <-chan gateway.SSEEvent {
		return gatewaySlackEvents(
			gateway.SSEEvent{Type: "final", Data: json.RawMessage(`{"text":"the answer"}`)},
		)
	}

	sink := &countingGatewaySink{}

	res1, err := svc.DeliverEvents(context.Background(), payload, sink, events(), map[string]any{})
	if err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	if !res1.Delivered || sink.sends != 1 {
		t.Fatalf("first delivery delivered=%v sends=%d", res1.Delivered, sink.sends)
	}

	res2, err := svc.DeliverEvents(context.Background(), payload, sink, events(), map[string]any{})
	if err != nil {
		t.Fatalf("retry delivery: %v", err)
	}
	if !res2.Delivered {
		t.Fatalf("retry should report delivered (already sent)")
	}
	if sink.sends != 1 {
		t.Fatalf("expected no second provider send, got %d sends", sink.sends)
	}

	var count int64
	dedupe := deliveryDedupeKey(payload, "the answer")
	if err := db.Model(&model.EmployeeGatewayDelivery{}).
		Where("dedupe_key = ?", dedupe).Count(&count).Error; err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 delivery row, got %d", count)
	}
}
