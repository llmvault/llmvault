package control

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/microsandbox/api"
	"github.com/usehivy/hivy/internal/microsandbox/httpx"
	"github.com/usehivy/hivy/internal/microsandbox/model"
)

const (
	defaultEnsureTimeout = 90 * time.Second
)

type sandboxRouteResponse struct {
	Route previewCacheRoute `json:"route"`
}

type ensureReadyRequest struct {
	GuestPort      int    `json:"guest_port"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	RequestID      string `json:"request_id"`
}

type runnerEnsureReadyRequest struct {
	GuestPort      int                       `json:"guest_port"`
	TimeoutSeconds int                       `json:"timeout_seconds"`
	HealthCheck    *runnerHealthCheckRequest `json:"health_check,omitempty"`
}

type runnerEnsureReadyResponse struct {
	Status   string `json:"status"`
	HostPort int    `json:"host_port"`
}

func (s *Server) getSandboxRoute(w http.ResponseWriter, r *http.Request) {
	sb, runner, ok := s.loadSandboxRunner(w, r)
	if !ok {
		return
	}
	route, err := s.routeForSandbox(r.Context(), sb, runner)
	if err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: err.Error()})
		return
	}
	httpx.JSON(w, http.StatusOK, sandboxRouteResponse{Route: route})
}

func (s *Server) ensureSandboxReady(w http.ResponseWriter, r *http.Request) {
	var req ensureReadyRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.JSON(w, http.StatusBadRequest, api.ErrorResponse{Error: "invalid request body"})
		return
	}
	if req.GuestPort <= 0 || req.GuestPort > 65535 {
		httpx.JSON(w, http.StatusBadRequest, api.ErrorResponse{Error: "valid guest_port is required"})
		return
	}
	timeout := defaultEnsureTimeout
	if req.TimeoutSeconds > 0 {
		timeout = time.Duration(req.TimeoutSeconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout+5*time.Second)
	defer cancel()

	sandboxID := chi.URLParam(r, "sandboxID")
	unlock := s.lifecycleLocks.Lock(sandboxID)
	defer unlock()
	distributedUnlock, err := s.acquireDistributedSandboxLock(ctx, sandboxID)
	if err != nil {
		httpx.JSON(w, http.StatusServiceUnavailable, api.ErrorResponse{Error: err.Error()})
		return
	}
	defer distributedUnlock()

	sb, runner, ok := s.loadSandboxRunner(w, r.WithContext(ctx))
	if !ok {
		return
	}
	var port model.SandboxPort
	if err := s.db.WithContext(ctx).
		Where("sandbox_id = ? AND guest_port = ?", sb.ID, req.GuestPort).
		First(&port).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			httpx.JSON(w, http.StatusNotFound, api.ErrorResponse{Error: "port not previewable"})
			return
		}
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to load sandbox port"})
		return
	}
	reservedRuntime := false
	if !runtimeReservationHeld(sb.Status) {
		if err := reserveRunnerReservationForSandbox(ctx, s.db, sb, runtimeReservationSize(sb)); err != nil {
			httpx.JSON(w, http.StatusServiceUnavailable, api.ErrorResponse{Error: err.Error()})
			return
		}
		reservedRuntime = true
	}
	var out runnerEnsureReadyResponse
	err = s.client.Post(ctx, runner.APIURL, "/v1/sandboxes/"+sb.ID+"/ensure-ready", runnerEnsureReadyRequest{
		GuestPort:      req.GuestPort,
		TimeoutSeconds: int(timeout.Seconds()),
		HealthCheck:    runnerHealthCheckForPort(port),
	}, &out)
	now := time.Now().UTC()
	if err != nil {
		if reservedRuntime {
			_ = releaseRunnerReservationForSandbox(context.WithoutCancel(r.Context()), s.db, sb, runtimeReservationSize(sb))
		}
		_ = s.db.WithContext(context.WithoutCancel(r.Context())).Model(&sb).Updates(map[string]any{
			"last_wake_error": err.Error(),
		}).Error
		httpx.JSON(w, http.StatusBadGateway, api.ErrorResponse{Error: err.Error()})
		return
	}

	sleepAfterAt := nextSleepAfter(sb, now)
	if err := s.db.WithContext(ctx).Model(&sb).Updates(map[string]any{
		"status":                   model.SandboxStatusRunning,
		"stopped_at":               nil,
		"last_gateway_activity_at": now,
		"last_wake_at":             now,
		"last_wake_error":          "",
		"sleep_after_at":           sleepAfterAt,
		"route_generation":         gorm.Expr("route_generation + 1"),
	}).Error; err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to update sandbox status"})
		return
	}
	sb.Status = model.SandboxStatusRunning
	sb.StoppedAt = nil
	sb.LastGatewayActivityAt = &now
	sb.LastWakeAt = &now
	sb.LastWakeError = ""
	sb.SleepAfterAt = sleepAfterAt
	sb.RouteGeneration++

	route, err := s.routeForSandbox(ctx, sb, runner)
	if err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: err.Error()})
		return
	}
	s.syncPreviewRoute(ctx, sb, runner, routePorts(ctx, s.db, sb.ID))
	httpx.JSON(w, http.StatusOK, map[string]any{
		"status": out.Status,
		"route":  route,
	})
}
