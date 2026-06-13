package sandbox

import (
	"context"
	"testing"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

// Scale-down: when more warm slots exist than desired, Reconcile deletes the
// surplus provider resources rather than leaving them running forever.
func TestWarmPoolReconcileTrimsSurplusSlots(t *testing.T) {
	db := setupTestDB(t)
	provider := newMockProvider()
	db.Where("provider_id = ?", provider.ID()).Delete(&model.SandboxWarmSlot{})
	encKey := testEncKey(t)
	secret, err := encKey.EncryptString("warm-secret")
	if err != nil {
		t.Fatalf("encrypt warm secret: %v", err)
	}

	const image = "runtime:test"
	for i := 0; i < 3; i++ {
		ext := "warm-surplus-" + string(rune('a'+i))
		provider.registerSandbox(ext, StatusRunning)
		slot := model.SandboxWarmSlot{
			ProviderID:             provider.ID(),
			Mode:                   model.SandboxWarmSlotModeAgent,
			Status:                 model.SandboxWarmSlotStatusWarm,
			ExternalID:             ext,
			EndpointURL:            "https://surplus.example",
			RuntimeImage:           image,
			RuntimePort:            7080,
			EncryptedRuntimeSecret: secret,
		}
		if err := db.Create(&slot).Error; err != nil {
			t.Fatalf("create warm slot: %v", err)
		}
	}

	pool := NewWarmPool(db, provider, encKey, &config.Config{
		SandboxWarmPoolAgentSize:  1,
		RailwayRuntimePort:        7080,
		SandboxesRuntimeBaseImage: image,
	})
	created, err := pool.Reconcile(context.Background(), model.SandboxWarmSlotModeAgent, nil)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(created) != 0 {
		t.Fatalf("created = %d, want 0 (pool already over desired)", len(created))
	}
	if len(provider.warmCreateCalls) != 0 {
		t.Fatalf("warm create calls = %d, want 0", len(provider.warmCreateCalls))
	}
	if len(provider.deletedIDs) != 2 {
		t.Fatalf("deleted provider resources = %d, want 2 surplus trimmed", len(provider.deletedIDs))
	}

	var warm int64
	if err := db.Model(&model.SandboxWarmSlot{}).
		Where("provider_id = ? AND mode = ? AND status = ?", provider.ID(), model.SandboxWarmSlotModeAgent, model.SandboxWarmSlotStatusWarm).
		Count(&warm).Error; err != nil {
		t.Fatalf("count warm: %v", err)
	}
	if warm != 1 {
		t.Fatalf("remaining warm slots = %d, want 1 (desired)", warm)
	}
}

// The Reconcile advisory lock must prevent two concurrent reconciles from both
// reading available=0 and each provisioning, over-provisioning past desired.
func TestWarmPoolReconcileSerializesConcurrentCalls(t *testing.T) {
	db := setupTestDB(t)
	provider := newMockProvider()
	db.Where("provider_id = ?", provider.ID()).Delete(&model.SandboxWarmSlot{})

	pool := NewWarmPool(db, provider, testEncKey(t), &config.Config{
		SandboxWarmPoolAgentSize:  2,
		RailwayRuntimePort:        7080,
		SandboxesRuntimeBaseImage: "runtime:test",
	})

	const goroutines = 4
	errCh := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			_, err := pool.Reconcile(context.Background(), model.SandboxWarmSlotModeAgent, nil)
			errCh <- err
		}()
	}
	for i := 0; i < goroutines; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("reconcile: %v", err)
		}
	}

	var total int64
	if err := db.Model(&model.SandboxWarmSlot{}).
		Where("provider_id = ? AND mode = ? AND status IN ?", provider.ID(), model.SandboxWarmSlotModeAgent, []string{
			model.SandboxWarmSlotStatusWarm, model.SandboxWarmSlotStatusWarming,
		}).
		Count(&total).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if total != 2 {
		t.Fatalf("provisioned warm slots = %d, want exactly desired (2); concurrent reconciles over-provisioned", total)
	}
}
