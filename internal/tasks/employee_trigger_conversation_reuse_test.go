package tasks

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
)

func TestEmployeeTriggerConversationReusesResourceAcrossTriggerIDs(t *testing.T) {
	db := openTasksMemoryTestDB(t)
	orgID := uuid.New()
	agent := model.Employee{ID: uuid.New(), OrgID: &orgID, Model: "test-model", Status: "active"}
	sb := model.Sandbox{ID: uuid.New(), OrgID: &orgID, EmployeeID: &agent.ID, EncryptedRuntimeSecret: []byte("test-secret"), Status: "running"}
	if err := db.Create(&model.Org{ID: orgID, Name: "trigger-reuse-" + uuid.NewString()[:8], Active: true}).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create employee: %v", err)
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	enq := &enqueue.MockClient{}
	handler := &EmployeeTriggerDispatchHandler{db: db, enqueuer: enq}
	resourceKey := "github/usehivy/hivy/pull/42"
	first, err := handler.findOrCreateTriggerConversation(t.Context(), &agent, &sb, uuid.New(), resourceKey, "mention-conv")
	if err != nil {
		t.Fatalf("create first trigger conversation: %v", err)
	}
	second, err := handler.findOrCreateTriggerConversation(t.Context(), &agent, &sb, uuid.New(), resourceKey, "workflow-conv")
	if err != nil {
		t.Fatalf("reuse trigger conversation: %v", err)
	}

	if first.ID != second.ID {
		t.Fatalf("conversation ids differ: first=%s second=%s", first.ID, second.ID)
	}
	if second.RuntimeConversationID != "mention-conv" {
		t.Fatalf("runtime conversation id = %q, want first conversation id", second.RuntimeConversationID)
	}

	enqueued := enq.Tasks()
	if len(enqueued) != 1 || enqueued[0].TypeName != TypeEmployeeMemoryRetain {
		t.Fatalf("memory retain tasks = %#v, want one %s task", enqueued, TypeEmployeeMemoryRetain)
	}
	var payload EmployeeMemoryRetainPayload
	if err := json.Unmarshal(enqueued[0].Payload, &payload); err != nil {
		t.Fatalf("decode retain payload: %v", err)
	}
	if payload.EmployeeSessionID != first.ID || payload.SessionID != first.RuntimeConversationID {
		t.Fatalf("retain payload mismatch: %#v first=%#v", payload, first)
	}
}
