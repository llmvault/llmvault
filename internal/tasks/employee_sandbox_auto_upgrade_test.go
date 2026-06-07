package tasks

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
)

func TestEmployeeSandboxAutoUpgradeHandler_EnqueuesOutdatedEmployeeSandbox(t *testing.T) {
	f := newEmployeeUpgradeFixture(t)
	targetImage := f.handler.compileDeps.Cfg.SandboxesRuntimeBaseImage
	if err := f.db.Delete(&f.upgrade).Error; err != nil {
		t.Fatalf("delete active upgrade: %v", err)
	}
	oldImage := "ghcr.io/usehivy/hivy-sandboxes-runtime:old"
	if err := f.db.Model(&model.Sandbox{}).Where("id = ?", f.old.ID).Update("snapshot_id", oldImage).Error; err != nil {
		t.Fatalf("mark old snapshot: %v", err)
	}
	handler := NewEmployeeSandboxAutoUpgradeHandler(f.db, f.handler.compileDeps, f.enqueuer)

	if err := handler.Handle(context.Background(), employeeAutoUpgradeTask(t, EmployeeSandboxAutoUpgradePayload{
		RuntimeImage: targetImage,
		Limit:        100,
	})); err != nil {
		t.Fatalf("handle auto upgrade: %v", err)
	}

	task := requireUpgradeTaskForEmployee(t, f.enqueuer, f.agent.ID)
	var payload EmployeeSandboxUpgradePayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		t.Fatalf("unmarshal upgrade payload: %v", err)
	}
	if payload.EmployeeID != f.agent.ID {
		t.Fatalf("employee id = %s, want %s", payload.EmployeeID, f.agent.ID)
	}
	var upgrade model.EmployeeSandboxUpgrade
	if err := f.db.First(&upgrade, "id = ?", payload.UpgradeID).Error; err != nil {
		t.Fatalf("load upgrade: %v", err)
	}
	if upgrade.OldSandboxID == nil || *upgrade.OldSandboxID != f.old.ID {
		t.Fatalf("old sandbox id = %v, want %s", upgrade.OldSandboxID, f.old.ID)
	}
}

func TestEmployeeSandboxAutoUpgradeHandler_SkipsActiveUpgrade(t *testing.T) {
	f := newEmployeeUpgradeFixture(t)
	targetImage := f.handler.compileDeps.Cfg.SandboxesRuntimeBaseImage
	oldImage := "ghcr.io/usehivy/hivy-sandboxes-runtime:old"
	if err := f.db.Model(&model.Sandbox{}).Where("id = ?", f.old.ID).Update("snapshot_id", oldImage).Error; err != nil {
		t.Fatalf("mark old snapshot: %v", err)
	}
	handler := NewEmployeeSandboxAutoUpgradeHandler(f.db, f.handler.compileDeps, f.enqueuer)

	if err := handler.Handle(context.Background(), employeeAutoUpgradeTask(t, EmployeeSandboxAutoUpgradePayload{
		RuntimeImage: targetImage,
		Limit:        100,
	})); err != nil {
		t.Fatalf("handle auto upgrade: %v", err)
	}

	for _, task := range f.enqueuer.Tasks() {
		if task.TypeName == TypeEmployeeSandboxUpgrade {
			var payload EmployeeSandboxUpgradePayload
			if err := json.Unmarshal(task.Payload, &payload); err != nil {
				t.Fatalf("unmarshal upgrade payload: %v", err)
			}
			if payload.EmployeeID == f.agent.ID {
				t.Fatalf("upgrade task enqueued despite active upgrade: %#v", task)
			}
		}
	}
}

func employeeAutoUpgradeTask(t *testing.T, payload EmployeeSandboxAutoUpgradePayload) *asynq.Task {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return asynq.NewTask(TypeEmployeeSandboxAutoUpgrade, body)
}

func requireUpgradeTaskForEmployee(t *testing.T, c *enqueue.MockClient, employeeID uuid.UUID) *enqueue.EnqueuedTask {
	t.Helper()
	for _, task := range c.Tasks() {
		if task.TypeName == TypeEmployeeSandboxUpgrade {
			var payload EmployeeSandboxUpgradePayload
			if err := json.Unmarshal(task.Payload, &payload); err != nil {
				t.Fatalf("unmarshal upgrade payload: %v", err)
			}
			if payload.EmployeeID == employeeID {
				return &task
			}
		}
	}
	t.Fatalf("upgrade task not enqueued for employee %s", employeeID)
	return nil
}
