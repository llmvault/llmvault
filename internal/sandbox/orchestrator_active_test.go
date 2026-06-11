package sandbox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

// A 'creating' row with no runtime_url must not probe a bare "/healthz" for 90s
// and blind-write 'running': if the row flips to 'error' while waiting for the
// URL, EnsureSandboxActive must return promptly with an error.
func TestEnsureSandboxActiveCreatingFailsFastOnEmptyURL(t *testing.T) {
	db := setupTestDB(t)
	provider := newMockProvider()
	orch := NewOrchestrator(db, provider, testEncKey(t), &config.Config{})

	org := createTestOrg(t, db)
	sb := model.Sandbox{
		OrgID:                  &org.ID,
		ProviderID:             provider.ID(),
		ExternalID:             "creating-no-url",
		EncryptedRuntimeSecret: []byte("enc"),
		RuntimeURL:             "", // not yet populated by the in-flight creator
		Status:                 string(StatusCreating),
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	// Simulate the in-flight creator failing shortly after.
	go func() {
		time.Sleep(50 * time.Millisecond)
		db.Model(&model.Sandbox{}).Where("id = ?", sb.ID).Update("status", string(StatusError))
	}()

	start := time.Now()
	_, err := orch.EnsureSandboxActive(context.Background(), &sb)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected error when runtime url never populates and row errors")
	}
	if elapsed > 30*time.Second {
		t.Fatalf("EnsureSandboxActive took %s; should fail fast, not probe for the full health timeout", elapsed)
	}
	if !strings.Contains(err.Error(), "error state") {
		t.Fatalf("err = %v, want error-state failure", err)
	}

	var reloaded model.Sandbox
	if err := db.First(&reloaded, "id = ?", sb.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Status == string(StatusRunning) {
		t.Fatalf("status = running; must not blind-write running on a failed in-flight create")
	}
}

// Once the in-flight creator writes runtime_url and the runtime is live, the
// guarded flip moves the row to 'running'.
func TestEnsureSandboxActiveCreatingActivatesWhenURLPopulates(t *testing.T) {
	db := setupTestDB(t)
	provider := newMockProvider()
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(health.Close)
	orch := NewOrchestrator(db, provider, testEncKey(t), &config.Config{})

	org := createTestOrg(t, db)
	sb := model.Sandbox{
		OrgID:                  &org.ID,
		ProviderID:             provider.ID(),
		ExternalID:             "creating-url-later",
		EncryptedRuntimeSecret: []byte("enc"),
		RuntimeURL:             "",
		Status:                 string(StatusCreating),
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		db.Model(&model.Sandbox{}).Where("id = ?", sb.ID).Update("runtime_url", health.URL)
	}()

	got, err := orch.EnsureSandboxActive(context.Background(), &sb)
	if err != nil {
		t.Fatalf("EnsureSandboxActive: %v", err)
	}
	if got.Status != string(StatusRunning) {
		t.Fatalf("status = %q, want running", got.Status)
	}
	var reloaded model.Sandbox
	if err := db.First(&reloaded, "id = ?", sb.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Status != string(StatusRunning) {
		t.Fatalf("persisted status = %q, want running", reloaded.Status)
	}
}

// From the orchestrator side: a sandbox parked in 'upgrading' is mid-upgrade and
// must not be routed to / flipped to running by EnsureSandboxActive.
func TestEnsureSandboxActiveUpgradingErrors(t *testing.T) {
	db := setupTestDB(t)
	provider := newMockProvider()
	orch := NewOrchestrator(db, provider, testEncKey(t), &config.Config{})

	org := createTestOrg(t, db)
	sb := model.Sandbox{
		OrgID:                  &org.ID,
		ProviderID:             provider.ID(),
		ExternalID:             "upgrading-ext",
		EncryptedRuntimeSecret: []byte("enc"),
		Status:                 string(StatusUpgrading),
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	if _, err := orch.EnsureSandboxActive(context.Background(), &sb); err == nil {
		t.Fatal("expected error for mid-upgrade sandbox")
	}
}
