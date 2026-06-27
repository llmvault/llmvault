package sandbox

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/goroutine"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// defaultLastActiveTouchInterval is the maximum gap between persisted last_active_at
// writes for a single sandbox. It must stay well under the idle-stop cutoff so
// the lifecycle sweep never sees a stale timestamp for an active sandbox.
const defaultLastActiveTouchInterval = 60 * time.Second

func (o *Orchestrator) touchLastActive(ctx context.Context, sb *model.Sandbox) {
	now := time.Now()
	sb.LastActiveAt = &now
	if !o.shouldPersistLastActive(sb.ID, now) {
		return
	}
	id := sb.ID
	goroutine.Go(context.WithoutCancel(ctx), func(ctx context.Context) {
		if err := o.db.Model(&model.Sandbox{}).
			Where("id = ?", id).
			Update("last_active_at", now).Error; err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "failed to persist last_active_at",
				"sandbox_id", id, "error", err)
			logging.Capture(ctx, err)
		}
	})
}

// shouldPersistLastActive reports whether enough time has elapsed since the last
// touch to warrant another DB write.
func (o *Orchestrator) shouldPersistLastActive(id uuid.UUID, now time.Time) bool {
	o.lastActiveTouchMu.Lock()
	defer o.lastActiveTouchMu.Unlock()
	if o.lastActiveTouch == nil {
		o.lastActiveTouch = make(map[uuid.UUID]time.Time)
	}
	interval := defaultLastActiveTouchInterval
	if o.cfg != nil && o.cfg.SandboxIdleTimeout > 0 {
		interval = o.cfg.SandboxIdleTimeout / 4
		if interval < time.Second {
			interval = time.Second
		}
		if interval > defaultLastActiveTouchInterval {
			interval = defaultLastActiveTouchInterval
		}
	}
	if last, ok := o.lastActiveTouch[id]; ok && now.Sub(last) < interval {
		return false
	}
	o.lastActiveTouch[id] = now
	return true
}

func (o *Orchestrator) needsURLRefresh(sb *model.Sandbox) bool {
	if sb.RuntimeURL == "" {
		return true
	}
	if sb.RuntimeURLExpiresAt == nil {
		return true
	}
	return time.Now().Add(runtimeURLRefreshBuffer).After(*sb.RuntimeURLExpiresAt)
}

func (o *Orchestrator) ExecuteCommand(ctx context.Context, sb *model.Sandbox, command string) (string, error) {
	return o.ExecuteCommandWithTimeout(ctx, sb, command, 0)
}

func (o *Orchestrator) ExecuteCommandWithTimeout(ctx context.Context, sb *model.Sandbox, command string, timeout time.Duration) (string, error) {
	if err := o.ensureSandboxProvider(sb); err != nil {
		return "", err
	}
	if executor, ok := o.provider.(RuntimeCommandExecutor); ok {
		secret, err := o.encKey.DecryptString(sb.EncryptedRuntimeSecret)
		if err != nil {
			return "", fmt.Errorf("decrypt runtime secret: %w", err)
		}
		return executor.ExecuteCommandViaRuntime(ctx, RuntimeCommandContext{
			RuntimeURL:    o.runtimeControlURL(sb.RuntimeURL),
			RuntimeSecret: secret,
		}, command, timeout)
	}
	return o.provider.ExecuteCommandWithTimeout(ctx, sb.ExternalID, command, timeout)
}
