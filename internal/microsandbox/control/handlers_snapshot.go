package control

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/microsandbox/api"
	"github.com/usehivy/hivy/internal/microsandbox/httpx"
	"github.com/usehivy/hivy/internal/microsandbox/model"
	"github.com/usehivy/hivy/internal/microsandbox/security"
	"github.com/usehivy/hivy/internal/microsandbox/storage"
)

type createSnapshotRequest struct {
	OrgID        string            `json:"org_id"`
	Name         string            `json:"name"`
	Alias        string            `json:"alias"`
	Global       bool              `json:"global"`
	BaseImageRef string            `json:"base_image_ref"`
	Size         string            `json:"size"`
	CPU          int               `json:"cpu"`
	MemoryMB     int               `json:"memory_mb"`
	DiskGB       int               `json:"disk_gb"`
	Commands     []string          `json:"commands"`
	Env          map[string]string `json:"env"`
}

type runnerCreateSnapshotResponse struct {
	ID                  string `json:"id"`
	ArtifactURL         string `json:"artifact_url"`
	ArtifactDigest      string `json:"artifact_digest"`
	ArtifactSizeBytes   int64  `json:"artifact_size_bytes"`
	ArtifactMediaType   string `json:"artifact_media_type"`
	SnapshotDigest      string `json:"snapshot_digest"`
	ImageManifestDigest string `json:"image_manifest_digest"`
	Logs                string `json:"logs"`
}

func (s *Server) createSnapshot(w http.ResponseWriter, r *http.Request) {
	var req createSnapshotRequest
	if err := httpx.Decode(r, &req); err != nil || req.OrgID == "" || req.BaseImageRef == "" {
		httpx.JSON(w, http.StatusBadRequest, api.ErrorResponse{Error: "org_id and base_image_ref are required"})
		return
	}
	req.Alias = normalizeSnapshotAlias(req.Alias)
	if err := validateSnapshotAlias(req.Alias); err != nil {
		httpx.JSON(w, http.StatusBadRequest, api.ErrorResponse{Error: err.Error()})
		return
	}
	if req.Alias != "" {
		var existing model.Snapshot
		err := s.db.WithContext(r.Context()).First(&existing, "alias = ? OR id = ?", req.Alias, req.Alias).Error
		if err == nil {
			httpx.JSON(w, http.StatusConflict, api.ErrorResponse{Error: "snapshot alias already exists"})
			return
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to check snapshot alias"})
			return
		}
	}
	id, err := security.ShortID("snp")
	if err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to allocate snapshot id"})
		return
	}
	if req.Name == "" {
		req.Name = id
	}
	size := api.Sizes[api.DefaultSize]
	if req.Size != "" {
		if picked, ok := api.Sizes[req.Size]; ok {
			size = picked
		}
	}
	if req.CPU > 0 || req.MemoryMB > 0 || req.DiskGB > 0 {
		size = api.Size{CPU: max(req.CPU, 1), MemoryMB: max(req.MemoryMB, 2048), DiskGB: max(req.DiskGB, 40)}
	}
	commands, _ := json.Marshal(req.Commands)

	var runner model.Runner
	err = s.db.Transaction(func(tx *gorm.DB) error {
		selected, err := selectRunnerForUpdate(tx, size)
		if err != nil {
			return err
		}
		runner = selected
		if err := reserveRunner(tx, &runner, size); err != nil {
			return err
		}
		return tx.Create(&model.Snapshot{
			ID: id, OrgID: req.OrgID, RunnerID: runner.ID, Name: req.Name,
			Alias: req.Alias, Global: req.Global, BaseImageRef: req.BaseImageRef, Status: model.SnapshotStatusBuilding,
			CommandsJSON: string(commands),
		}).Error
	})
	if err != nil {
		httpx.JSON(w, http.StatusServiceUnavailable, api.ErrorResponse{Error: err.Error()})
		return
	}

	var out runnerCreateSnapshotResponse
	callErr := s.client.Post(r.Context(), runner.APIURL, "/v1/snapshots", map[string]any{
		"id": id, "name": req.Name, "base_image_ref": req.BaseImageRef, "commands": req.Commands,
		"env": req.Env, "cpu": size.CPU, "memory_mb": size.MemoryMB, "disk_gb": size.DiskGB,
	}, &out)
	status := model.SnapshotStatusReady
	errMsg := ""
	if callErr != nil {
		status = model.SnapshotStatusError
		errMsg = callErr.Error()
	}
	_ = s.db.Transaction(func(tx *gorm.DB) error {
		_ = releaseRunner(tx, &runner, size)
		return tx.Model(&model.Snapshot{}).Where("id = ?", id).Updates(map[string]any{
			"status": status, "artifact_url": out.ArtifactURL, "artifact_digest": out.ArtifactDigest,
			"artifact_size_bytes": out.ArtifactSizeBytes, "artifact_media_type": out.ArtifactMediaType,
			"snapshot_digest": out.SnapshotDigest, "image_manifest_digest": out.ImageManifestDigest,
			"logs": out.Logs, "error_message": errMsg,
		}).Error
	})
	if callErr != nil {
		httpx.JSON(w, http.StatusBadGateway, api.ErrorResponse{Error: callErr.Error()})
		return
	}
	var snapshot model.Snapshot
	s.db.First(&snapshot, "id = ?", id)
	httpx.JSON(w, http.StatusCreated, snapshot)
}

func (s *Server) getSnapshot(w http.ResponseWriter, r *http.Request) {
	snapshot, err := s.loadSnapshotByRef(r.Context(), chi.URLParam(r, "snapshotID"))
	if err != nil {
		httpx.JSON(w, http.StatusNotFound, api.ErrorResponse{Error: "snapshot not found"})
		return
	}
	httpx.JSON(w, http.StatusOK, snapshot)
}

func (s *Server) deleteSnapshot(w http.ResponseWriter, r *http.Request) {
	snapshot, err := s.loadSnapshotByRef(r.Context(), chi.URLParam(r, "snapshotID"))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			httpx.JSON(w, http.StatusNotFound, api.ErrorResponse{Error: "snapshot not found"})
			return
		}
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to load snapshot"})
		return
	}
	var runner model.Runner
	if err := s.db.WithContext(r.Context()).First(&runner, "id = ?", snapshot.RunnerID).Error; err != nil {
		httpx.JSON(w, http.StatusNotFound, api.ErrorResponse{Error: "runner not found"})
		return
	}
	if err := s.client.Delete(r.Context(), runner.APIURL, "/v1/snapshots/"+snapshot.ID); err != nil {
		httpx.JSON(w, http.StatusBadGateway, api.ErrorResponse{Error: err.Error()})
		return
	}
	if snapshot.ArtifactURL != "" {
		store, err := storage.NewSnapshotStore(r.Context(), s.cfg)
		if err != nil {
			slog.Error("snapshot artifact store init failed", "snapshot_id", snapshot.ID, "error", err)
		} else if store != nil {
			if err := store.Delete(r.Context(), snapshot.ArtifactURL); err != nil {
				slog.Error("snapshot artifact delete failed", "snapshot_id", snapshot.ID, "artifact_url", snapshot.ArtifactURL, "error", err)
			}
		}
	}
	if err := s.db.WithContext(r.Context()).Delete(&snapshot).Error; err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to delete snapshot"})
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
