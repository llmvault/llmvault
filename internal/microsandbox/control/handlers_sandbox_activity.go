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
