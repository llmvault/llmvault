package sandbox

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

const (
	sandboxIdleTimeoutMinutes = 10
	sandboxArchiveAfterHours  = 24
)

// sandboxStillIdle re-reads the row and reports whether it is still running and idle past the
// cutoff, guarding the idle-stop against a request that became active after the bulk list query.
func (o *Orchestrator) sandboxStillIdle(ctx context.Context, id uuid.UUID, idleCutoff time.Time) (bool, error) {
	var current model.Sandbox
	if err := o.db.WithContext(ctx).
		Select("id", "status", "last_active_at").
		Where("id = ?", id).
		First(&current).Error; err != nil {
		return false, err
	}
	if current.Status != string(StatusRunning) {
		return false, nil
	}
	if current.LastActiveAt == nil {
		return false, nil
	}
	return current.LastActiveAt.Before(idleCutoff), nil
}

func (o *Orchestrator) RunSandboxLifecycle(ctx context.Context) {
	now := time.Now()
	idleCutoff := now.Add(-time.Duration(sandboxIdleTimeoutMinutes) * time.Minute)
	archiveCutoff := now.Add(-time.Duration(sandboxArchiveAfterHours) * time.Hour)

	var idleRunning []model.Sandbox
	if err := o.db.Where(
		`status = ? AND last_active_at IS NOT NULL AND last_active_at < ?
		 AND NOT EXISTS (SELECT 1 FROM agents a WHERE a.id = sandboxes.agent_id AND a.harness = 'agent-sandbox')`,
		string(StatusRunning),
		idleCutoff,
	).Find(&idleRunning).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "sandbox lifecycle: query idle running sandboxes failed", "error", err)
	} else {
		for i := range idleRunning {
			sb := &idleRunning[i]
			// Re-check before stopping: an in-flight request may have touched
			// last_active_at since the list query (don't stop mid-turn).
			stillIdle, err := o.sandboxStillIdle(ctx, sb.ID, idleCutoff)
			if err != nil {
				logging.FromContext(ctx).WarnContext(ctx, "sandbox lifecycle: re-check idle sandbox failed",
					"sandbox_id", sb.ID, "error", err)
				continue
			}
			if !stillIdle {
				continue
			}
			if err := o.StopSandbox(ctx, sb); err != nil && !errors.Is(err, ErrSandboxNotFound) && !errors.Is(err, ErrUnsupported) {
				logging.FromContext(ctx).WarnContext(ctx, "sandbox lifecycle: failed to stop idle sandbox",
					"sandbox_id", sb.ID, "error", err)
				logging.Capture(ctx, err)
			}
		}
	}

	var staleStopped []model.Sandbox
	if err := o.db.Where(
		`status = ? AND stopped_at IS NOT NULL AND stopped_at < ?
		 AND NOT EXISTS (SELECT 1 FROM agents a WHERE a.id = sandboxes.agent_id AND a.harness = 'agent-sandbox')`,
		string(StatusStopped),
		archiveCutoff,
	).Find(&staleStopped).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "sandbox lifecycle: query stale stopped sandboxes failed", "error", err)
	} else {
		for i := range staleStopped {
			sb := &staleStopped[i]
			if err := o.ArchiveSandbox(ctx, sb); err != nil && !errors.Is(err, ErrSandboxNotFound) && !errors.Is(err, ErrUnsupported) {
				logging.FromContext(ctx).WarnContext(ctx, "sandbox lifecycle: failed to archive stopped sandbox",
					"sandbox_id", sb.ID, "error", err)
				logging.Capture(ctx, err)
			}
		}
	}
}
