package control

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/microsandbox/api"
	"github.com/usehivy/hivy/internal/microsandbox/httpx"
	"github.com/usehivy/hivy/internal/microsandbox/model"
	"github.com/usehivy/hivy/internal/microsandbox/security"
)

type createSandboxRequest struct {
	OrgID           string            `json:"org_id"`
	Name            string            `json:"name"`
	ImageRef        string            `json:"image_ref"`
	SnapshotID      string            `json:"snapshot_id"`
	Size            string            `json:"size"`
	CPU             int               `json:"cpu"`
	MemoryMB        int               `json:"memory_mb"`
	DiskGB          int               `json:"disk_gb"`
	PreviewPorts    []int             `json:"preview_ports"`
	PreviewPassword string            `json:"preview_password"`
	Env             map[string]string `json:"env"`
	Metadata        map[string]any    `json:"metadata"`
}

type sandboxResponse struct {
	model.Sandbox
	PreviewPassword string              `json:"preview_password,omitempty"`
	PreviewURLs     map[string]string   `json:"preview_urls,omitempty"`
	Ports           []model.SandboxPort `json:"ports,omitempty"`
}

func (s *Server) createSandbox(w http.ResponseWriter, r *http.Request) {
	var req createSandboxRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.JSON(w, http.StatusBadRequest, api.ErrorResponse{Error: "invalid request body"})
		return
	}
	if req.OrgID == "" || req.ImageRef == "" {
		httpx.JSON(w, http.StatusBadRequest, api.ErrorResponse{Error: "org_id and image_ref are required"})
		return
	}
	size := resolveSize(req)
	id, err := security.ShortID("sbx")
	if err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to allocate sandbox id"})
		return
	}
	if req.Name == "" {
		req.Name = id
	}
	if len(req.PreviewPorts) == 0 {
		req.PreviewPorts = api.DefaultPreviewPorts()
	}
	metadata, _ := json.Marshal(req.Metadata)

	password, err := s.ensureOrgPassword(r.Context(), req.OrgID, req.PreviewPassword)
	if err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to prepare preview password"})
		return
	}

	var sb model.Sandbox
	var runner model.Runner
	err = s.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		selected, err := selectRunnerForUpdate(tx, size)
		if err != nil {
			return err
		}
		runner = selected
		if err := reserveRunner(tx, &runner, size); err != nil {
			return err
		}
		sb = model.Sandbox{
			ID: id, OrgID: req.OrgID, RunnerID: runner.ID, Name: req.Name, ImageRef: req.ImageRef,
			SnapshotID: req.SnapshotID, Status: model.SandboxStatusCreating, CPU: size.CPU,
			MemoryMB: size.MemoryMB, DiskGB: size.DiskGB, MetadataJSON: string(metadata),
		}
		return tx.Create(&sb).Error
	})
	if err != nil {
		httpx.JSON(w, http.StatusServiceUnavailable, api.ErrorResponse{Error: err.Error()})
		return
	}

	var createResp runnerCreateSandboxResponse
	err = s.client.Post(r.Context(), runner.APIURL, "/v1/sandboxes", runnerCreateSandboxRequest{
		ID: sb.ID, Name: sb.Name, ImageRef: sb.ImageRef, SnapshotID: sb.SnapshotID,
		CPU: sb.CPU, MemoryMB: sb.MemoryMB, DiskGB: sb.DiskGB, Env: req.Env,
		PreviewPorts: req.PreviewPorts,
		Labels:       map[string]string{"org_id": sb.OrgID, "sandbox_id": sb.ID},
	}, &createResp)
	if err != nil {
		_ = s.db.Transaction(func(tx *gorm.DB) error {
			_ = releaseRunner(tx, &runner, size)
			return tx.Model(&sb).Updates(map[string]any{"status": model.SandboxStatusError, "error_message": err.Error()}).Error
		})
		httpx.JSON(w, http.StatusBadGateway, api.ErrorResponse{Error: "runner create failed: " + err.Error()})
		return
	}
	ports := make([]model.SandboxPort, 0, len(createResp.Ports))
	for _, p := range createResp.Ports {
		ports = append(ports, model.SandboxPort{
			ID: uuid.NewString(), SandboxID: sb.ID, GuestPort: p.GuestPort, HostPort: p.HostPort, Protocol: "http",
		})
	}
	_ = s.db.Transaction(func(tx *gorm.DB) error {
		if len(ports) > 0 {
			if err := tx.Create(&ports).Error; err != nil {
				return err
			}
		}
		return tx.Model(&sb).Update("status", model.SandboxStatusRunning).Error
	})
	sb.Status = model.SandboxStatusRunning
	s.syncPreviewRoute(r.Context(), sb, runner, ports)
	resp := sandboxResponse{Sandbox: sb, Ports: ports, PreviewURLs: s.previewURLs(sb.ID, ports)}
	resp.PreviewPassword = password
	httpx.JSON(w, http.StatusCreated, resp)
}

func (s *Server) listSandboxes(w http.ResponseWriter, r *http.Request) {
	var sandboxes []model.Sandbox
	if err := s.db.Order("created_at desc").Find(&sandboxes).Error; err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to list sandboxes"})
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": sandboxes})
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
	Port       int `json:"port"`
	TTLSeconds int `json:"ttl_seconds"`
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
	ttl := time.Hour
	if req.TTLSeconds > 0 {
		ttl = time.Duration(req.TTLSeconds) * time.Second
	}
	token, err := s.signRuntimeToken(sb.ID, req.Port, ttl)
	if err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to sign runtime endpoint"})
		return
	}
	runtimeURL := fmt.Sprintf("https://%d-%s.%s?rt=%s", req.Port, sb.ID, s.cfg.PreviewBaseDomain, url.QueryEscape(token))
	httpx.JSON(w, http.StatusOK, map[string]string{"url": runtimeURL})
}

func (s *Server) startSandbox(w http.ResponseWriter, r *http.Request) {
	s.lifecycle(w, r, "start", model.SandboxStatusRunning)
}

func (s *Server) stopSandbox(w http.ResponseWriter, r *http.Request) {
	s.lifecycle(w, r, "stop", model.SandboxStatusStopped)
}

func (s *Server) lifecycle(w http.ResponseWriter, r *http.Request, action, nextStatus string) {
	sb, runner, ok := s.loadSandboxRunner(w, r)
	if !ok {
		return
	}
	if err := s.client.Post(r.Context(), runner.APIURL, "/v1/sandboxes/"+sb.ID+"/"+action, nil, nil); err != nil {
		httpx.JSON(w, http.StatusBadGateway, api.ErrorResponse{Error: err.Error()})
		return
	}
	updates := map[string]any{"status": nextStatus}
	if nextStatus == model.SandboxStatusStopped {
		updates["stopped_at"] = time.Now()
	} else if nextStatus == model.SandboxStatusRunning {
		updates["stopped_at"] = nil
	}
	s.db.Model(&sb).Updates(updates)
	sb.Status = nextStatus
	var ports []model.SandboxPort
	if err := s.db.Order("guest_port asc").Find(&ports, "sandbox_id = ?", sb.ID).Error; err == nil {
		s.syncPreviewRoute(r.Context(), sb, runner, ports)
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": nextStatus})
}

func (s *Server) deleteSandbox(w http.ResponseWriter, r *http.Request) {
	sb, runner, ok := s.loadSandboxRunner(w, r)
	if !ok {
		return
	}
	_ = s.client.Post(r.Context(), runner.APIURL, "/v1/sandboxes/"+sb.ID+"/delete", nil, nil)
	_ = s.db.Transaction(func(tx *gorm.DB) error {
		_ = releaseRunner(tx, &runner, api.Size{CPU: sb.CPU, MemoryMB: sb.MemoryMB, DiskGB: sb.DiskGB})
		_ = tx.Where("sandbox_id = ?", sb.ID).Delete(&model.SandboxPort{}).Error
		return tx.Delete(&sb).Error
	})
	s.deletePreviewRoute(r.Context(), sb.ID)
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

type execRequest struct {
	Command        string `json:"command"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

func (s *Server) execSandbox(w http.ResponseWriter, r *http.Request) {
	sb, runner, ok := s.loadSandboxRunner(w, r)
	if !ok {
		return
	}
	var req execRequest
	if err := httpx.Decode(r, &req); err != nil || req.Command == "" {
		httpx.JSON(w, http.StatusBadRequest, api.ErrorResponse{Error: "command is required"})
		return
	}
	var out map[string]any
	if err := s.client.Post(r.Context(), runner.APIURL, "/v1/sandboxes/"+sb.ID+"/exec", req, &out); err != nil {
		httpx.JSON(w, http.StatusBadGateway, api.ErrorResponse{Error: err.Error()})
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (s *Server) logsSandbox(w http.ResponseWriter, r *http.Request) {
	sb, runner, ok := s.loadSandboxRunner(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if err := s.client.Get(r.Context(), runner.APIURL, "/v1/sandboxes/"+sb.ID+"/logs", w); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
	}
}
