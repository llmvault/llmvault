package tasks

import (
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// TestClaimTriggerDelivery_IsIdempotent asserts that claiming the same
// (trigger_id, delivery_id) twice only wins the claim once. asynq retries the
// whole dispatch batch on failure, so without this the agent re-receives already
// delivered triggers (P0-14).
func TestClaimTriggerDelivery_IsIdempotent(t *testing.T) {
	db := openTasksMemoryTestDB(t)
	orgID := uuid.New()
	if err := db.Create(&model.Org{ID: orgID, Name: "trigger-idem-" + uuid.NewString()[:8], Active: true}).Error; err != nil {
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
	trigger := model.EmployeeTrigger{ID: uuid.New(), OrgID: orgID, EmployeeID: agent.ID, TriggerType: "webhook", Enabled: true}
	if err := db.Create(&trigger).Error; err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	conv := model.EmployeeSession{
		ID:                    uuid.New(),
		OrgID:                 orgID,
		EmployeeID:            agent.ID,
		SandboxID:             sb.ID,
		RuntimeConversationID: "trigger-conv-idem",
		Source:                triggerConversationSource,
		SourceResourceKey:     "github/usehivy/hivy/issue/7",
		Status:                "active",
	}
	if err := db.Create(&conv).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	h := &EmployeeTriggerDispatchHandler{db: db}
	payload := EmployeeTriggerDispatchPayload{
		OrgID:       orgID,
		EventType:   "issues",
		EventAction: "opened",
		DeliveryID:  "delivery-idem-1",
		PayloadJSON: []byte(`{"issue":{"number":7}}`),
	}
	compiled := compiledTriggerMessage{ResourceKey: conv.SourceResourceKey}

	first, err := h.claimTriggerDelivery(t.Context(), payload, trigger, &conv, compiled)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if !first {
		t.Fatalf("first claim should win")
	}

	second, err := h.claimTriggerDelivery(t.Context(), payload, trigger, &conv, compiled)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if second {
		t.Fatalf("second claim must be skipped (already delivered)")
	}

	var rows int64
	db.Model(&model.EmployeeTriggerDelivery{}).
		Where("trigger_id = ? AND delivery_id = ?", trigger.ID, payload.DeliveryID).
		Count(&rows)
	if rows != 1 {
		t.Fatalf("expected exactly one delivery row, got %d", rows)
	}

	// Releasing the claim (post failed before delivery) allows a re-claim.
	h.releaseTriggerDeliveryClaim(t.Context(), trigger.ID, payload.DeliveryID)
	reclaimed, err := h.claimTriggerDelivery(t.Context(), payload, trigger, &conv, compiled)
	if err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	if !reclaimed {
		t.Fatalf("claim should win again after release")
	}
}
