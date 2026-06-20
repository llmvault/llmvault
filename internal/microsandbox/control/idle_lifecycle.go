package control

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/microsandbox/model"
)

func (s *Server) watchIdleSandboxes(ctx context.Context) {
	interval := s.cfg.IdleCheckInterval
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.stopIdleSandboxes(ctx)
		}
	}
}

func (s *Server) stopIdleSandboxes(ctx context.Context) {
	logger := logging.FromContext(ctx)
	var sandboxes []model.Sandbox
	now := time.Now().UTC()
	if err := s.db.WithContext(ctx).
		Where("status = ? AND auto_sleep_after_seconds > 0 AND sleep_after_at IS NOT NULL AND sleep_after_at <= ?", model.SandboxStatusRunning, now).
		Find(&sandboxes).Error; err != nil {
		logger.ErrorContext(ctx, "microsandbox idle lifecycle query failed", "error", err)
		return
	}
	for i := range sandboxes {
		sb := sandboxes[i]
		if !s.sandboxIdleEnough(sb, time.Now().UTC()) {
			continue
		}
		if err := s.stopIdleSandbox(ctx, sb); err != nil {
			logger.WarnContext(ctx, "microsandbox idle stop failed", "sandbox_id", sb.ID, "error", err)
			continue
		}
	}
}

func (s *Server) sandboxIdleEnough(sb model.Sandbox, now time.Time) bool {
	if sb.AutoSleepAfterSeconds <= 0 || sb.Status != model.SandboxStatusRunning {
		return false
	}
	if sb.RuntimeBusy {
		timeout := s.cfg.RuntimeBusyTimeout
		if timeout <= 0 {
			timeout = 90 * time.Second
		}
		if sb.LastRuntimeActivityAt != nil && now.Sub(*sb.LastRuntimeActivityAt) <= timeout {
			return false
		}
	}
	if sb.SleepAfterAt != nil {
		return !sb.SleepAfterAt.After(now)
	}
	return false
}

func (s *Server) stopIdleSandbox(ctx context.Context, sb model.Sandbox) error {
	unlock := s.lifecycleLocks.Lock(sb.ID)
	defer unlock()
	distributedUnlock, err := s.acquireDistributedSandboxLock(ctx, sb.ID)
	if err != nil {
		return err
	}
	defer distributedUnlock()

	var fresh model.Sandbox
	if err := s.db.WithContext(ctx).First(&fresh, "id = ?", sb.ID).Error; err != nil {
		return err
	}
	if !s.sandboxIdleEnough(fresh, time.Now().UTC()) {
		return nil
	}
	var runner model.Runner
	if err := s.db.WithContext(ctx).First(&runner, "id = ?", fresh.RunnerID).Error; err != nil {
		return err
	}
	if s.sandboxHasActiveConnections(ctx, runner, fresh) {
		now := time.Now().UTC()
		sleepAfterAt := nextSleepAfter(fresh, now)
		if err := s.db.WithContext(ctx).Model(&fresh).Updates(map[string]any{
			"last_gateway_activity_at": now,
			"sleep_after_at":           sleepAfterAt,
		}).Error; err != nil {
			return err
		}
		fresh.LastGatewayActivityAt = &now
		fresh.SleepAfterAt = sleepAfterAt
		s.syncPreviewRoute(ctx, fresh, runner, routePorts(ctx, s.db, fresh.ID))
		logging.FromContext(ctx).InfoContext(ctx, "microsandbox idle stop deferred for active connections", "sandbox_id", fresh.ID)
		return nil
	}
	if err := s.client.Post(ctx, runner.APIURL, "/v1/sandboxes/"+fresh.ID+"/stop", nil, nil); err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if runtimeReservationHeld(fresh.Status) {
			if err := releaseRunnerReservationForSandboxTx(tx, fresh, runtimeReservationSize(fresh)); err != nil {
				return err
			}
		}
		return tx.Model(&fresh).Updates(map[string]any{
			"status":           model.SandboxStatusStopped,
			"stopped_at":       now,
			"runtime_busy":     false,
			"sleep_after_at":   nil,
			"route_generation": gorm.Expr("route_generation + 1"),
		}).Error
	}); err != nil {
		return err
	}
	fresh.Status = model.SandboxStatusStopped
	fresh.StoppedAt = &now
	fresh.RuntimeBusy = false
	fresh.SleepAfterAt = nil
	fresh.RouteGeneration++
	s.syncPreviewRoute(ctx, fresh, runner, routePorts(ctx, s.db, fresh.ID))
	logging.FromContext(ctx).InfoContext(ctx, "microsandbox idle stopped", "sandbox_id", fresh.ID)
	return nil
}

type runnerConnectionsResponse struct {
	ActiveConnections int `json:"active_connections"`
}

func (s *Server) sandboxHasActiveConnections(ctx context.Context, runner model.Runner, sb model.Sandbox) bool {
	var resp runnerConnectionsResponse
	if err := s.client.GetJSON(ctx, runner.APIURL, "/v1/sandboxes/"+sb.ID+"/connections", &resp); err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "microsandbox active connection check failed", "sandbox_id", sb.ID, "error", err)
		return false
	}
	return resp.ActiveConnections > 0
}
