package sandbox

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/goroutine"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// lastActiveTouchInterval is the minimum gap between persisted last_active_at
// writes for a single sandbox. It must stay well under the idle-stop cutoff so
// the lifecycle sweep never sees a stale timestamp for an active sandbox.
const lastActiveTouchInterval = 60 * time.Second

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
	if last, ok := o.lastActiveTouch[id]; ok && now.Sub(last) < lastActiveTouchInterval {
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

func (o *Orchestrator) refreshRuntimeURL(ctx context.Context, sb *model.Sandbox) error {
	if err := o.ensureSandboxProvider(sb); err != nil {
		return err
	}
	url, err := o.provider.GetEndpoint(ctx, sb.ExternalID, RuntimePort)
	if err != nil {
		return err
	}
	expiresAt := time.Now().Add(runtimeURLTTL)
	if err := o.db.Model(sb).Updates(map[string]any{
		"runtime_url":            url,
		"runtime_url_expires_at": expiresAt,
	}).Error; err != nil {
		return fmt.Errorf("updating runtime URL: %w", err)
	}
	sb.RuntimeURL = url
	sb.RuntimeURLExpiresAt = &expiresAt
	return nil
}

func (o *Orchestrator) waitForRuntimeHealthy(ctx context.Context, sb *model.Sandbox) error {
	healthURL := sb.RuntimeURL + "/health"
	deadline := time.Now().Add(runtimeHealthTimeout)
	client := &http.Client{Timeout: 5 * time.Second}
	attempt := 0

	logging.FromContext(ctx).InfoContext(ctx, "waiting for runtime healthy", "sandbox_id", sb.ID)

	for time.Now().Before(deadline) {
		attempt++

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if err != nil {
			return fmt.Errorf("creating health request: %w", err)
		}

		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				logging.FromContext(ctx).InfoContext(ctx, "runtime healthy", "sandbox_id", sb.ID, "attempts", attempt)
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(runtimeHealthInterval):
		}
	}

	return fmt.Errorf("runtime did not become healthy within %s (%d attempts)", runtimeHealthTimeout, attempt)
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
			RuntimeURL:    sb.RuntimeURL,
			RuntimeSecret: secret,
		}, command, timeout)
	}
	return o.provider.ExecuteCommandWithTimeout(ctx, sb.ExternalID, command, timeout)
}
