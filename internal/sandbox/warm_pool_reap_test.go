package sandbox

import (
	"context"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

// MarkError releases the warm slot's paid provider resource.
func TestWarmPoolMarkErrorDeletesProviderResource(t *testing.T) {
	db := setupTestDB(t)
	provider := newMockProvider()
	db.Where("provider_id = ?", provider.ID()).Delete(&model.SandboxWarmSlot{})
	pool := NewWarmPool(db, provider, testEncKey(t), &config.Config{
		SandboxesRuntimeBaseImage: "runtime:test",
	})
	provider.registerSandbox("warm-ext-1", StatusRunning)
	slot := model.SandboxWarmSlot{
		ProviderID:             provider.ID(),
		Mode:                   model.SandboxWarmSlotModeEmployee,
		Status:                 model.SandboxWarmSlotStatusWarm,
		ExternalID:             "warm-ext-1",
		EndpointURL:            "https://warm.test",
		RuntimeImage:           "runtime:test",
		EncryptedRuntimeSecret: []byte("enc"),
	}
	if err := db.Create(&slot).Error; err != nil {
		t.Fatalf("create warm slot: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", slot.ID).Delete(&model.SandboxWarmSlot{}) })

	if err := pool.MarkError(context.Background(), slot.ID, "decrypt failed"); err != nil {
		t.Fatalf("mark error: %v", err)
	}
	if len(provider.deletedIDs) != 1 || provider.deletedIDs[0] != "warm-ext-1" {
		t.Fatalf("provider delete calls = %v, want [warm-ext-1]", provider.deletedIDs)
	}
	var reloaded model.SandboxWarmSlot
	if err := db.First(&reloaded, "id = ?", slot.ID).Error; err != nil {
		t.Fatalf("reload slot: %v", err)
	}
	if reloaded.Status != model.SandboxWarmSlotStatusError {
		t.Fatalf("slot status = %q, want error", reloaded.Status)
	}
}

// Warm slots stranded in 'claiming' beyond the TTL must have their provider resources deleted.
func TestWarmPoolReapStaleSlots(t *testing.T) {
	db := setupTestDB(t)
	provider := newMockProvider()
	db.Where("provider_id = ?", provider.ID()).Delete(&model.SandboxWarmSlot{})
	pool := NewWarmPool(db, provider, testEncKey(t), &config.Config{
		SandboxesRuntimeBaseImage: "runtime:test",
	})
	provider.registerSandbox("stale-claim-1", StatusRunning)
	slot := model.SandboxWarmSlot{
		ProviderID:             provider.ID(),
		Mode:                   model.SandboxWarmSlotModeEmployee,
		Status:                 model.SandboxWarmSlotStatusClaiming,
		ExternalID:             "stale-claim-1",
		EndpointURL:            "https://warm.test",
		RuntimeImage:           "runtime:test",
		EncryptedRuntimeSecret: []byte("enc"),
	}
	if err := db.Create(&slot).Error; err != nil {
		t.Fatalf("create warm slot: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", slot.ID).Delete(&model.SandboxWarmSlot{}) })
	if err := db.Model(&model.SandboxWarmSlot{}).Where("id = ?", slot.ID).
		Update("updated_at", time.Now().Add(-20*time.Minute)).Error; err != nil {
		t.Fatalf("backdate slot: %v", err)
	}

	if err := pool.ReapStaleSlots(context.Background()); err != nil {
		t.Fatalf("reap stale slots: %v", err)
	}
	if len(provider.deletedIDs) != 1 || provider.deletedIDs[0] != "stale-claim-1" {
		t.Fatalf("provider delete calls = %v, want [stale-claim-1]", provider.deletedIDs)
	}
	var reloaded model.SandboxWarmSlot
	if err := db.First(&reloaded, "id = ?", slot.ID).Error; err != nil {
		t.Fatalf("reload slot: %v", err)
	}
	if reloaded.Status != model.SandboxWarmSlotStatusError {
		t.Fatalf("slot status = %q, want error", reloaded.Status)
	}
}

// Reaper does not touch a recently-claimed slot (claim still in flight).
func TestWarmPoolReapStaleSlotsLeavesFreshClaiming(t *testing.T) {
	db := setupTestDB(t)
	provider := newMockProvider()
	db.Where("provider_id = ?", provider.ID()).Delete(&model.SandboxWarmSlot{})
	pool := NewWarmPool(db, provider, testEncKey(t), &config.Config{
		SandboxesRuntimeBaseImage: "runtime:test",
	})
	provider.registerSandbox("fresh-claim-1", StatusRunning)
	slot := model.SandboxWarmSlot{
		ProviderID:             provider.ID(),
		Mode:                   model.SandboxWarmSlotModeEmployee,
		Status:                 model.SandboxWarmSlotStatusClaiming,
		ExternalID:             "fresh-claim-1",
		EndpointURL:            "https://warm.test",
		RuntimeImage:           "runtime:test",
		EncryptedRuntimeSecret: []byte("enc"),
	}
	if err := db.Create(&slot).Error; err != nil {
		t.Fatalf("create warm slot: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", slot.ID).Delete(&model.SandboxWarmSlot{}) })

	if err := pool.ReapStaleSlots(context.Background()); err != nil {
		t.Fatalf("reap stale slots: %v", err)
	}
	if len(provider.deletedIDs) != 0 {
		t.Fatalf("provider delete calls = %v, want none for fresh claiming slot", provider.deletedIDs)
	}
}
