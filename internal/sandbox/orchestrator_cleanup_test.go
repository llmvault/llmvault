package sandbox

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

// TestCreateEmployeeSandboxCleansUpOnEndpointFailure verifies P0-18: a failure
// after provider-create deletes the provider resource and revokes the minted
// proxy token instead of leaking a live billing sandbox.
func TestCreateEmployeeSandboxCleansUpOnEndpointFailure(t *testing.T) {
	db := setupTestDB(t)
	provider := newMockProvider()
	// Fail GetEndpoint so create fails after the provider resource exists.
	provider.getEndpointFn = func(_ context.Context, _ string, _ int) (string, error) {
		return "", fmt.Errorf("boom: endpoint unavailable")
	}
	orch := NewOrchestrator(db, provider, testEncKey(t), &config.Config{
		SandboxesRuntimeBaseImage: "runtime:test",
	})
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, cred.ID)

	_, err := orch.CreateEmployeeSandbox(context.Background(), &agent, employeeStartupSecrets())
	if err == nil {
		t.Fatal("expected CreateEmployeeSandbox to fail")
	}

	// The provider resource must have been deleted.
	if len(provider.deletedIDs) != 1 {
		t.Fatalf("provider delete calls = %d, want 1 (resource must be released)", len(provider.deletedIDs))
	}

	// The sandbox row must be marked error with the external id recorded.
	var sb model.Sandbox
	if err := db.Where("employee_id = ?", agent.ID).First(&sb).Error; err != nil {
		t.Fatalf("load sandbox row: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })
	if sb.Status != string(StatusError) {
		t.Fatalf("sandbox status = %q, want error", sb.Status)
	}
	if sb.ExternalID == "" {
		t.Fatal("sandbox external_id should be recorded for the reaper")
	}
}

// TestCleanupFailedSandboxRevokesProxyTokens verifies the shared cleanup helper
// revokes proxy tokens tagged to the sandbox.
func TestCleanupFailedSandboxRevokesProxyTokens(t *testing.T) {
	db := setupTestDB(t)
	provider := newMockProvider()
	orch := NewOrchestrator(db, provider, testEncKey(t), &config.Config{})
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)

	sb := model.Sandbox{
		OrgID:                  &org.ID,
		ProviderID:             provider.ID(),
		ExternalID:             "mock-ext-1",
		RuntimeURL:             "https://x.test",
		EncryptedRuntimeSecret: []byte("enc"),
		Status:                 "running",
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })
	provider.registerSandbox("mock-ext-1", StatusRunning)

	tok := model.Token{
		OrgID:        org.ID,
		CredentialID: cred.ID,
		JTI:          uuid.NewString(),
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		Meta:         model.JSON{model.TokenMetaSandboxID: sb.ID.String()},
	}
	if err := db.Create(&tok).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", tok.ID).Delete(&model.Token{}) })

	orch.cleanupFailedSandbox(context.Background(), &sb, "mock-ext-1", "boom")

	var reloaded model.Token
	if err := db.Where("id = ?", tok.ID).First(&reloaded).Error; err != nil {
		t.Fatalf("reload token: %v", err)
	}
	if reloaded.RevokedAt == nil {
		t.Fatal("proxy token should have been revoked")
	}
	if len(provider.deletedIDs) != 1 {
		t.Fatalf("provider delete calls = %d, want 1", len(provider.deletedIDs))
	}
}

// TestReapStuckSandboxes verifies the reaper releases sandboxes stuck in
// 'creating' beyond the TTL (P0-18 backstop).
func TestReapStuckSandboxes(t *testing.T) {
	db := setupTestDB(t)
	provider := newMockProvider()
	orch := NewOrchestrator(db, provider, testEncKey(t), &config.Config{})
	org := createTestOrg(t, db)
	provider.registerSandbox("stuck-ext-1", StatusRunning)

	sb := model.Sandbox{
		OrgID:                  &org.ID,
		ProviderID:             provider.ID(),
		ExternalID:             "stuck-ext-1",
		RuntimeURL:             "https://x.test",
		EncryptedRuntimeSecret: []byte("enc"),
		Status:                 "creating",
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })
	if err := db.Model(&model.Sandbox{}).Where("id = ?", sb.ID).
		Update("updated_at", time.Now().Add(-time.Hour)).Error; err != nil {
		t.Fatalf("backdate sandbox: %v", err)
	}

	orch.ReapStuckSandboxes(context.Background())

	if len(provider.deletedIDs) != 1 || provider.deletedIDs[0] != "stuck-ext-1" {
		t.Fatalf("provider delete calls = %v, want [stuck-ext-1]", provider.deletedIDs)
	}
	var reloaded model.Sandbox
	if err := db.First(&reloaded, "id = ?", sb.ID).Error; err != nil {
		t.Fatalf("reload sandbox: %v", err)
	}
	if reloaded.Status != string(StatusArchived) {
		t.Fatalf("sandbox status = %q, want archived", reloaded.Status)
	}
}

// TestReapIdleSpecialistSandboxes verifies P0-20: a specialist sandbox whose
// task is idle beyond the TTL has its provider resource released even though the
// LLM never called terminate.
func TestReapIdleSpecialistSandboxes(t *testing.T) {
	db := setupTestDB(t)
	provider := newMockProvider()
	orch := NewOrchestrator(db, provider, testEncKey(t), &config.Config{})
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, cred.ID)
	provider.registerSandbox("spec-ext-1", StatusRunning)

	sb := model.Sandbox{
		OrgID:                  &org.ID,
		EmployeeID:             &agent.ID,
		ProviderID:             provider.ID(),
		ExternalID:             "spec-ext-1",
		RuntimeURL:             "https://x.test",
		EncryptedRuntimeSecret: []byte("enc"),
		Status:                 "running",
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })

	task := model.SpecialistTask{
		OrgID:                  org.ID,
		EmployeeID:             agent.ID,
		SpecialistSlug:         "researcher",
		EmployeeSessionID:      "sess-1",
		SandboxID:              sb.ID,
		ParentConversationType: "employee_session",
		ParentConversationID:   "sess-1",
		Brief:                  "do a thing",
		Status:                 "idle",
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("create specialist task: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", task.ID).Delete(&model.SpecialistTask{}) })
	if err := db.Model(&model.SpecialistTask{}).Where("id = ?", task.ID).
		Update("updated_at", time.Now().Add(-2*time.Hour)).Error; err != nil {
		t.Fatalf("backdate task: %v", err)
	}

	orch.ReapStuckSandboxes(context.Background())

	if len(provider.deletedIDs) != 1 || provider.deletedIDs[0] != "spec-ext-1" {
		t.Fatalf("provider delete calls = %v, want [spec-ext-1]", provider.deletedIDs)
	}
	var reloaded model.Sandbox
	if err := db.First(&reloaded, "id = ?", sb.ID).Error; err != nil {
		t.Fatalf("reload sandbox: %v", err)
	}
	if reloaded.Status != string(StatusArchived) {
		t.Fatalf("sandbox status = %q, want archived", reloaded.Status)
	}
}
