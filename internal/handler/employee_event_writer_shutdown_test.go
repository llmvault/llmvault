package handler

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// During graceful shutdown the root signal context is already cancelled, so the
// final flush must run on a detached context — otherwise buffered events (the
// only durable record of a reply) are discarded.
func TestEmployeeEventWriterFlushesOnShutdownAfterRootCancel(t *testing.T) {
	db := connectEmployeeSkillSyncTestDB(t)
	org := model.Org{Name: "event-writer-shutdown-" + uuid.NewString(), RateLimit: 1000, Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Employee{OrgID: &org.ID, Name: "Aria", Model: "test", IsEmployee: true}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sandbox := model.Sandbox{OrgID: &org.ID, EmployeeID: &agent.ID, ExternalID: "event-writer-shutdown-sb", RuntimeURL: "http://localhost:7080", EncryptedRuntimeSecret: []byte("key"), Status: "running"}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	session := model.EmployeeSession{OrgID: org.ID, EmployeeID: agent.ID, SandboxID: sandbox.ID, RuntimeConversationID: "conv-shutdown", Source: "web", Status: "active"}
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

	rootCtx, cancelRoot := context.WithCancel(context.Background())
	writer := NewEmployeeEventWriter(rootCtx, db, 100, time.Hour)

	const eventCount = 3
	for i := 0; i < eventCount; i++ {
		writer.Write(rootCtx, model.EmployeeSessionEvent{
			OrgID:             org.ID,
			EmployeeID:        agent.ID,
			SandboxID:         sandbox.ID,
			EmployeeSessionID: session.ID,
			SessionID:         "conv-shutdown",
			EventType:         "agent.message.sent",
			Source:            "web",
			Mode:              "employee",
			Payload:           model.RawJSON(`{"text":"hello"}`),
			EventAt:           time.Now().UTC(),
		})
	}

	// Graceful shutdown: root ctx cancelled, then drain on a fresh context.
	cancelRoot()
	shutdownCtx, cancelShutdown := context.WithTimeout(context.WithoutCancel(rootCtx), 30*time.Second)
	defer cancelShutdown()
	writer.Shutdown(shutdownCtx)

	var count int64
	db.Model(&model.EmployeeSessionEvent{}).Where("sandbox_id = ?", sandbox.ID).Count(&count)
	if count != eventCount {
		t.Fatalf("persisted events = %d, want %d (buffer lost on shutdown)", count, eventCount)
	}
}
