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
	defaultSandboxIdleTimeout = 5 * time.Minute
	sandboxArchiveAfterHours  = 24
)

// sandboxStillIdle re-reads the row and reports whether it is still running and idle past the
// cutoff, guarding the idle-stop against a request that became active after the bulk list query.
func (o *Orchestrator) sandboxStillIdle(ctx context.Context, id uuid.UUID, idleCutoff time.Time) (bool, error) {
	var current model.Sandbox
	if err := o.db.WithContext(ctx).
		Select("id", "status", "last_active_at", "last_preview_at").
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
	if !current.LastActiveAt.Before(idleCutoff) {
		return false, nil
	}
	if current.LastPreviewAt != nil && !current.LastPreviewAt.Before(idleCutoff) {
		return false, nil
	}
	var blockers int64
	if err := o.db.WithContext(ctx).Raw(`
		SELECT COALESCE(SUM(blocked), 0) FROM (
			SELECT COUNT(*) AS blocked FROM (
				SELECT 1
				FROM sessions s
				LEFT JOIN agents a ON a.id = s.agent_id
				WHERE s.agent_turn_status = ?
				  AND (
					s.sandbox_id = ?
					OR (
						s.sandbox_id IS NULL
						AND s.agent_id = (SELECT agent_id FROM sandboxes WHERE id = ?)
						AND a.sandbox_strategy = 'always_on'
					)
				  )
				LIMIT 1
			) active_turns
			UNION ALL
			SELECT COUNT(*) AS blocked FROM (
				SELECT 1
				FROM session_message_queue q
				JOIN sessions s ON s.id = q.session_id
				LEFT JOIN agents a ON a.id = s.agent_id
				WHERE q.status IN ('pending', 'processing')
				  AND (
					s.sandbox_id = ?
					OR (
						s.sandbox_id IS NULL
						AND s.agent_id = (SELECT agent_id FROM sandboxes WHERE id = ?)
						AND a.sandbox_strategy = 'always_on'
					)
				  )
				LIMIT 1
			) queued_messages
			UNION ALL
			SELECT COUNT(*) AS blocked FROM (
				SELECT 1
				FROM agent_schedule_runs
				WHERE sandbox_id = ? AND status IN ('queued', 'processing', 'running')
				LIMIT 1
			) queued_schedules
		) blockers
	`, model.SessionAgentTurnActive, id, id, id, id, id).Scan(&blockers).Error; err != nil {
		return false, err
	}
	return blockers == 0, nil
}

func (o *Orchestrator) RunSandboxLifecycle(ctx context.Context) {
	now := time.Now()
	idleTimeout := defaultSandboxIdleTimeout
	if o.cfg != nil && o.cfg.SandboxIdleTimeout > 0 {
		idleTimeout = o.cfg.SandboxIdleTimeout
	}
	idleCutoff := now.Add(-idleTimeout)
	archiveCutoff := now.Add(-time.Duration(sandboxArchiveAfterHours) * time.Hour)

	var idleRunning []model.Sandbox
	if err := o.db.Where(
		`status = ? AND last_active_at IS NOT NULL AND last_active_at < ?
		 AND (last_preview_at IS NULL OR last_preview_at < ?)`,
		string(StatusRunning),
		idleCutoff,
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
		`status = ? AND stopped_at IS NOT NULL AND stopped_at < ? AND agent_id IS NULL`,
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
