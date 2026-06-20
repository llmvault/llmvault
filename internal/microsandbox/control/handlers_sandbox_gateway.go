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
	readinessPortOpen     = "port_open"
	readinessRuntimeReady = "runtime_ready"
	defaultEnsureTimeout  = 90 * time.Second
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
	GuestPort      int    `json:"guest_port"`
	Readiness      string `json:"readiness"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	ProbeToken     string `json:"probe_token,omitempty"`
}

type runnerEnsureReadyResponse struct {
	Status    string `json:"status"`
	HostPort  int    `json:"host_port"`
	Readiness string `json:"readiness"`
}

type sandboxActivityRequest struct {
	Source      string `json:"source"`
	RuntimeBusy *bool  `json:"runtime_busy,omitempty"`
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
	if req.Readiness == "" {
		req.Readiness = readinessPortOpen
	}
	if req.Readiness != readinessPortOpen && req.Readiness != readinessRuntimeReady {
		httpx.JSON(w, http.StatusBadRequest, api.ErrorResponse{Error: "readiness must be port_open or runtime_ready"})
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
	err := s.client.Post(ctx, runner.APIURL, "/v1/sandboxes/"+sb.ID+"/ensure-ready", runnerEnsureReadyRequest{
		GuestPort:      req.GuestPort,
		Readiness:      req.Readiness,
		TimeoutSeconds: int(timeout.Seconds()),
		ProbeToken:     sb.InfraProbeToken,
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

	if err := s.db.WithContext(ctx).Model(&sb).Updates(map[string]any{
		"status":                   model.SandboxStatusRunning,
		"stopped_at":               nil,
		"last_gateway_activity_at": now,
		"last_wake_at":             now,
		"last_wake_error":          "",
	}).Error; err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to update sandbox status"})
		return
	}
	sb.Status = model.SandboxStatusRunning
	sb.StoppedAt = nil
	sb.LastGatewayActivityAt = &now
	sb.LastWakeAt = &now
	sb.LastWakeError = ""

	route, err := s.routeForSandbox(ctx, sb, runner)
	if err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: err.Error()})
		return
	}
	s.syncPreviewRoute(ctx, sb, runner, routePorts(ctx, s.db, sb.ID))
	httpx.JSON(w, http.StatusOK, map[string]any{
		"status":    out.Status,
		"readiness": out.Readiness,
		"route":     route,
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
	now := time.Now().UTC()
	updates := map[string]any{}
	switch strings.ToLower(strings.TrimSpace(req.Source)) {
	case "runtime":
		updates["last_runtime_activity_at"] = now
		if req.RuntimeBusy != nil {
			updates["runtime_busy"] = *req.RuntimeBusy
		}
	default:
		updates["last_gateway_activity_at"] = now
	}
	if len(updates) == 0 {
		updates["last_gateway_activity_at"] = now
	}
	if err := s.db.WithContext(r.Context()).Model(&sb).Updates(updates).Error; err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to record activity"})
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"status": "ok", "sandbox_id": sb.ID})
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
	if err := s.db.WithContext(r.Context()).Model(&sb).Update("auto_sleep_after_seconds", *req.AutoSleepAfterSeconds).Error; err != nil {
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
	return sb.InfraProbeToken != "" && security.ConstantTimeStringEqual(token, sb.InfraProbeToken)
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
