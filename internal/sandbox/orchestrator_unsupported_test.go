package sandbox

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

// When the provider has no pause primitive (ErrUnsupported), the orchestrator
// must NOT flip to 'stopped' (a lie while the service bills); it returns the
// sentinel so callers skip the transition.
func TestStopSandboxUnsupportedDoesNotPersistStopped(t *testing.T) {
	db := setupTestDB(t)
	provider := newMockProvider()
	provider.stopErr = fmt.Errorf("railway stop: %w", ErrUnsupported)
	orch := NewOrchestrator(db, provider, testEncKey(t), &config.Config{})

	org := createTestOrg(t, db)
	provider.registerSandbox("ext-unsupported", StatusRunning)
	sb := model.Sandbox{
		OrgID:                  &org.ID,
		ProviderID:             provider.ID(),
		ExternalID:             "ext-unsupported",
		EncryptedRuntimeSecret: []byte("enc"),
		Status:                 string(StatusRunning),
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	err := orch.StopSandbox(context.Background(), &sb)
	if err == nil || !errors.Is(err, ErrUnsupported) {
		t.Fatalf("StopSandbox err = %v, want ErrUnsupported", err)
	}

	var reloaded model.Sandbox
	if err := db.First(&reloaded, "id = ?", sb.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Status != string(StatusRunning) {
		t.Fatalf("status = %q, want running (must not persist stopped lie)", reloaded.Status)
	}
}

// Archive on an unsupported provider must delete the live resource instead of
// persisting an 'archived' lie that keeps billing.
func TestArchiveSandboxUnsupportedDeletesResource(t *testing.T) {
	db := setupTestDB(t)
	provider := newMockProvider()
	provider.stopErr = fmt.Errorf("railway stop: %w", ErrUnsupported)
	orch := NewOrchestrator(db, provider, testEncKey(t), &config.Config{})

	org := createTestOrg(t, db)
	provider.registerSandbox("ext-archive", StatusRunning)
	sb := model.Sandbox{
		OrgID:                  &org.ID,
		ProviderID:             provider.ID(),
		ExternalID:             "ext-archive",
		EncryptedRuntimeSecret: []byte("enc"),
		Status:                 string(StatusRunning),
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	if err := orch.ArchiveSandbox(context.Background(), &sb); err != nil {
		t.Fatalf("ArchiveSandbox: %v", err)
	}
	if len(provider.deletedIDs) != 1 || provider.deletedIDs[0] != "ext-archive" {
		t.Fatalf("deleted IDs = %#v, want the live resource deleted", provider.deletedIDs)
	}
	var reloaded model.Sandbox
	if err := db.First(&reloaded, "id = ?", sb.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Status != string(StatusArchived) {
		t.Fatalf("status = %q, want archived", reloaded.Status)
	}
}
