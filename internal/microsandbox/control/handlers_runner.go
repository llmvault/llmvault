package control

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/microsandbox/api"
	"github.com/usehivy/hivy/internal/microsandbox/httpx"
	"github.com/usehivy/hivy/internal/microsandbox/model"
	"github.com/usehivy/hivy/internal/microsandbox/security"
)

type registerRunnerRequest struct {
	Name           string         `json:"name"`
	APIURL         string         `json:"api_url"`
	PreviewBaseURL string         `json:"preview_base_url"`
	Capacity       runnerCapacity `json:"capacity"`
	Metadata       map[string]any `json:"metadata"`
}

type runnerCapacity struct {
	CPU              int     `json:"cpu"`
	MemoryMB         int     `json:"memory_mb"`
	DiskGB           int     `json:"disk_gb"`
	CPUOvercommit    float64 `json:"cpu_overcommit"`
	MemoryOvercommit float64 `json:"memory_overcommit"`
	DiskOvercommit   float64 `json:"disk_overcommit"`
}

func (s *Server) registerRunner(w http.ResponseWriter, r *http.Request) {
	if s.cfg.RunnerJoinSecret == "" || httpx.Bearer(r) != s.cfg.RunnerJoinSecret {
		httpx.JSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req registerRunnerRequest
	if err := httpx.Decode(r, &req); err != nil || req.Name == "" || req.APIURL == "" || req.PreviewBaseURL == "" {
		httpx.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid runner registration"})
		return
	}
	token, err := security.RandomToken(32)
	if err != nil {
		httpx.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create runner token"})
		return
	}
	now := time.Now()
	id := uuid.NewString()
	var existing model.Runner
	err = s.db.Where("name = ?", req.Name).First(&existing).Error
	if err == nil {
		id = existing.ID
	}
	cpuOvercommit := req.Capacity.CPUOvercommit
	if cpuOvercommit <= 0 {
		cpuOvercommit = defaultCPUOvercommit
	}
	memoryOvercommit := req.Capacity.MemoryOvercommit
	if memoryOvercommit <= 0 {
		memoryOvercommit = defaultMemoryOvercommit
	}
	diskOvercommit := req.Capacity.DiskOvercommit
	if diskOvercommit <= 0 {
		diskOvercommit = defaultDiskOvercommit
	}
	runner := model.Runner{
		ID: id, Name: req.Name, APIURL: req.APIURL, PreviewBaseURL: req.PreviewBaseURL, AuthTokenHash: security.HashToken(token),
		Status: model.RunnerStatusHealthy, TotalCPU: req.Capacity.CPU, TotalMemoryMB: req.Capacity.MemoryMB,
		TotalDiskGB: req.Capacity.DiskGB, CPUOvercommit: cpuOvercommit, MemoryOvercommit: memoryOvercommit,
		DiskOvercommit: diskOvercommit, LastHeartbeatAt: &now,
	}
	if err == nil {
		runner.ReservedCPU = existing.ReservedCPU
		runner.ReservedMemoryMB = existing.ReservedMemoryMB
		runner.ReservedDiskGB = existing.ReservedDiskGB
		if err := s.db.Model(&existing).Updates(map[string]any{
			"api_url": req.APIURL, "preview_base_url": req.PreviewBaseURL, "auth_token_hash": runner.AuthTokenHash, "status": runner.Status,
			"total_cpu": runner.TotalCPU, "total_memory_mb": runner.TotalMemoryMB, "total_disk_gb": runner.TotalDiskGB,
			"cpu_overcommit": runner.CPUOvercommit, "memory_overcommit": runner.MemoryOvercommit,
			"disk_overcommit": runner.DiskOvercommit, "last_heartbeat_at": now,
		}).Error; err != nil {
			httpx.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update runner"})
			return
		}
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := s.db.Create(&runner).Error; err != nil {
			httpx.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to register runner"})
			return
		}
	} else {
		httpx.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to look up runner"})
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"runner_id": id, "runner_token": token, "heartbeat_interval_seconds": int(s.cfg.HeartbeatInterval.Seconds())})
}

type heartbeatRequest struct {
	RunningSandboxes int                `json:"running_sandboxes"`
	Capacity         runnerCapacity     `json:"capacity"`
	Pressure         api.RunnerPressure `json:"pressure"`
}

func (s *Server) runnerHeartbeat(w http.ResponseWriter, r *http.Request) {
	runnerID := chi.URLParam(r, "runnerID")
	var runner model.Runner
	if err := s.db.First(&runner, "id = ?", runnerID).Error; err != nil {
		httpx.JSON(w, http.StatusNotFound, map[string]string{"error": "runner not found"})
		return
	}
	if !security.CheckToken(httpx.Bearer(r), runner.AuthTokenHash) {
		httpx.JSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req heartbeatRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.JSON(w, http.StatusBadRequest, api.ErrorResponse{Error: "invalid heartbeat"})
		return
	}
	now := time.Now()
	pressureJSON, err := json.Marshal(req.Pressure)
	if err != nil {
		httpx.JSON(w, http.StatusBadRequest, api.ErrorResponse{Error: "invalid runner pressure"})
		return
	}
	updates := map[string]any{
		"status": model.RunnerStatusHealthy, "last_heartbeat_at": now,
		"reported_running_sandboxes": req.RunningSandboxes,
		"cpu_utilization":            req.Pressure.CPUUtilization,
		"load1":                      req.Pressure.Load1,
		"runnable_processes":         req.Pressure.RunnableProcesses,
		"starting_operations":        req.Pressure.StartingOperations,
		"metadata_json":              string(pressureJSON),
	}
	if req.Capacity.CPU > 0 {
		updates["total_cpu"] = req.Capacity.CPU
		updates["total_memory_mb"] = req.Capacity.MemoryMB
		updates["total_disk_gb"] = req.Capacity.DiskGB
		updates["cpu_overcommit"] = defaultOvercommit(req.Capacity.CPUOvercommit, defaultCPUOvercommit)
		updates["memory_overcommit"] = defaultOvercommit(req.Capacity.MemoryOvercommit, defaultMemoryOvercommit)
		updates["disk_overcommit"] = defaultOvercommit(req.Capacity.DiskOvercommit, defaultDiskOvercommit)
	}
	if err := s.db.WithContext(r.Context()).Model(&runner).Updates(updates).Error; err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to update runner heartbeat"})
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func defaultOvercommit(value, fallback float64) float64 {
	if value <= 0 {
		return fallback
	}
	return value
}

func (s *Server) listRunners(w http.ResponseWriter, r *http.Request) {
	var runners []model.Runner
	if err := s.db.Order("created_at desc").Find(&runners).Error; err != nil {
		httpx.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list runners"})
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": runners})
}
