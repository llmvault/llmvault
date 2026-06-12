package sandbox

import (
	"context"
	"testing"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

// A single transient bad observation does not persist 'error'; N consecutive do.
func TestHealthCheckRequiresConsecutiveBadObservations(t *testing.T) {
	db := setupTestDB(t)
	provider := newMockProvider()
	orch := NewOrchestrator(db, provider, testEncKey(t), &config.Config{})
	org := createTestOrg(t, db)

	sb := model.Sandbox{
		OrgID:                  &org.ID,
		ProviderID:             provider.ID(),
		ExternalID:             "health-ext-1",
		RuntimeURL:             "https://x.test",
		EncryptedRuntimeSecret: []byte("enc"),
		Status:                 "running",
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })

	badStatus := StatusRunning
	provider.getStatusFn = func(_ context.Context, _ string) (SandboxStatus, error) {
		return badStatus, nil
	}

	statusOf := func() string {
		var r model.Sandbox
		if err := db.First(&r, "id = ?", sb.ID).Error; err != nil {
			t.Fatalf("reload: %v", err)
		}
		return r.Status
	}

	badStatus = StatusError
	orch.RunHealthCheck(context.Background())
	if got := statusOf(); got != "running" {
		t.Fatalf("after 1 bad observation status = %q, want running", got)
	}
	orch.RunHealthCheck(context.Background())
	if got := statusOf(); got != "running" {
		t.Fatalf("after 2 bad observations status = %q, want running", got)
	}
	orch.RunHealthCheck(context.Background())
	if got := statusOf(); got != string(StatusError) {
		t.Fatalf("after 3 bad observations status = %q, want error", got)
	}
}

// A recovery before the threshold resets the counter so the sandbox stays running.
func TestHealthCheckTransientBadThenRecoverDoesNotBrick(t *testing.T) {
	db := setupTestDB(t)
	provider := newMockProvider()
	orch := NewOrchestrator(db, provider, testEncKey(t), &config.Config{})
	org := createTestOrg(t, db)

	sb := model.Sandbox{
		OrgID:                  &org.ID,
		ProviderID:             provider.ID(),
		ExternalID:             "health-ext-2",
		RuntimeURL:             "https://x.test",
		EncryptedRuntimeSecret: []byte("enc"),
		Status:                 "running",
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })

	cur := StatusError
	provider.getStatusFn = func(_ context.Context, _ string) (SandboxStatus, error) {
		return cur, nil
	}
	orch.RunHealthCheck(context.Background()) // 1 bad
	orch.RunHealthCheck(context.Background()) // 2 bad
	cur = StatusRunning
	orch.RunHealthCheck(context.Background()) // recovered -> counter reset
	cur = StatusError
	orch.RunHealthCheck(context.Background()) // 1 bad again

	var r model.Sandbox
	if err := db.First(&r, "id = ?", sb.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if r.Status != "running" {
		t.Fatalf("status = %q, want running (counter must reset on recovery)", r.Status)
	}
}

// An error-state row that the provider now reports as running is recovered.
func TestHealthCheckReprobesErrorRows(t *testing.T) {
	db := setupTestDB(t)
	provider := newMockProvider()
	orch := NewOrchestrator(db, provider, testEncKey(t), &config.Config{})
	org := createTestOrg(t, db)
	provider.registerSandbox("recover-ext-1", StatusRunning)

	sb := model.Sandbox{
		OrgID:                  &org.ID,
		ProviderID:             provider.ID(),
		ExternalID:             "recover-ext-1",
		RuntimeURL:             "https://x.test",
		EncryptedRuntimeSecret: []byte("enc"),
		Status:                 "error",
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })

	orch.RunHealthCheck(context.Background())

	var r model.Sandbox
	if err := db.First(&r, "id = ?", sb.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if r.Status != "running" {
		t.Fatalf("status = %q, want running (error row should re-probe to running)", r.Status)
	}
}
