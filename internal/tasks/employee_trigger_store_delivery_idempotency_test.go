package tasks

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/usehivy/hivy/internal/model"
)

// TestStoreDelivery_IsIdempotentUnderRetry verifies P2-45: the store-delivery
// task upserts onto the unique (trigger_id, delivery_id) row, so an asynq retry
// of the same task does not insert a duplicate delivery row and instead refreshes
// the runtime correlation fields on the existing row.
func TestStoreDelivery_IsIdempotentUnderRetry(t *testing.T) {
	db := openTasksMemoryTestDB(t)
	orgID := uuid.New()
	if err := db.Create(&model.Org{ID: orgID, Name: "store-idem-" + uuid.NewString()[:8], Active: true}).Error; err != nil {
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
		RuntimeConversationID: "store-conv-idem",
		Source:                triggerConversationSource,
		Status:                "active",
	}
	if err := db.Create(&conv).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	h := NewEmployeeTriggerStoreDeliveryHandler(db)
	payload := EmployeeTriggerStoreDeliveryPayload{
		OrgID:            orgID,
		EmployeeID:       agent.ID,
		TriggerID:        trigger.ID,
		DeliveryID:       "store-delivery-idem-1",
		EventKey:         "issues/opened",
		ConversationID:   conv.ID,
		RuntimeSessionID: "session-v1",
		PayloadJSON:      []byte(`{"issue":{"number":9}}`),
	}
	task := newStoreDeliveryTask(t, payload)

	if err := h.Handle(t.Context(), task); err != nil {
		t.Fatalf("first store: %v", err)
	}

	// Retry the same delivery with an updated runtime session id (as a later
	// dispatch would carry). It must upsert, not insert a duplicate.
	payload.RuntimeSessionID = "session-v2"
	if err := h.Handle(t.Context(), newStoreDeliveryTask(t, payload)); err != nil {
		t.Fatalf("retried store: %v", err)
	}

	var rows []model.EmployeeTriggerDelivery
	if err := db.Where("trigger_id = ? AND delivery_id = ?", trigger.ID, payload.DeliveryID).Find(&rows).Error; err != nil {
		t.Fatalf("load deliveries: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected exactly one delivery row, got %d", len(rows))
	}
	if rows[0].RuntimeSessionID != "session-v2" {
		t.Fatalf("runtime_session_id = %q, want session-v2 (upsert should refresh it)", rows[0].RuntimeSessionID)
	}
}

func newStoreDeliveryTask(t *testing.T, payload EmployeeTriggerStoreDeliveryPayload) *asynq.Task {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return asynq.NewTask(TypeEmployeeTriggerStoreDelivery, encoded)
}
