package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// TestEmployeeSessionEventRedeliveryIsIdempotent guards the ack-means-durable
// contract for session events (P0-15): the runtime outbox redelivers an event
// whenever it does not observe a clean 2xx ack — including when Go committed the
// row but the ack response was lost in transit, or when one event in a batch
// failed and the whole batch is redelivered. A redelivery of the same
// (sandbox_id, session_id, sequence) event must NOT create a second
// session-event row (which would duplicate the agent's reply in the persisted
// timeline and re-drive memory retain / gateway delivery).
func TestEmployeeSessionEventRedeliveryIsIdempotent(t *testing.T) {
	db := connectEmployeeSkillSyncTestDB(t)
	encKey := outboundWebhookTestSymmetricKey(t)
	org := model.Org{Name: "session-event-redeliver-" + uuid.NewString(), RateLimit: 1000, Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Employee{OrgID: &org.ID, Name: "Aria", Model: "test", IsEmployee: true}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	runtimeSecret := "redeliver-secret-" + uuid.NewString()
	encryptedKey, err := encKey.EncryptString(runtimeSecret)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	sandbox := model.Sandbox{OrgID: &org.ID, EmployeeID: &agent.ID, ExternalID: "session-event-redeliver-sb", RuntimeURL: "http://localhost:7080", EncryptedRuntimeSecret: encryptedKey, Status: "running"}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	t.Cleanup(func() {
		db.Where("sandbox_id = ?", sandbox.ID).Delete(&model.EmployeeSessionEvent{})
		db.Where("org_id = ?", org.ID).Delete(&model.EmployeeSession{})
		db.Where("id = ?", sandbox.ID).Delete(&model.Sandbox{})
		db.Where("id = ?", agent.ID).Delete(&model.Employee{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})

	// Synchronous writer path: the ack depends on the durable write, so a redeliver
	// must be deduped against the already-stored row.
	h := NewEmployeeOutboundWebhookHandler(db, encKey, nil)
	r := chi.NewRouter()
	r.Post("/internal/webhooks/employee/{sandboxID}", h.Handle)

	payload := map[string]any{
		"session_id": "conv-redeliver-1",
		"source":     "web",
		"event_id":   "evt-redeliver-1",
		"sequence":   float64(7),
		"text":       "Here is your answer.",
	}
	deliver := func() int {
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		envelope, err := json.Marshal(employeeOutboundEvent{EventType: "agent.message.sent", Payload: body, At: time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)})
		if err != nil {
			t.Fatalf("marshal envelope: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "/internal/webhooks/employee/"+sandbox.ID.String(), bytes.NewReader(envelope))
		req.Header.Set("X-Hivy-Signature", "sha256="+hmacHex(runtimeSecret, envelope))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := deliver(); code != http.StatusOK {
		t.Fatalf("first delivery status = %d, want 200", code)
	}
	// Ack lost in transit / batch redelivered: the runtime re-POSTs the same event.
	if code := deliver(); code != http.StatusOK {
		t.Fatalf("redelivery status = %d, want 200 (idempotent ack)", code)
	}

	var count int64
	db.Model(&model.EmployeeSessionEvent{}).
		Where("sandbox_id = ? AND runtime_session_id = ? AND event_type = ?", sandbox.ID, "conv-redeliver-1", "agent.message.sent").
		Count(&count)
	if count != 1 {
		t.Fatalf("session event rows = %d, want 1 (redelivery duplicated the reply)", count)
	}
}

// TestEmployeeEventWriterRedeliveryIsIdempotent guards the same contract on the
// buffered writer path used in production (cmd/server/serve.go). The HTTP ack is
// returned immediately, but a redelivery of the same event must still not produce
// a duplicate row once both have been flushed.
func TestEmployeeEventWriterRedeliveryIsIdempotent(t *testing.T) {
	db := connectEmployeeSkillSyncTestDB(t)
	org := model.Org{Name: "writer-redeliver-" + uuid.NewString(), RateLimit: 1000, Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Employee{OrgID: &org.ID, Name: "Aria", Model: "test", IsEmployee: true}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sandbox := model.Sandbox{OrgID: &org.ID, EmployeeID: &agent.ID, ExternalID: "writer-redeliver-sb", RuntimeURL: "http://localhost:7080", EncryptedRuntimeSecret: []byte("key"), Status: "running"}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	session := model.EmployeeSession{OrgID: org.ID, EmployeeID: agent.ID, SandboxID: sandbox.ID, RuntimeConversationID: "conv-writer-redeliver", Source: "web", Status: "active"}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	t.Cleanup(func() {
		db.Where("sandbox_id = ?", sandbox.ID).Delete(&model.EmployeeSessionEvent{})
		db.Where("id = ?", session.ID).Delete(&model.EmployeeSession{})
		db.Where("id = ?", sandbox.ID).Delete(&model.Sandbox{})
		db.Where("id = ?", agent.ID).Delete(&model.Employee{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})

	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	writer := NewEmployeeEventWriter(rootCtx, db, 100, 25*time.Millisecond)

	event := func() model.EmployeeSessionEvent {
		return model.EmployeeSessionEvent{
			OrgID:             org.ID,
			EmployeeID:        agent.ID,
			SandboxID:         sandbox.ID,
			EmployeeSessionID: session.ID,
			SessionID:         "conv-writer-redeliver",
			EventID:           "evt-writer-redeliver-1",
			EventType:         "agent.message.sent",
			Source:            "web",
			Mode:              "employee",
			SequenceNumber:    9,
			Payload:           model.RawJSON(`{"text":"reply"}`),
			EventAt:           time.Now().UTC(),
		}
	}
	writer.Write(rootCtx, event())
	writer.Write(rootCtx, event())

	shutdownCtx, cancelShutdown := context.WithTimeout(context.WithoutCancel(rootCtx), 30*time.Second)
	defer cancelShutdown()
	writer.Shutdown(shutdownCtx)

	var count int64
	db.Model(&model.EmployeeSessionEvent{}).
		Where("sandbox_id = ? AND runtime_session_id = ?", sandbox.ID, "conv-writer-redeliver").
		Count(&count)
	if count != 1 {
		t.Fatalf("session event rows = %d, want 1 (writer redelivery duplicated the event)", count)
	}
}
