package handler

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// A transient Postgres error on a batch flush must not silently drop the events. The drain retries
// with backoff, so the events are eventually persisted.
func TestAgentEventWriterRetriesTransientFlushFailure(t *testing.T) {
	db := connectAgentSkillSyncTestDB(t)
	org := model.Org{Name: "event-writer-retry-" + uuid.NewString(), RateLimit: 1000, Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Agent{OrgID: &org.ID, Name: "Aria", Model: "test", IsManaged: true}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sandbox := model.Sandbox{OrgID: &org.ID, AgentID: &agent.ID, ExternalID: "event-writer-retry-sb", RuntimeURL: "http://localhost:7080", EncryptedRuntimeSecret: []byte("key"), Status: "running"}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	session := model.AgentSession{OrgID: org.ID, AgentID: agent.ID, SandboxID: sandbox.ID, RuntimeConversationID: "conv-retry", Source: "web", Status: "active"}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	t.Cleanup(func() {
		db.Where("sandbox_id = ?", sandbox.ID).Delete(&model.AgentSessionEvent{})
		db.Where("id = ?", session.ID).Delete(&model.AgentSession{})
		db.Where("id = ?", sandbox.ID).Delete(&model.Sandbox{})
		db.Where("id = ?", agent.ID).Delete(&model.Agent{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})

	// Register a create callback that fails the first batch insert of session
	// events, then disarms itself — simulating a transient DB blip.
	var fail int32 = 1
	cbName := "test:fail_first_session_event_create"
	err := db.Callback().Create().Before("gorm:create").Register(cbName, func(tx *gorm.DB) {
		if _, ok := tx.Statement.Dest.([]model.AgentSessionEvent); !ok {
			if _, ok := tx.Statement.Dest.(*[]model.AgentSessionEvent); !ok {
				return
			}
		}
		if atomic.CompareAndSwapInt32(&fail, 1, 0) {
			_ = tx.AddError(fmt.Errorf("simulated transient db error"))
		}
	})
	if err != nil {
		t.Fatalf("register callback: %v", err)
	}
	t.Cleanup(func() { _ = db.Callback().Create().Remove(cbName) })

	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	writer := NewAgentEventWriter(rootCtx, db, 100, 50*time.Millisecond)

	writer.Write(rootCtx, model.AgentSessionEvent{
		OrgID:          org.ID,
		AgentID:        agent.ID,
		SandboxID:      sandbox.ID,
		AgentSessionID: session.ID,
		SessionID:      "conv-retry",
		EventType:      "agent.message.sent",
		Source:         "web",
		Mode:           "agent",
		Payload:        model.RawJSON(`{"text":"retry me"}`),
		EventAt:        time.Now().UTC(),
	})

	shutdownCtx, cancelShutdown := context.WithTimeout(context.WithoutCancel(rootCtx), 30*time.Second)
	defer cancelShutdown()
	writer.Shutdown(shutdownCtx)

	var count int64
	db.Model(&model.AgentSessionEvent{}).Where("sandbox_id = ?", sandbox.ID).Count(&count)
	if count != 1 {
		t.Fatalf("persisted events = %d, want 1 (event dropped on transient error)", count)
	}
}
