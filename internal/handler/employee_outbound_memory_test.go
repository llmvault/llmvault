package handler

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

func TestEmployeeOutboundAgentMessageSentSchedulesRetainForExistingSession(t *testing.T) {
	db := connectEmployeeSkillSyncTestDB(t)
	org := model.Org{Name: "outbound-retain-" + uuid.NewString(), RateLimit: 1000, Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Employee{OrgID: &org.ID, Name: "Aria", Model: "test", IsEmployee: true}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create employee: %v", err)
	}
	sandbox := model.Sandbox{OrgID: &org.ID, EmployeeID: &agent.ID, EncryptedRuntimeSecret: []byte("secret"), Status: "running"}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	session := model.EmployeeSession{
		OrgID:                 org.ID,
		EmployeeID:            agent.ID,
		SandboxID:             sandbox.ID,
		RuntimeConversationID: "http-gateway-existing",
		Source:                "gateway",
		Status:                "active",
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}

	enq := &enqueue.MockClient{}
	h := NewEmployeeOutboundWebhookHandler(db, nil, enq)
	h.storeAndMaybeEnqueue(t.Context(), &sandbox, &employeeOutboundEvent{
		EventType: "agent.message.sent",
		Payload: mustJSON(t, map[string]any{
			"session_id": "http-gateway-existing",
			"source":     "gateway",
			"provider":   "slack",
			"text":       "Done.",
		}),
		At: time.Now().UTC(),
	})

	enqueued := enq.Tasks()
	if len(enqueued) != 1 || enqueued[0].TypeName != tasks.TypeEmployeeMemoryRetain {
		t.Fatalf("memory retain tasks = %#v, want one %s task", enqueued, tasks.TypeEmployeeMemoryRetain)
	}
	var payload tasks.EmployeeMemoryRetainPayload
	if err := json.Unmarshal(enqueued[0].Payload, &payload); err != nil {
		t.Fatalf("decode retain payload: %v", err)
	}
	if payload.EmployeeSessionID != session.ID || payload.SourceEvent != "agent.message.sent" {
		t.Fatalf("retain payload mismatch: %#v", payload)
	}
}
