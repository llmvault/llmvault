package tasks

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/model"
)

func TestEmployeeTriggerRecentSoftwareEngineeringTasksIncluded(t *testing.T) {
	db := openTasksMemoryTestDB(t)
	now := time.Now().UTC()
	orgID := uuid.New()
	employee := model.Employee{
		ID:                  uuid.New(),
		OrgID:               &orgID,
		Model:               "test-model",
		Status:              "active",
		AttachedSpecialists: pq.StringArray{triggerSoftwareEngineeringSpecialistSlug},
	}
	sb := model.Sandbox{ID: uuid.New(), OrgID: &orgID, EmployeeID: &employee.ID, EncryptedRuntimeSecret: []byte("test-secret"), Status: "running"}
	session := model.EmployeeSession{ID: uuid.New(), OrgID: orgID, EmployeeID: employee.ID, SandboxID: sb.ID, RuntimeConversationID: "runtime-session"}
	if err := db.Create(&model.Org{ID: orgID, Name: "trigger-recent-" + uuid.NewString()[:8], Active: true}).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := db.Create(&employee).Error; err != nil {
		t.Fatalf("create employee: %v", err)
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}

	recentTask := model.SpecialistTask{
		ID:                     uuid.New(),
		OrgID:                  orgID,
		EmployeeID:             employee.ID,
		SpecialistSlug:         triggerSoftwareEngineeringSpecialistSlug,
		EmployeeSessionID:      session.RuntimeConversationID,
		SandboxID:              sb.ID,
		ConversationID:         &session.ID,
		ParentConversationType: "trigger",
		ParentConversationID:   "github/usehivy/hivy/pull/42",
		Brief:                  "Fix failing GitHub CI on pull request 42.",
		Status:                 "running",
		CreatedAt:              now.Add(-2 * time.Hour),
		UpdatedAt:              now.Add(-30 * time.Minute),
	}
	oldTask := recentTask
	oldTask.ID = uuid.New()
	oldTask.Brief = "Old task should not appear."
	oldTask.CreatedAt = now.Add(-8 * 24 * time.Hour)
	oldTask.UpdatedAt = now.Add(-8 * 24 * time.Hour)
	if err := db.Create(&[]model.SpecialistTask{recentTask, oldTask}).Error; err != nil {
		t.Fatalf("create tasks: %v", err)
	}
	if err := db.Create(&model.EmployeeSessionEvent{
		ID:                uuid.New(),
		OrgID:             orgID,
		EmployeeID:        employee.ID,
		SandboxID:         sb.ID,
		EmployeeSessionID: session.ID,
		SessionID:         session.RuntimeConversationID,
		EventType:         "agent.message.sent",
		Mode:              "specialist",
		SpecialistSlug:    triggerSoftwareEngineeringSpecialistSlug,
		SpecialistTaskID:  &recentTask.ID,
		Payload:           model.RawJSON(`{"text":"I found the failing test and am preparing a patch."}`),
		EventAt:           now.Add(-20 * time.Minute),
	}).Error; err != nil {
		t.Fatalf("create task event: %v", err)
	}

	handler := &EmployeeTriggerDispatchHandler{db: db, catalog: catalog.Global()}
	recentTasks, err := handler.loadRecentSoftwareEngineeringTasks(t.Context(), employee)
	if err != nil {
		t.Fatalf("load recent tasks: %v", err)
	}
	compiled := handler.compileMessage(
		EmployeeTriggerDispatchPayload{Provider: "github", EventType: "check_suite", EventAction: "completed", DeliveryID: "delivery-1"},
		model.EmployeeTrigger{ID: uuid.New(), TriggerType: "webhook", Instructions: "Handle GitHub check result."},
		map[string]any{},
		recentTasks,
	)

	for _, want := range []string{
		"Recent software engineering specialist tasks:",
		recentTask.ID.String(),
		"Fix failing GitHub CI on pull request 42.",
		"agent.message.sent: I found the failing test",
		"Call specialist_task_status with a listed task_id",
		"use search_sessions before deciding",
	} {
		if !strings.Contains(compiled.Text, want) {
			t.Fatalf("compiled text missing %q:\n%s", want, compiled.Text)
		}
	}
	if strings.Contains(compiled.Text, oldTask.Brief) {
		t.Fatalf("compiled text included old task:\n%s", compiled.Text)
	}
}

func TestEmployeeTriggerRecentSoftwareEngineeringTasksOmittedWhenNotAttached(t *testing.T) {
	handler := &EmployeeTriggerDispatchHandler{catalog: catalog.Global()}
	recentTasks := triggerRecentSpecialistTasks{SpecialistSlug: triggerSoftwareEngineeringSpecialistSlug}
	compiled := handler.compileMessage(
		EmployeeTriggerDispatchPayload{Provider: "github", EventType: "issue_comment", EventAction: "created", DeliveryID: "delivery-2"},
		model.EmployeeTrigger{ID: uuid.New(), TriggerType: "webhook", Instructions: "Handle mention."},
		map[string]any{},
		recentTasks,
	)
	if strings.Contains(compiled.Text, "Recent software engineering specialist tasks") {
		t.Fatalf("compiled text included task block for detached specialist:\n%s", compiled.Text)
	}
}

func TestEmployeeTriggerRecentSoftwareEngineeringTasksEmptyAttachedBlock(t *testing.T) {
	handler := &EmployeeTriggerDispatchHandler{catalog: catalog.Global()}
	recentTasks := triggerRecentSpecialistTasks{
		SpecialistSlug: triggerSoftwareEngineeringSpecialistSlug,
		Attached:       true,
	}
	compiled := handler.compileMessage(
		EmployeeTriggerDispatchPayload{Provider: "github", EventType: "issue_comment", EventAction: "created", DeliveryID: "delivery-3"},
		model.EmployeeTrigger{ID: uuid.New(), TriggerType: "webhook", Instructions: "Handle mention."},
		map[string]any{},
		recentTasks,
	)
	if !strings.Contains(compiled.Text, "- none created in the past 7 days") {
		t.Fatalf("compiled text missing empty attached task block:\n%s", compiled.Text)
	}
}
