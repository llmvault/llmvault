package control

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/microsandbox/api"
	"github.com/usehivy/hivy/internal/microsandbox/httpx"
	"github.com/usehivy/hivy/internal/microsandbox/model"
	"github.com/usehivy/hivy/internal/microsandbox/security"
)

const (
	defaultEnsureTimeout = 90 * time.Second
)

type sandboxRouteResponse struct {
	Route previewCacheRoute `json:"route"`
}

type ensureReadyRequest struct {
	GuestPort      int    `json:"guest_port"`
	Readiness      string `json:"readiness"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	RequestID      string `json:"request_id"`
}

type runnerEnsureReadyRequest struct {
	GuestPort      int `json:"guest_port"`
	TimeoutSeconds int `json:"timeout_seconds"`
}

type runnerEnsureReadyResponse struct {
	Status   string `json:"status"`
	HostPort int    `json:"host_port"`
}

type sandboxActivityRequest struct {
	Source      string `json:"source"`
	RuntimeBusy *bool  `json:"runtime_busy,omitempty"`
}

type sandboxActivityBulkRequest struct {
	Source      string   `json:"source"`
	SandboxIDs  []string `json:"sandbox_ids"`
	RuntimeBusy *bool    `json:"runtime_busy,omitempty"`
}

type sandboxActivityBulkResponse struct {
	Routes []previewCacheRoute `json:"routes"`
}

type sandboxPolicyRequest struct {
	AutoSleepAfterSeconds *int `json:"auto_sleep_after_seconds"`
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
	if strings.TrimSpace(req.Readiness) != "" {
		logging.FromContext(r.Context()).WarnContext(r.Context(), "deprecated ensure-ready readiness ignored",
			"sandbox_id", chi.URLParam(r, "sandboxID"), "readiness", req.Readiness)
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

func (s *Server) sandboxActivity(w http.ResponseWriter, r *http.Request) {
	var sb model.Sandbox
	if err := s.db.WithContext(r.Context()).First(&sb, "id = ?", chi.URLParam(r, "sandboxID")).Error; err != nil {
		httpx.JSON(w, http.StatusNotFound, api.ErrorResponse{Error: "sandbox not found"})
		return
	}
	if !s.authorizedSandboxActivity(r, sb) {
		httpx.JSON(w, http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized"})
		return
	}

	var req sandboxActivityRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.JSON(w, http.StatusBadRequest, api.ErrorResponse{Error: "invalid request body"})
		return
	}
	if _, err := s.recordSandboxActivity(r.Context(), &sb, req.Source, req.RuntimeBusy); err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to record activity"})
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"status": "ok", "sandbox_id": sb.ID})
}

func (s *Server) sandboxActivityBulk(w http.ResponseWriter, r *http.Request) {
	if s.cfg.APIToken == "" || httpx.Bearer(r) != s.cfg.APIToken {
		httpx.JSON(w, http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized"})
		return
	}
	var req sandboxActivityBulkRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.JSON(w, http.StatusBadRequest, api.ErrorResponse{Error: "invalid request body"})
		return
	}
	seen := map[string]bool{}
	routes := make([]previewCacheRoute, 0, len(req.SandboxIDs))
	for _, sandboxID := range req.SandboxIDs {
		sandboxID = strings.TrimSpace(sandboxID)
		if sandboxID == "" || seen[sandboxID] {
			continue
		}
		seen[sandboxID] = true
		var sb model.Sandbox
		if err := s.db.WithContext(r.Context()).First(&sb, "id = ?", sandboxID).Error; err != nil {
			continue
		}
		route, err := s.recordSandboxActivity(r.Context(), &sb, req.Source, req.RuntimeBusy)
		if err != nil {
			logging.FromContext(r.Context()).WarnContext(r.Context(), "bulk activity update failed", "sandbox_id", sandboxID, "error", err)
			continue
		}
		routes = append(routes, route)
	}
	httpx.JSON(w, http.StatusOK, sandboxActivityBulkResponse{Routes: routes})
}

func (s *Server) recordSandboxActivity(ctx context.Context, sb *model.Sandbox, source string, runtimeBusy *bool) (previewCacheRoute, error) {
	now := time.Now().UTC()
	updates := map[string]any{
		"sleep_after_at": nextSleepAfter(*sb, now),
	}
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "runtime":
		updates["last_runtime_activity_at"] = now
		if runtimeBusy != nil {
			updates["runtime_busy"] = *runtimeBusy
		}
		sb.LastRuntimeActivityAt = &now
		if runtimeBusy != nil {
			sb.RuntimeBusy = *runtimeBusy
		}
	default:
		updates["last_gateway_activity_at"] = now
		sb.LastGatewayActivityAt = &now
	}
	if err := s.db.WithContext(ctx).Model(sb).Updates(updates).Error; err != nil {
		return previewCacheRoute{}, err
	}
	sb.SleepAfterAt = nextSleepAfter(*sb, now)
	var runner model.Runner
	if err := s.db.WithContext(ctx).First(&runner, "id = ?", sb.RunnerID).Error; err != nil {
		return previewCacheRoute{}, err
	}
	return s.routeForSandbox(ctx, *sb, runner)
}

func (s *Server) updateSandboxPolicy(w http.ResponseWriter, r *http.Request) {
	var req sandboxPolicyRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.JSON(w, http.StatusBadRequest, api.ErrorResponse{Error: "invalid request body"})
		return
	}
	if req.AutoSleepAfterSeconds == nil || *req.AutoSleepAfterSeconds < 0 {
		httpx.JSON(w, http.StatusBadRequest, api.ErrorResponse{Error: "auto_sleep_after_seconds must be non-negative"})
		return
	}
	sb, _, ok := s.loadSandboxRunner(w, r)
	if !ok {
		return
	}
	sb.AutoSleepAfterSeconds = *req.AutoSleepAfterSeconds
	sleepAfterAt := nextSleepAfter(sb, time.Now().UTC())
	if err := s.db.WithContext(r.Context()).Model(&sb).Updates(map[string]any{
		"auto_sleep_after_seconds": *req.AutoSleepAfterSeconds,
		"sleep_after_at":           sleepAfterAt,
		"route_generation":         gorm.Expr("route_generation + 1"),
	}).Error; err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to update policy"})
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"sandbox_id":                 sb.ID,
		"auto_sleep_after_seconds":   *req.AutoSleepAfterSeconds,
		"auto_sleep_after_humanized": strconv.Itoa(*req.AutoSleepAfterSeconds) + "s",
	})
}

func (s *Server) authorizedSandboxActivity(r *http.Request, sb model.Sandbox) bool {
	token := httpx.Bearer(r)
	if token == "" {
		return false
	}
	if s.cfg.APIToken != "" && security.ConstantTimeStringEqual(token, s.cfg.APIToken) {
		return true
	}
	return sb.ActivityToken != "" && security.ConstantTimeStringEqual(token, sb.ActivityToken)
}

func (s *Server) routeForSandbox(ctx context.Context, sb model.Sandbox, runner model.Runner) (previewCacheRoute, error) {
	ports := routePorts(ctx, s.db, sb.ID)
	if len(ports) == 0 {
		return previewCacheRoute{}, fmt.Errorf("sandbox %s has no ports", sb.ID)
	}
	return previewCacheRouteFor(sb, runner, ports)
}

func routePorts(ctx context.Context, db *gorm.DB, sandboxID string) []model.SandboxPort {
	var ports []model.SandboxPort
	if err := db.WithContext(ctx).Order("guest_port asc").Find(&ports, "sandbox_id = ?", sandboxID).Error; err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "load sandbox ports failed", "sandbox_id", sandboxID, "error", err)
	}
	return ports
}
