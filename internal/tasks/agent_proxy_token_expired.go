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
	_, err := EnsureAgentProxyTokenRefreshScheduledForToken(ctx, s.db, s.enqueuer, s.inspector, s.compileDeps, tok)
	return err
}

func EnsureAgentProxyTokenRefreshScheduledForToken(ctx context.Context, db *gorm.DB, enqueuer enqueue.TaskEnqueuer, inspector enqueue.TaskInspector, compileDeps agentruntime.CompileDeps, tok model.Token) (bool, error) {
	if db == nil || enqueuer == nil {
		return false, nil
	}
	now := time.Now().UTC()
	if tok.ID == uuid.Nil || tok.RevokedAt != nil || tok.ExpiresAt.After(now) {
		return false, nil
	}
	if tokenMetaString(tok.Meta, model.TokenMetaType) != model.TokenTypeAgentProxy ||
		tokenMetaString(tok.Meta, model.TokenMetaHarness) != model.TokenHarnessAgentSandbox {
		return false, nil
	}
	agentID, err := uuid.Parse(tokenMetaString(tok.Meta, model.TokenMetaAgentID))
	if err != nil || agentID == uuid.Nil {
		return false, nil
	}
	sandboxID, err := uuid.Parse(tokenMetaString(tok.Meta, model.TokenMetaSandboxID))
	if err != nil || sandboxID == uuid.Nil {
		return false, nil
	}
	var agent model.Agent
	if err := db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", agentID, tok.OrgID, "archived").
		First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("load agent for expired proxy token refresh: %w", err)
	}
	if agent.OrgID == nil {
		return false, nil
	}
	sb, err := loadAgentSandboxByID(ctx, db, *agent.OrgID, agent.ID, sandboxID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("load sandbox for expired proxy token refresh: %w", err)
	}
	if sb.AgentID == nil || *sb.AgentID != agent.ID {
		return false, nil
	}
	scheduledAt := tok.ExpiresAt.Add(-agentProxyTokenRefreshLead).UTC()
	taskID := AgentProxyTokenRefreshTaskID(sandboxID, scheduledAt)
	if inspector != nil {
		if _, err := inspector.GetTaskInfo(QueueDefault, taskID); err == nil {
			return false, nil
		} else if !errors.Is(err, asynq.ErrTaskNotFound) && !errors.Is(err, asynq.ErrQueueNotFound) {
			return false, fmt.Errorf("inspect agent proxy token refresh task: %w", err)
		}
	}
	task, opts, err := NewAgentProxyTokenRefreshTask(AgentProxyTokenRefreshPayload{
		AgentID:     agent.ID,
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
		return false, fmt.Errorf("enqueue expired agent proxy token refresh: %w", err)
	}
	return true, nil
}

func tokenMetaString(meta model.JSON, key string) string {
	if value, ok := meta[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}
