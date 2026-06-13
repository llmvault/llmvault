package sandbox

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

// seedSandboxWithHistory creates a running sandbox plus a session and event that
// FK the sandbox ON DELETE CASCADE — the org history an upgrade must carry forward.
func seedSandboxWithHistory(t *testing.T, db *gorm.DB) (model.Sandbox, model.AgentSession, model.AgentSessionEvent) {
	t.Helper()
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, cred.ID)

	sb := model.Sandbox{
		OrgID:                  &org.ID,
		AgentID:                &agent.ID,
		ExternalID:             "retire-history-" + time.Now().Format("150405.000000"),
		RuntimeURL:             "http://localhost:1",
		EncryptedRuntimeSecret: []byte("test-key"),
		Status:                 string(StatusRunning),
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })

	session := model.AgentSession{
		OrgID:                 org.ID,
		AgentID:               agent.ID,
		SandboxID:             sb.ID,
		RuntimeConversationID: "conv-" + sb.ExternalID,
		Status:                "active",
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	event := model.AgentSessionEvent{
		OrgID:          org.ID,
		AgentID:        agent.ID,
		SandboxID:      sb.ID,
		AgentSessionID: session.ID,
		SessionID:      "rt-session",
		EventType:      "agent.message.sent",
		EventAt:        time.Now().UTC(),
	}
	if err := db.Create(&event).Error; err != nil {
		t.Fatalf("create session event: %v", err)
	}
	return sb, session, event
}

func countSandboxHistory(t *testing.T, db *gorm.DB, sandboxID, sessionID, eventID uuid.UUID) (int64, int64, int64) {
	t.Helper()
	var sandboxes, sessions, events int64
	db.Model(&model.Sandbox{}).Where("id = ?", sandboxID).Count(&sandboxes)
	db.Model(&model.AgentSession{}).Where("id = ?", sessionID).Count(&sessions)
	db.Model(&model.AgentSessionEvent{}).Where("id = ?", eventID).Count(&events)
	return sandboxes, sessions, events
}

// Retire-path invariant: retiring the old sandbox must release the provider
// resource WITHOUT cascade-deleting the org's history. DeleteSandboxResource
// archives the row and keeps the session/event children; DeleteSandbox wipes them.
func TestDeleteSandboxResourcePreservesHistory(t *testing.T) {
	db := setupTestDB(t)
	provider := newMockProvider()
	orch := NewOrchestrator(db, provider, testEncKey(t), &config.Config{})

	sb, session, event := seedSandboxWithHistory(t, db)

	if err := orch.DeleteSandboxResource(context.Background(), &sb); err != nil {
		t.Fatalf("DeleteSandboxResource: %v", err)
	}

	if len(provider.deletedIDs) != 1 || provider.deletedIDs[0] != sb.ExternalID {
		t.Fatalf("provider delete calls = %v, want [%s]", provider.deletedIDs, sb.ExternalID)
	}

	sandboxes, sessions, events := countSandboxHistory(t, db, sb.ID, session.ID, event.ID)
	if sandboxes != 1 || sessions != 1 || events != 1 {
		t.Fatalf("history must survive retire: sandbox=%d session=%d event=%d, want 1/1/1", sandboxes, sessions, events)
	}

	var refreshed model.Sandbox
	if err := db.Where("id = ?", sb.ID).First(&refreshed).Error; err != nil {
		t.Fatalf("reload sandbox: %v", err)
	}
	if refreshed.Status != string(StatusArchived) {
		t.Fatalf("retired sandbox status = %q, want archived", refreshed.Status)
	}
}

// TestDeleteSandboxCascadesHistory documents the contrast: the hard row delete
// DOES cascade-wipe the session/event children, which is why the retire path must
// not use it for a sandbox that still owns live org history.
func TestDeleteSandboxCascadesHistory(t *testing.T) {
	db := setupTestDB(t)
	provider := newMockProvider()
	orch := NewOrchestrator(db, provider, testEncKey(t), &config.Config{})

	sb, session, event := seedSandboxWithHistory(t, db)

	if err := orch.DeleteSandbox(context.Background(), &sb); err != nil {
		t.Fatalf("DeleteSandbox: %v", err)
	}

	sandboxes, sessions, events := countSandboxHistory(t, db, sb.ID, session.ID, event.ID)
	if sandboxes != 0 || sessions != 0 || events != 0 {
		t.Fatalf("hard delete should cascade everything: sandbox=%d session=%d event=%d, want 0/0/0", sandboxes, sessions, events)
	}
}
