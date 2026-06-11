package sandbox

import (
	"context"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/model"
)

// TestSandboxStillIdle_SkipsRecentlyActive verifies the P1-16 guarded
// conditional stop: a sandbox that was listed as idle but whose last_active_at
// advanced past the cutoff (an in-flight request landed) must not be stopped.
func TestSandboxStillIdle_SkipsRecentlyActive(t *testing.T) {
	orch, _, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := newAgentWithHarness(t, db, org.ID, cred.ID, "claude")

	idleCutoff := time.Now().Add(-10 * time.Minute)

	// Originally idle.
	staleAt := time.Now().Add(-30 * time.Minute)
	sb := newRunningSandbox(t, db, &org.ID, &agent.ID, "ext-race", staleAt)

	stillIdle, err := orch.sandboxStillIdle(context.Background(), sb.ID, idleCutoff)
	if err != nil {
		t.Fatalf("sandboxStillIdle: %v", err)
	}
	if !stillIdle {
		t.Fatal("expected sandbox to be idle before any activity")
	}

	// Simulate an in-flight request touching last_active_at after the bulk list.
	freshAt := time.Now()
	if err := db.Model(&model.Sandbox{}).Where("id = ?", sb.ID).Update("last_active_at", freshAt).Error; err != nil {
		t.Fatalf("touch last_active_at: %v", err)
	}

	stillIdle, err = orch.sandboxStillIdle(context.Background(), sb.ID, idleCutoff)
	if err != nil {
		t.Fatalf("sandboxStillIdle after touch: %v", err)
	}
	if stillIdle {
		t.Fatal("expected sandbox to NOT be idle after an in-flight request touched last_active_at")
	}
}

// TestSandboxStillIdle_SkipsNonRunning verifies a sandbox that left the running
// state between the list and the re-check is not stopped.
func TestSandboxStillIdle_SkipsNonRunning(t *testing.T) {
	orch, _, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := newAgentWithHarness(t, db, org.ID, cred.ID, "claude")

	staleAt := time.Now().Add(-30 * time.Minute)
	sb := newRunningSandbox(t, db, &org.ID, &agent.ID, "ext-race-status", staleAt)

	if err := db.Model(&model.Sandbox{}).Where("id = ?", sb.ID).Update("status", string(StatusStopped)).Error; err != nil {
		t.Fatalf("update status: %v", err)
	}

	stillIdle, err := orch.sandboxStillIdle(context.Background(), sb.ID, time.Now().Add(-10*time.Minute))
	if err != nil {
		t.Fatalf("sandboxStillIdle: %v", err)
	}
	if stillIdle {
		t.Fatal("expected a non-running sandbox to be reported not idle")
	}
}

// TestRunSandboxLifecycle_DoesNotStopRecentlyActive verifies that a freshly
// active sandbox (last_active_at within the cutoff) is never selected for an
// idle stop by the full lifecycle pass.
func TestRunSandboxLifecycle_DoesNotStopRecentlyActive(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := newAgentWithHarness(t, db, org.ID, cred.ID, "claude")

	activeAt := time.Now().Add(-1 * time.Minute) // well within the 10m idle cutoff
	sb := newRunningSandbox(t, db, &org.ID, &agent.ID, "ext-active", activeAt)
	provider.registerSandbox("ext-active", StatusRunning)

	orch.RunSandboxLifecycle(context.Background())

	var after model.Sandbox
	db.Where("id = ?", sb.ID).First(&after)
	if after.Status != string(StatusRunning) {
		t.Fatalf("status = %q, want running (active sandbox must not be stopped)", after.Status)
	}
	if len(provider.stoppedIDs) != 0 {
		t.Fatalf("stoppedIDs = %#v, want none", provider.stoppedIDs)
	}
}

// TestTouchLastActiveDebounces verifies the P1-16 debounce: repeated touches
// within the interval persist the timestamp only once, while the in-memory
// LastActiveAt is always updated.
func TestTouchLastActiveDebounces(t *testing.T) {
	orch, _, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := newAgentWithHarness(t, db, org.ID, cred.ID, "claude")

	old := time.Now().Add(-time.Hour)
	sb := newRunningSandbox(t, db, &org.ID, &agent.ID, "ext-touch", old)

	ctx := context.Background()
	orch.touchLastActive(ctx, &sb)
	if sb.LastActiveAt == nil || sb.LastActiveAt.Before(old.Add(time.Minute)) {
		t.Fatal("expected in-memory LastActiveAt to advance on touch")
	}

	// Wait for the first async persist to land.
	deadline := time.Now().Add(3 * time.Second)
	var firstPersist model.Sandbox
	for {
		db.Where("id = ?", sb.ID).First(&firstPersist)
		if firstPersist.LastActiveAt != nil && firstPersist.LastActiveAt.After(old.Add(time.Minute)) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("first touch was never persisted")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// A second touch within the debounce window must not schedule another write.
	orch.touchLastActive(ctx, &sb)
	time.Sleep(200 * time.Millisecond)
	var secondPersist model.Sandbox
	db.Where("id = ?", sb.ID).First(&secondPersist)
	if !secondPersist.LastActiveAt.Equal(*firstPersist.LastActiveAt) {
		t.Fatalf("debounced touch persisted a new timestamp: %v != %v", secondPersist.LastActiveAt, firstPersist.LastActiveAt)
	}
}
