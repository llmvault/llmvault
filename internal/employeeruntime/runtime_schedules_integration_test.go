package employeeruntime

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
	"gorm.io/gorm"
)

// scheduleTestFixture creates an org, employee, and a sandbox the schedule is
// initially pointed at.
func scheduleTestFixture(t *testing.T, db *gorm.DB) (model.Employee, model.Sandbox) {
	t.Helper()
	org := model.Org{Name: "Sched-" + uuid.NewString()}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	agent := model.Employee{
		ID:            uuid.New(),
		OrgID:         &org.ID,
		Name:          "Aria",
		Model:         DefaultEmployeeModel,
		Tools:         model.JSON{},
		McpServers:    model.RawJSON("[]"),
		Skills:        model.JSON{},
		Integrations:  model.JSON{},
		Resources:     model.JSON{},
		RuntimeConfig: model.JSON{},
		Permissions:   model.JSON{},
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}

	sb := model.Sandbox{
		ID:                     uuid.New(),
		OrgID:                  &org.ID,
		EmployeeID:             &agent.ID,
		ProviderID:             "docker",
		ExternalID:             "ext-" + uuid.NewString(),
		RuntimeURL:             "http://localhost:7080",
		EncryptedRuntimeSecret: []byte("secret"),
		Status:                 "running",
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	return agent, sb
}

func createSchedule(t *testing.T, db *gorm.DB, agent model.Employee, sandboxID uuid.UUID) model.EmployeeSchedule {
	t.Helper()
	next := time.Now().Add(time.Hour)
	sched := model.EmployeeSchedule{
		ID:           uuid.New(),
		OrgID:        *agent.OrgID,
		EmployeeID:   agent.ID,
		SandboxID:    &sandboxID,
		RuntimeJobID: "cron-" + uuid.NewString(),
		Status:       "active",
		TaskPrompt:   "do the thing",
		NextRunAt:    &next,
	}
	if err := db.Create(&sched).Error; err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	return sched
}

// TestBuildRuntimeSchedules_DoesNotRepointSandbox verifies that the read path no
// longer mutates schedule rows: compiling a config for a different sandbox must
// leave the schedule's sandbox_id untouched.
func TestBuildRuntimeSchedules_DoesNotRepointSandbox(t *testing.T) {
	db := connectCompileTestDB(t)
	agent, origSb := scheduleTestFixture(t, db)
	sched := createSchedule(t, db, agent, origSb.ID)

	// A second sandbox the config is being compiled for (e.g. an upgrade target
	// or, before the fix, a specialist sandbox).
	otherSb := model.Sandbox{
		ID:                     uuid.New(),
		OrgID:                  agent.OrgID,
		EmployeeID:             &agent.ID,
		ProviderID:             "docker",
		ExternalID:             "ext-" + uuid.NewString(),
		RuntimeURL:             "http://localhost:7081",
		EncryptedRuntimeSecret: []byte("secret"),
		Status:                 "running",
	}
	if err := db.Create(&otherSb).Error; err != nil {
		t.Fatalf("create other sandbox: %v", err)
	}

	schedules, err := BuildRuntimeSchedules(context.Background(), db, &agent, &otherSb)
	if err != nil {
		t.Fatalf("BuildRuntimeSchedules: %v", err)
	}
	if len(schedules) != 1 {
		t.Fatalf("expected 1 compiled schedule, got %d", len(schedules))
	}

	var reloaded model.EmployeeSchedule
	if err := db.Where("id = ?", sched.ID).First(&reloaded).Error; err != nil {
		t.Fatalf("reload schedule: %v", err)
	}
	if reloaded.SandboxID == nil || *reloaded.SandboxID != origSb.ID {
		t.Fatalf("BuildRuntimeSchedules must not repoint sandbox_id: got %v want %v", reloaded.SandboxID, origSb.ID)
	}
}

// TestRepointEmployeeSchedules_RepointsAfterPush verifies the explicit repoint
// path moves active/paused schedules onto the new sandbox.
func TestRepointEmployeeSchedules_RepointsAfterPush(t *testing.T) {
	db := connectCompileTestDB(t)
	agent, origSb := scheduleTestFixture(t, db)
	sched := createSchedule(t, db, agent, origSb.ID)

	newSb := model.Sandbox{
		ID:                     uuid.New(),
		OrgID:                  agent.OrgID,
		EmployeeID:             &agent.ID,
		ProviderID:             "docker",
		ExternalID:             "ext-" + uuid.NewString(),
		RuntimeURL:             "http://localhost:7082",
		EncryptedRuntimeSecret: []byte("secret"),
		Status:                 "running",
	}
	if err := db.Create(&newSb).Error; err != nil {
		t.Fatalf("create new sandbox: %v", err)
	}

	if err := RepointEmployeeSchedules(context.Background(), db, &agent, &newSb); err != nil {
		t.Fatalf("RepointEmployeeSchedules: %v", err)
	}

	var reloaded model.EmployeeSchedule
	if err := db.Where("id = ?", sched.ID).First(&reloaded).Error; err != nil {
		t.Fatalf("reload schedule: %v", err)
	}
	if reloaded.SandboxID == nil || *reloaded.SandboxID != newSb.ID {
		t.Fatalf("RepointEmployeeSchedules must repoint to new sandbox: got %v want %v", reloaded.SandboxID, newSb.ID)
	}
}

// TestSandboxDelete_SetsScheduleSandboxNull verifies the FK is ON DELETE SET NULL:
// hard-deleting the sandbox a schedule points at must null sandbox_id, not delete
// the schedule (which CASCADE used to do — the data-loss bug).
func TestSandboxDelete_SetsScheduleSandboxNull(t *testing.T) {
	db := connectCompileTestDB(t)
	agent, sb := scheduleTestFixture(t, db)
	sched := createSchedule(t, db, agent, sb.ID)

	if err := db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}).Error; err != nil {
		t.Fatalf("delete sandbox: %v", err)
	}

	var reloaded model.EmployeeSchedule
	if err := db.Where("id = ?", sched.ID).First(&reloaded).Error; err != nil {
		t.Fatalf("schedule must survive sandbox deletion: %v", err)
	}
	if reloaded.SandboxID != nil {
		t.Fatalf("sandbox_id must be NULL after sandbox delete, got %v", *reloaded.SandboxID)
	}
}
