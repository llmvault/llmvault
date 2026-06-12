package tasks

import (
	"context"
	"testing"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

// The new sandbox must NOT receive live traffic before its session DB is restored
// and synced. It is parked 'upgrading' (non-selectable) until sync, so the selector
// must resolve to the OLD sandbox while the new one syncs.
func TestEmployeeSandboxUpgradeKeepsNewSandboxNonSelectableUntilSync(t *testing.T) {
	f := newEmployeeUpgradeFixture(t)

	selector := employeeRuntimeSelector(f.db, f.handler.compileDeps)
	var selectedDuringSync *model.Sandbox
	f.runtime.setOnConfig(func() {
		sb, err := selector.MainRuntime(context.Background(), f.org.ID, f.agent.ID)
		if err != nil {
			t.Errorf("selector during sync: %v", err)
			return
		}
		selectedDuringSync = sb
	})

	if err := f.handler.Handle(context.Background(), employeeUpgradeTask(t, f.upgrade.ID, f.agent.ID)); err != nil {
		t.Fatalf("handle: %v", err)
	}

	if selectedDuringSync == nil {
		t.Fatal("config sync hook did not run")
	}
	if selectedDuringSync.ID != f.old.ID {
		t.Fatalf("selector returned %s during pre-activation sync, want old sandbox %s (new sandbox was selectable before restore/sync)",
			selectedDuringSync.ID, f.old.ID)
	}

	var upgrade model.EmployeeSandboxUpgrade
	if err := f.db.First(&upgrade, "id = ?", f.upgrade.ID).Error; err != nil {
		t.Fatalf("load upgrade: %v", err)
	}
	if upgrade.NewSandboxID == nil {
		t.Fatal("new sandbox id not recorded")
	}
	var newSandbox model.Sandbox
	if err := f.db.First(&newSandbox, "id = ?", *upgrade.NewSandboxID).Error; err != nil {
		t.Fatalf("load new sandbox: %v", err)
	}
	if newSandbox.Status != string(sandbox.StatusRunning) {
		t.Fatalf("new sandbox status = %q, want running after upgrade", newSandbox.Status)
	}
	final, err := selector.MainRuntime(context.Background(), f.org.ID, f.agent.ID)
	if err != nil {
		t.Fatalf("selector after upgrade: %v", err)
	}
	if final.ID != *upgrade.NewSandboxID {
		t.Fatalf("selector returned %s after upgrade, want new sandbox %s", final.ID, *upgrade.NewSandboxID)
	}
}

// Rollback-on-WithoutCancel fix: a create failure with an already-cancelled task
// context must still roll back (mark failed, leave old running) rather than
// no-op on the cancelled ctx and strand both sandboxes.
func TestEmployeeSandboxUpgradeRollbackRunsOnCancelledContext(t *testing.T) {
	f := newEmployeeUpgradeFixture(t)
	f.provider.failCreate = true

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel the task context when create is attempted, so the rollback that
	// follows sees an already-cancelled ctx (the WithoutCancel regression).
	f.provider.onCreate = cancel

	_ = f.handler.Handle(ctx, employeeUpgradeTask(t, f.upgrade.ID, f.agent.ID))

	var upgrade model.EmployeeSandboxUpgrade
	if err := f.db.First(&upgrade, "id = ?", f.upgrade.ID).Error; err != nil {
		t.Fatalf("load upgrade: %v", err)
	}
	if upgrade.Status != model.EmployeeSandboxUpgradeStatusFailed {
		t.Fatalf("upgrade status = %q, want failed (rollback did not run on cancelled ctx)", upgrade.Status)
	}
	var old model.Sandbox
	if err := f.db.First(&old, "id = ?", f.old.ID).Error; err != nil {
		t.Fatalf("load old sandbox: %v", err)
	}
	if old.Status != string(sandbox.StatusRunning) {
		t.Fatalf("old sandbox status = %q, want running after rollback", old.Status)
	}
}
