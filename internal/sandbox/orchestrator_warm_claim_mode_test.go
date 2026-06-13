package sandbox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

// claimWarmRuntimeForMode runs the full claimWarmRuntime path for the given mode,
// recording whether the agent-config pusher callback fired.
func claimWarmRuntimeForMode(t *testing.T, mode string) (pushed bool) {
	t.Helper()
	db := setupTestDB(t)
	provider := newMockProvider()
	if err := db.Where("provider_id = ?", provider.ID()).Delete(&model.SandboxWarmSlot{}).Error; err != nil {
		t.Fatalf("clean warm slots: %v", err)
	}
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Satisfy both the orchestrator health wait (/healthz) and warm-slot checks.
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(health.Close)
	provider.warmEndpoint = health.URL

	cfg := &config.Config{
		SandboxWarmPoolAgentSize:  1,
		RailwayRuntimePort:        7080,
		SandboxesRuntimeBaseImage: "runtime:test",
	}
	pool := NewWarmPool(db, provider, testEncKey(t), cfg)
	if _, err := pool.Reconcile(context.Background(), mode, nil); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	created, err := pool.Reconcile(context.Background(), mode, nil)
	_ = created
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	var slots []model.SandboxWarmSlot
	if err := db.Where("provider_id = ? AND mode = ?", provider.ID(), mode).Find(&slots).Error; err != nil {
		t.Fatalf("load slots: %v", err)
	}
	if len(slots) == 0 {
		t.Fatalf("no warm slots provisioned for mode %s", mode)
	}
	for _, slot := range slots {
		if _, err := pool.CheckWarmSlot(context.Background(), slot.ID); err != nil {
			t.Fatalf("check warm slot: %v", err)
		}
	}

	orch := NewOrchestrator(db, provider, testEncKey(t), cfg)
	orch.warmPool = pool

	var mu sync.Mutex
	orch.SetAgentRuntimeConfigPusher(func(ctx context.Context, sb *model.Sandbox) error {
		mu.Lock()
		pushed = true
		mu.Unlock()
		return nil
	})

	org := createTestOrg(t, db)
	sb := model.Sandbox{
		OrgID:                  &org.ID,
		ProviderID:             provider.ID(),
		ExternalID:             "pending",
		RuntimeURL:             "pending",
		EncryptedRuntimeSecret: []byte("encrypted"),
		Status:                 "creating",
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })

	if err := orch.claimWarmRuntime(context.Background(), &sb, mode); err != nil {
		t.Fatalf("claimWarmRuntime(%s): %v", mode, err)
	}
	mu.Lock()
	defer mu.Unlock()
	return pushed
}

func TestClaimWarmRuntime_PushesAgentConfig(t *testing.T) {
	if pushed := claimWarmRuntimeForMode(t, model.SandboxWarmSlotModeAgent); !pushed {
		t.Fatal("agent warm claim must push the agent runtime config")
	}
}
