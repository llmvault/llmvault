package control

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/usehivy/hivy/internal/microsandbox/api"
	"github.com/usehivy/hivy/internal/microsandbox/httpx"
	"github.com/usehivy/hivy/internal/microsandbox/model"
)

type upgradeSandboxRequest struct {
	Name         string                      `json:"name"`
	ImageRef     string                      `json:"image_ref"`
	TemplateID   string                      `json:"template_id"`
	CPU          int                         `json:"cpu"`
	MemoryMB     int                         `json:"memory_mb"`
	DiskGB       int                         `json:"disk_gb"`
	PreviewPorts []int                       `json:"preview_ports"`
	HealthChecks []sandboxHealthCheckRequest `json:"health_checks"`
	Init         *sandboxInitConfig          `json:"init"`
	Env          map[string]string           `json:"env"`
	Metadata     map[string]any              `json:"metadata"`
}

type runnerUpgradeSandboxRequest struct {
	Name             string              `json:"name"`
	ImageRef         string              `json:"image_ref"`
	PreviousImageRef string              `json:"previous_image_ref"`
	CPU              int                 `json:"cpu"`
	MemoryMB         int                 `json:"memory_mb"`
	DiskGB           int                 `json:"disk_gb"`
	Env              map[string]string   `json:"env"`
	Labels           map[string]string   `json:"labels"`
	PreviewPorts     []int               `json:"preview_ports"`
	PortBindings     []runnerPortBinding `json:"port_bindings"`
	Init             *sandboxInitConfig  `json:"init"`
}

type runnerPortBinding struct {
	GuestPort int `json:"guest_port"`
	HostPort  int `json:"host_port"`
}

type runnerUpgradeSandboxResponse struct {
	ID     string              `json:"id"`
	Status string              `json:"status"`
	Error  string              `json:"error,omitempty"`
	Ports  []runnerPortBinding `json:"ports"`
}

type sandboxResources struct {
	CPU      int
	MemoryMB int
	DiskGB   int
}

func (s *Server) upgradeSandbox(w http.ResponseWriter, r *http.Request) {
	var req upgradeSandboxRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.JSON(w, http.StatusBadRequest, api.ErrorResponse{Error: "invalid request body"})
		return
	}
	if req.ImageRef == "" && req.TemplateID == "" {
		httpx.JSON(w, http.StatusBadRequest, api.ErrorResponse{Error: "image_ref or template_id is required"})
		return
	}

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
	ports := routePorts(r.Context(), s.db, sb.ID)
	if len(ports) == 0 {
		httpx.JSON(w, http.StatusConflict, api.ErrorResponse{Error: "sandbox has no port bindings"})
		return
	}
	previewPorts := requestedUpgradePorts(req.PreviewPorts, ports)
	if err := validatePortSetUnchanged(previewPorts, ports); err != nil {
		httpx.JSON(w, http.StatusBadRequest, api.ErrorResponse{Error: err.Error()})
		return
	}
	healthChecks, err := validateSandboxHealthChecks(req.HealthChecks, previewPorts)
	if err != nil {
		httpx.JSON(w, http.StatusBadRequest, api.ErrorResponse{Error: err.Error()})
		return
	}
	if req.TemplateID != "" {
		template, err := s.loadTemplateByID(r.Context(), req.TemplateID)
		if err != nil {
			httpx.JSON(w, http.StatusNotFound, api.ErrorResponse{Error: "template not found"})
			return
		}
		if template.OrgID != sb.OrgID {
			httpx.JSON(w, http.StatusForbidden, api.ErrorResponse{Error: "template does not belong to org"})
			return
		}
		if template.Status != model.TemplateStatusReady || template.ImageRef == "" {
			httpx.JSON(w, http.StatusConflict, api.ErrorResponse{Error: "template is not ready"})
			return
		}
		req.ImageRef = template.ImageRef
	}
	currentResources := sandboxResources{CPU: sb.CPU, MemoryMB: sb.MemoryMB, DiskGB: sb.DiskGB}
	nextResources := requestedUpgradeResources(req, sb)
	if err := s.adjustUpgradeResources(r.Context(), sb, currentResources, nextResources); err != nil {
		httpx.JSON(w, http.StatusServiceUnavailable, api.ErrorResponse{Error: err.Error()})
		return
	}
	reservedNext := true
	defer func(ctx context.Context) {
		if reservedNext {
			_ = s.adjustUpgradeResources(context.WithoutCancel(ctx), sb, nextResources, currentResources)
		}
	}(r.Context())

	if err := s.markSandboxUpgrading(r.Context(), &sb); err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to mark sandbox upgrading"})
		return
	}
	prepareUpgradeEnv(s.cfg.ControlURL, sb, &req)

	var out runnerUpgradeSandboxResponse
	err = s.client.Post(r.Context(), runner.APIURL, "/v1/sandboxes/"+sb.ID+"/upgrade", runnerUpgradeSandboxRequest{
		Name:             req.Name,
		ImageRef:         req.ImageRef,
		PreviousImageRef: sb.ImageRef,
		CPU:              nextResources.CPU,
		MemoryMB:         nextResources.MemoryMB,
		DiskGB:           nextResources.DiskGB,
		Env:              req.Env,
		Labels:           runnerUpgradeLabels(sb, req.Metadata),
		PreviewPorts:     previewPorts,
		PortBindings:     runnerBindingsForPorts(ports),
		Init:             req.Init,
	}, &out)
	if err != nil {
		_ = s.markSandboxUpgradeFailed(context.WithoutCancel(r.Context()), &sb, err.Error())
		httpx.JSON(w, http.StatusBadGateway, api.ErrorResponse{Error: err.Error()})
		return
	}
	if out.Status == upgradeStatusRolledBack {
		_ = s.markSandboxUpgradeRolledBack(context.WithoutCancel(r.Context()), &sb, out.Error)
		httpx.JSON(w, http.StatusConflict, api.ErrorResponse{Error: "runner rolled back upgrade: " + out.Error})
		return
	}

	if err := s.finishSandboxUpgrade(r.Context(), &sb, req, nextResources, ports, healthChecks); err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to finish sandbox upgrade"})
		return
	}
	reservedNext = false
	s.syncPreviewRoute(r.Context(), sb, runner, ports)
	httpx.JSON(w, http.StatusOK, sandboxResponse{Sandbox: sb, Ports: ports, PreviewURLs: s.previewURLs(sb.ID, ports)})
}
