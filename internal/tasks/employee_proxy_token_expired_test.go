package tasks

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/usehivy/hivy/internal/model"
)

type fakeTaskInspector struct {
	info *asynq.TaskInfo
	err  error
}

func (f fakeTaskInspector) GetTaskInfo(_, _ string) (*asynq.TaskInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.info, nil
}

func TestEnsureEmployeeProxyTokenRefreshScheduledForToken_EnqueuesExpiredEmployeeToken(t *testing.T) {
	f := newEmployeeProxyTokenRefreshFixture(t, 0)
	expiredAt := time.Now().UTC().Add(-time.Hour)
	tok := model.Token{
		ID:           uuid.New(),
		OrgID:        f.org.ID,
		CredentialID: *f.agent.CredentialID,
		JTI:          uuid.NewString(),
		ExpiresAt:    expiredAt,
		Meta: model.JSON{
			model.TokenMetaType:        model.TokenTypeEmployeeProxy,
			model.TokenMetaEmployeeID:  f.agent.ID.String(),
			model.TokenMetaSandboxID:   f.sandbox.ID.String(),
			model.TokenMetaHarness:     model.TokenHarnessEmployeeSandbox,
			model.TokenMetaRuntimeMode: model.TokenRuntimeModeEmployee,
		},
	}
	if err := f.db.Create(&tok).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}

	enqueued, err := EnsureEmployeeProxyTokenRefreshScheduledForToken(context.Background(), f.db, f.enqueuer, fakeTaskInspector{err: asynq.ErrTaskNotFound}, f.compileDeps, tok)
	if err != nil {
		t.Fatalf("ensure refresh scheduled: %v", err)
	}
	if !enqueued {
		t.Fatal("expected refresh task to be enqueued")
	}
	task := requireProxyRefreshTask(t, f.enqueuer)
	var payload EmployeeProxyTokenRefreshPayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		t.Fatalf("unmarshal task payload: %v", err)
	}
	if payload.EmployeeID != f.agent.ID || payload.SandboxID != f.sandbox.ID {
		t.Fatalf("payload ids = %s/%s, want %s/%s", payload.EmployeeID, payload.SandboxID, f.agent.ID, f.sandbox.ID)
	}
	wantScheduled := expiredAt.Add(-employeeProxyTokenRefreshLead).UTC().Truncate(time.Microsecond)
	if !payload.ScheduledAt.UTC().Truncate(time.Microsecond).Equal(wantScheduled) {
		t.Fatalf("scheduled_at = %s, want %s", payload.ScheduledAt, wantScheduled)
	}
}

func TestEnsureEmployeeProxyTokenRefreshScheduledForToken_SkipsExistingTask(t *testing.T) {
	f := newEmployeeProxyTokenRefreshFixture(t, 0)
	tok := model.Token{
		ID:           uuid.New(),
		OrgID:        f.org.ID,
		CredentialID: *f.agent.CredentialID,
		JTI:          uuid.NewString(),
		ExpiresAt:    time.Now().UTC().Add(-time.Hour),
		Meta: model.JSON{
			model.TokenMetaType:        model.TokenTypeEmployeeProxy,
			model.TokenMetaEmployeeID:  f.agent.ID.String(),
			model.TokenMetaSandboxID:   f.sandbox.ID.String(),
			model.TokenMetaHarness:     model.TokenHarnessEmployeeSandbox,
			model.TokenMetaRuntimeMode: model.TokenRuntimeModeEmployee,
		},
	}
	if err := f.db.Create(&tok).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}

	enqueued, err := EnsureEmployeeProxyTokenRefreshScheduledForToken(context.Background(), f.db, f.enqueuer, fakeTaskInspector{info: &asynq.TaskInfo{}}, f.compileDeps, tok)
	if err != nil {
		t.Fatalf("ensure refresh scheduled: %v", err)
	}
	if enqueued {
		t.Fatal("expected existing task to skip enqueue")
	}
	if len(f.enqueuer.Tasks()) != 0 {
		t.Fatalf("expected no enqueued tasks, got %d", len(f.enqueuer.Tasks()))
	}
}
