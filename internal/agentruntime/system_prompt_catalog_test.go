package agentruntime

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// TestEffectiveAgentInstructions_ClonesTrackCatalogUntilForked verifies the
// auto-sync contract for catalog-agent clones: a clone with a nil Instructions
// field resolves to the catalog's live instructions, and setting Instructions
// on the clone forks it (its own value wins, no more sync).
func TestEffectiveAgentInstructions_ClonesTrackCatalogUntilForked(t *testing.T) {
	db := connectCompileTestDB(t)
	ctx := context.Background()

	catalog := model.AgentCatalog{
		ID:           uuid.New(),
		Slug:         "eff-instr-" + uuid.NewString()[:8],
		Name:         "Eff Instr " + uuid.NewString()[:8],
		Model:        "test",
		SandboxImage: model.SandboxImageDefault,
		Instructions: "Catalog live instructions.",
		Manifest:     model.RawJSON(`{}`),
		Status:       model.AgentCatalogStatusActive,
	}
	if err := db.Create(&catalog).Error; err != nil {
		t.Fatalf("create catalog: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", catalog.ID).Delete(&model.AgentCatalog{}) })

	catalogID := catalog.ID
	// Unedited clone: Instructions is nil, AgentCatalog association not preloaded,
	// so it must fall back to loading the catalog's live instructions from the DB.
	clone := &model.Agent{
		AgentCatalogID: &catalogID,
	}
	if got := effectiveAgentInstructions(ctx, db, clone); got != "Catalog live instructions." {
		t.Fatalf("unedited clone instructions = %q, want catalog live", got)
	}

	// After a user edits the clone (Instructions set), the clone forks: its own
	// value wins and the catalog is no longer consulted.
	forked := "My own instructions."
	clone.Instructions = &forked
	if got := effectiveAgentInstructions(ctx, db, clone); got != forked {
		t.Fatalf("forked clone instructions = %q, want %q", got, forked)
	}
}

// TestEffectiveAgentInstructions_SnapshotPinnedOverLiveCatalog verifies that a
// clone with an InstructionsSnapshot resolves to the snapshot, NOT the live
// catalog, so a catalog edit/rename/archive cannot rewrite or blank it.
func TestEffectiveAgentInstructions_SnapshotPinnedOverLiveCatalog(t *testing.T) {
	db := connectCompileTestDB(t)
	ctx := context.Background()

	catalog := model.AgentCatalog{
		ID:           uuid.New(),
		Slug:         "snap-instr-" + uuid.NewString()[:8],
		Name:         "Snap Instr " + uuid.NewString()[:8],
		Model:        "test",
		SandboxImage: model.SandboxImageDefault,
		Instructions: "NEW live catalog instructions.",
		Manifest:     model.RawJSON(`{}`),
		Status:       model.AgentCatalogStatusActive,
	}
	if err := db.Create(&catalog).Error; err != nil {
		t.Fatalf("create catalog: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", catalog.ID).Delete(&model.AgentCatalog{}) })

	catalogID := catalog.ID
	snapshot := "Frozen template prompt from install time."
	clone := &model.Agent{
		AgentCatalogID:       &catalogID,
		InstructionsSnapshot: &snapshot,
	}
	// The snapshot wins over the (now-different) live catalog.
	if got := effectiveAgentInstructions(ctx, db, clone); got != snapshot {
		t.Fatalf("snapshotted clone instructions = %q, want the frozen snapshot", got)
	}

	// A blanked catalog (empty live instructions) still resolves to the snapshot,
	// never to an empty prompt.
	if err := db.Model(&model.AgentCatalog{}).Where("id = ?", catalogID).
		Update("instructions", "").Error; err != nil {
		t.Fatalf("blank catalog: %v", err)
	}
	if got := effectiveAgentInstructions(ctx, db, clone); got != snapshot {
		t.Fatalf("after catalog blanked, instructions = %q, want the frozen snapshot", got)
	}
}
