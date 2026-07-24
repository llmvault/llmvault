package control

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/usehivy/hivy/internal/microsandbox/api"
	"github.com/usehivy/hivy/internal/microsandbox/httpx"
	"github.com/usehivy/hivy/internal/microsandbox/model"
)

func (s *Server) listSandboxes(w http.ResponseWriter, r *http.Request) {
	var sandboxes []model.Sandbox
	if err := s.db.Order("created_at desc").Find(&sandboxes).Error; err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to list sandboxes"})
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": sandboxes})
}

// sandboxState is the lean per-sandbox projection the reconciler pulls in bulk
// (no ports/env/secrets) so the whole fleet fits one cheap query.
type sandboxState struct {
	ID                    string     `json:"id"`
	Status                string     `json:"status"`
	SleepAfterAt          *time.Time `json:"sleep_after_at"`
	LastGatewayActivityAt *time.Time `json:"last_gateway_activity_at"`
	RuntimeBusy           bool       `json:"runtime_busy"`
	LastRuntimeActivityAt *time.Time `json:"last_runtime_activity_at"`
}

// listSandboxStates returns every sandbox's liveness state in one batch for the
// Go-API reconciler.
func (s *Server) listSandboxStates(w http.ResponseWriter, r *http.Request) {
	var states []sandboxState
	if err := s.db.WithContext(r.Context()).Model(&model.Sandbox{}).
		Select("id, status, sleep_after_at, last_gateway_activity_at, runtime_busy, last_runtime_activity_at").
		Find(&states).Error; err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to list sandbox states"})
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": states})
}

func (s *Server) getSandbox(w http.ResponseWriter, r *http.Request) {
	var sb model.Sandbox
	if err := s.db.First(&sb, "id = ?", chi.URLParam(r, "sandboxID")).Error; err != nil {
		httpx.JSON(w, http.StatusNotFound, api.ErrorResponse{Error: "sandbox not found"})
		return
	}
	var ports []model.SandboxPort
	if err := s.db.Order("guest_port asc").Find(&ports, "sandbox_id = ?", sb.ID).Error; err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to load sandbox ports"})
		return
	}
	httpx.JSON(w, http.StatusOK, sandboxResponse{Sandbox: sb, Ports: ports, PreviewURLs: s.previewURLs(sb.ID, ports)})
}

type runtimeEndpointRequest struct {
	Port int `json:"port"`
}

func (s *Server) createRuntimeEndpoint(w http.ResponseWriter, r *http.Request) {
	var req runtimeEndpointRequest
	if err := httpx.Decode(r, &req); err != nil || req.Port <= 0 || req.Port > 65535 {
		httpx.JSON(w, http.StatusBadRequest, api.ErrorResponse{Error: "valid port is required"})
		return
	}
	var sb model.Sandbox
	if err := s.db.First(&sb, "id = ?", chi.URLParam(r, "sandboxID")).Error; err != nil {
		httpx.JSON(w, http.StatusNotFound, api.ErrorResponse{Error: "sandbox not found"})
		return
	}
	runtimeURL := fmt.Sprintf("https://%d-%s.%s", req.Port, sb.ID, s.cfg.PreviewBaseDomain)
	httpx.JSON(w, http.StatusOK, map[string]string{"url": runtimeURL})
}
