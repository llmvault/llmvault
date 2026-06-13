package tasks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
)

type ExpiredProxyTokenRefreshScheduler struct {
	db          *gorm.DB
	enqueuer    enqueue.TaskEnqueuer
	inspector   enqueue.TaskInspector
	compileDeps agentruntime.CompileDeps
}

func NewExpiredProxyTokenRefreshScheduler(db *gorm.DB, enqueuer enqueue.TaskEnqueuer, inspector enqueue.TaskInspector, compileDeps agentruntime.CompileDeps) *ExpiredProxyTokenRefreshScheduler {
	return &ExpiredProxyTokenRefreshScheduler{
		db:          db,
		enqueuer:    enqueuer,
		inspector:   inspector,
		compileDeps: compileDeps,
	}
}

func (s *ExpiredProxyTokenRefreshScheduler) HandleExpiredProxyToken(ctx context.Context, tok model.Token) error {
	_, err := EnsureEmployeeProxyTokenRefreshScheduledForToken(ctx, s.db, s.enqueuer, s.inspector, s.compileDeps, tok)
	return err
}

func EnsureEmployeeProxyTokenRefreshScheduledForToken(ctx context.Context, db *gorm.DB, enqueuer enqueue.TaskEnqueuer, inspector enqueue.TaskInspector, compileDeps agentruntime.CompileDeps, tok model.Token) (bool, error) {
	if db == nil || enqueuer == nil {
		return false, nil
	}
	now := time.Now().UTC()
	if tok.ID == uuid.Nil || tok.RevokedAt != nil || tok.ExpiresAt.After(now) {
		return false, nil
	}
	if tokenMetaString(tok.Meta, model.TokenMetaType) != model.TokenTypeEmployeeProxy ||
		tokenMetaString(tok.Meta, model.TokenMetaHarness) != model.TokenHarnessEmployeeSandbox {
		return false, nil
	}
	employeeID, err := uuid.Parse(tokenMetaString(tok.Meta, model.TokenMetaEmployeeID))
	if err != nil || employeeID == uuid.Nil {
		return false, nil
	}
	sandboxID, err := uuid.Parse(tokenMetaString(tok.Meta, model.TokenMetaSandboxID))
	if err != nil || sandboxID == uuid.Nil {
		return false, nil
	}
	var agent model.Employee
	if err := db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND harness = ? AND status <> ?", employeeID, tok.OrgID, "employee-sandbox", "archived").
		First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("load employee for expired proxy token refresh: %w", err)
	}
	if agent.OrgID == nil {
		return false, nil
	}
	sb, err := employeeRuntimeSelector(db, compileDeps).MainRuntimeByID(ctx, *agent.OrgID, agent.ID, sandboxID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("load sandbox for expired proxy token refresh: %w", err)
	}
	if sb.EmployeeID == nil || *sb.EmployeeID != agent.ID {
		return false, nil
	}
	scheduledAt := tok.ExpiresAt.Add(-employeeProxyTokenRefreshLead).UTC()
	taskID := EmployeeProxyTokenRefreshTaskID(sandboxID, scheduledAt)
	if inspector != nil {
		if _, err := inspector.GetTaskInfo(QueueDefault, taskID); err == nil {
			return false, nil
		} else if !errors.Is(err, asynq.ErrTaskNotFound) && !errors.Is(err, asynq.ErrQueueNotFound) {
			return false, fmt.Errorf("inspect employee proxy token refresh task: %w", err)
		}
	}
	task, opts, err := NewEmployeeProxyTokenRefreshTask(EmployeeProxyTokenRefreshPayload{
		EmployeeID:  agent.ID,
		SandboxID:   sb.ID,
		ScheduledAt: scheduledAt,
	})
	if err != nil {
		return false, err
	}
	if _, err := enqueuer.EnqueueContext(ctx, task, opts...); err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) || errors.Is(err, asynq.ErrTaskIDConflict) {
			return false, nil
		}
		return false, fmt.Errorf("enqueue expired employee proxy token refresh: %w", err)
	}
	return true, nil
}

func tokenMetaString(meta model.JSON, key string) string {
	if value, ok := meta[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}
