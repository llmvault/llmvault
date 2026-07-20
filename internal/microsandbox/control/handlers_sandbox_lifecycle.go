package control

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/microsandbox/api"
	"github.com/usehivy/hivy/internal/microsandbox/httpx"
	"github.com/usehivy/hivy/internal/microsandbox/model"
)

func (s *Server) startSandbox(w http.ResponseWriter, r *http.Request) {
	s.lifecycle(w, r, "start", model.SandboxStatusRunning)
}

func (s *Server) stopSandbox(w http.ResponseWriter, r *http.Request) {
	s.stopSandboxLifecycle(w, r)
}

func (s *Server) lifecycle(w http.ResponseWriter, r *http.Request, action, nextStatus string) {
	sandboxID := chi.URLParam(r, "sandboxID")
	unlock := s.lifecycleLocks.Lock(sandboxID)
	defer unlock()
	distributedUnlock, err := s.acquireDistributedSandboxLock(r.Context(), sandboxID)
	if err != nil {
		httpx.JSON(w, http.StatusServiceUnavailable, api.ErrorResponse{Error: err.Error()})
		return
	}
	defer distributedUnlock()

	sb, runner, ok := s.loadSandboxRunner(w, r)
	if !ok {
		return
	}
	if sb.Status == nextStatus {
		httpx.JSON(w, http.StatusOK, map[string]string{"status": nextStatus})
		return
	}
	if sb.Status == model.SandboxStatusStopping {
		httpx.JSON(w, http.StatusConflict, api.ErrorResponse{Error: "sandbox lifecycle operation is in progress"})
		return
	}
	if nextStatus == model.SandboxStatusRunning && !runtimeReservationHeld(sb.Status) {
		if err := reserveRunnerReservationForSandbox(r.Context(), s.db, sb, runtimeReservationSize(sb)); err != nil {
			httpx.JSON(w, http.StatusServiceUnavailable, api.ErrorResponse{Error: err.Error()})
			return
		}
	}
	if err := s.client.Post(r.Context(), runner.APIURL, "/v1/sandboxes/"+sb.ID+"/"+action, nil, nil); err != nil {
		if nextStatus == model.SandboxStatusRunning && !runtimeReservationHeld(sb.Status) {
			_ = releaseRunnerReservationForSandbox(context.WithoutCancel(r.Context()), s.db, sb, runtimeReservationSize(sb))
		}
		httpx.JSON(w, http.StatusBadGateway, api.ErrorResponse{Error: err.Error()})
		return
	}
	now := time.Now().UTC()
	updates := map[string]any{
		"status":           nextStatus,
		"route_generation": gorm.Expr("route_generation + 1"),
	}
	if nextStatus == model.SandboxStatusStopped {
		updates["stopped_at"] = now
		updates["sleep_after_at"] = nil
	} else if nextStatus == model.SandboxStatusRunning {
		updates["stopped_at"] = nil
		updates["sleep_after_at"] = nextSleepAfter(sb, now)
		updates["last_wake_at"] = now
	}
	err = s.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		if nextStatus == model.SandboxStatusStopped && runtimeReservationHeld(sb.Status) {
			if err := releaseRunnerReservationForSandboxTx(tx, sb, runtimeReservationSize(sb)); err != nil {
				return err
			}
		}
		return tx.Model(&sb).Updates(updates).Error
	})
	if err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to update sandbox status"})
		return
	}
	sb.Status = nextStatus
	sb.RouteGeneration++
	if nextStatus == model.SandboxStatusRunning {
		sb.StoppedAt = nil
		sb.LastWakeAt = &now
		sb.SleepAfterAt = nextSleepAfter(sb, now)
	} else if nextStatus == model.SandboxStatusStopped {
		sb.StoppedAt = &now
		sb.SleepAfterAt = nil
	}
	var ports []model.SandboxPort
	if err := s.db.Order("guest_port asc").Find(&ports, "sandbox_id = ?", sb.ID).Error; err == nil {
		s.syncPreviewRoute(r.Context(), sb, runner, ports)
	}
	s.syncSandboxAliasRoutes(r.Context(), sb.ID)
	httpx.JSON(w, http.StatusOK, map[string]string{"status": nextStatus})
}

func (s *Server) stopSandboxLifecycle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sandboxID := chi.URLParam(r, "sandboxID")
	localUnlock := s.lifecycleLocks.Lock(sandboxID)
	defer localUnlock()
	// Stop fan-out deliberately does not hold a PostgreSQL advisory-lock
	// connection across the runner network call. This compare-and-set is the
	// distributed claim: exactly one control-plane replica transitions a running
	// sandbox to stopping, while duplicates receive an in-progress response.
	claim := s.db.WithContext(ctx).Model(&model.Sandbox{}).
		Where("id = ? AND status = ?", sandboxID, model.SandboxStatusRunning).
		Updates(map[string]any{
			"status":           model.SandboxStatusStopping,
			"route_generation": gorm.Expr("route_generation + 1"),
		})
	if claim.Error != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to prepare sandbox stop"})
		return
	}
	if claim.RowsAffected == 0 {
		var current model.Sandbox
		if err := s.db.WithContext(ctx).First(&current, "id = ?", sandboxID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				httpx.JSON(w, http.StatusNotFound, api.ErrorResponse{Error: "sandbox not found"})
				return
			}
			logging.FromContext(ctx).ErrorContext(ctx, "load sandbox after duplicate stop claim failed", "sandbox_id", sandboxID, "error", err)
			httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to load sandbox state"})
			return
		}
		switch current.Status {
		case model.SandboxStatusStopped:
			httpx.JSON(w, http.StatusOK, map[string]string{"status": current.Status})
		case model.SandboxStatusStopping:
			httpx.JSON(w, http.StatusConflict, api.ErrorResponse{Error: "sandbox stop is already in progress"})
		default:
			httpx.JSON(w, http.StatusConflict, api.ErrorResponse{Error: "sandbox cannot be stopped from status " + current.Status})
		}
		return
	}

	var sb model.Sandbox
	if err := s.db.WithContext(ctx).First(&sb, "id = ?", sandboxID).Error; err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to load sandbox after stop claim"})
		return
	}
	var runner model.Runner
	if err := s.db.WithContext(ctx).First(&runner, "id = ?", sb.RunnerID).Error; err != nil {
		httpx.JSON(w, http.StatusNotFound, api.ErrorResponse{Error: "runner not found"})
		return
	}

	var ports []model.SandboxPort
	if err := s.db.WithContext(ctx).Order("guest_port asc").Find(&ports, "sandbox_id = ?", sb.ID).Error; err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to load sandbox ports"})
		return
	}
	if err := s.publishPreviewRoute(ctx, sb, runner, ports); err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "preview route invalidation failed", "sandbox_id", sb.ID, "error", err)
		httpx.JSON(w, http.StatusServiceUnavailable, api.ErrorResponse{Error: "failed to invalidate preview route"})
		return
	}
	s.syncSandboxAliasRoutes(ctx, sb.ID)

	if err := s.client.Post(ctx, runner.APIURL, "/v1/sandboxes/"+sb.ID+"/stop", nil, nil); err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "runner stop failed", "sandbox_id", sb.ID, "runner_id", runner.ID, "error", err)
		httpx.JSON(w, http.StatusBadGateway, api.ErrorResponse{Error: "runner stop failed"})
		return
	}

	now := time.Now().UTC()
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if runtimeReservationHeld(sb.Status) {
			if err := releaseRunnerReservationForSandboxTx(tx, sb, runtimeReservationSize(sb)); err != nil {
				return err
			}
		}
		return tx.Model(&sb).Updates(map[string]any{
			"status":           model.SandboxStatusStopped,
			"stopped_at":       now,
			"sleep_after_at":   nil,
			"route_generation": gorm.Expr("route_generation + 1"),
		}).Error
	}); err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to update sandbox status"})
		return
	}
	sb.Status = model.SandboxStatusStopped
	sb.StoppedAt = &now
	sb.SleepAfterAt = nil
	sb.RouteGeneration++
	// The stopping tombstone is already authoritative. Final-state publication
	// is best effort and cannot reopen the route if it fails.
	s.syncPreviewRoute(ctx, sb, runner, ports)
	s.syncSandboxAliasRoutes(ctx, sb.ID)
	httpx.JSON(w, http.StatusOK, map[string]string{"status": model.SandboxStatusStopped})
}

func (s *Server) deleteSandbox(w http.ResponseWriter, r *http.Request) {
	sandboxID := chi.URLParam(r, "sandboxID")
	unlock := s.lifecycleLocks.Lock(sandboxID)
	defer unlock()
	distributedUnlock, err := s.acquireDistributedSandboxLock(r.Context(), sandboxID)
	if err != nil {
		httpx.JSON(w, http.StatusServiceUnavailable, api.ErrorResponse{Error: err.Error()})
		return
	}
	defer distributedUnlock()

	sb, runner, ok := s.loadSandboxRunner(w, r)
	if !ok {
		return
	}
	_ = s.client.Post(r.Context(), runner.APIURL, "/v1/sandboxes/"+sb.ID+"/delete", nil, nil)
	_ = s.db.Transaction(func(tx *gorm.DB) error {
		releaseSize := api.Size{}
		if runtimeReservationHeld(sb.Status) {
			releaseSize.CPU = sb.CPU
			releaseSize.MemoryMB = sb.MemoryMB
		}
		if diskReservationHeld(sb.Status) {
			releaseSize.DiskGB = sb.DiskGB
		}
		_ = releaseRunnerReservationForSandboxTx(tx, sb, releaseSize)
		_ = tx.Where("sandbox_id = ?", sb.ID).Delete(&model.SandboxPort{}).Error
		return tx.Delete(&sb).Error
	})
	s.deletePreviewRoute(r.Context(), sb.ID)
	s.deleteSandboxAliasRoutes(r.Context(), sb.ID)
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
