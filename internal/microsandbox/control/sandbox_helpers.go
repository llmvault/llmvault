package control

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/microsandbox/api"
	"github.com/usehivy/hivy/internal/microsandbox/httpx"
	"github.com/usehivy/hivy/internal/microsandbox/model"
	"github.com/usehivy/hivy/internal/microsandbox/security"
)

func (s *Server) loadSandboxRunner(w http.ResponseWriter, r *http.Request) (model.Sandbox, model.Runner, bool) {
	var sb model.Sandbox
	if err := s.db.First(&sb, "id = ?", chi.URLParam(r, "sandboxID")).Error; err != nil {
		httpx.JSON(w, http.StatusNotFound, api.ErrorResponse{Error: "sandbox not found"})
		return sb, model.Runner{}, false
	}
	var runner model.Runner
	if err := s.db.First(&runner, "id = ?", sb.RunnerID).Error; err != nil {
		httpx.JSON(w, http.StatusNotFound, api.ErrorResponse{Error: "runner not found"})
		return sb, runner, false
	}
	return sb, runner, true
}

func resolveSize(req createSandboxRequest) api.Size {
	if req.CPU > 0 || req.MemoryMB > 0 || req.DiskGB > 0 {
		return api.Size{CPU: max(req.CPU, 1), MemoryMB: max(req.MemoryMB, 2048), DiskGB: max(req.DiskGB, 40)}
	}
	if req.Size == "" {
		req.Size = api.DefaultSize
	}
	if size, ok := api.Sizes[req.Size]; ok {
		return size
	}
	return api.Sizes[api.DefaultSize]
}

func selectRunnerForUpdate(tx *gorm.DB, size api.Size) (model.Runner, error) {
	var runners []model.Runner
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("status = ? AND drain = ?", model.RunnerStatusHealthy, false).
		Find(&runners).Error; err != nil {
		return model.Runner{}, err
	}
	var best *model.Runner
	var bestFree int
	for i := range runners {
		r := &runners[i]
		if !runnerHasCapacity(*r, size) {
			continue
		}
		cpuLimit := int(float64(r.TotalCPU) * r.CPUOvercommit)
		free := (cpuLimit - r.ReservedCPU - size.CPU) + ((r.TotalMemoryMB - r.ReservedMemoryMB - size.MemoryMB) / 1024)
		if best == nil || free < bestFree {
			best = r
			bestFree = free
		}
	}
	if best == nil {
		return model.Runner{}, fmt.Errorf("no runner has enough capacity")
	}
	return *best, nil
}

func selectRunnerByIDForUpdate(tx *gorm.DB, runnerID string, size api.Size) (model.Runner, error) {
	var runner model.Runner
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND status = ? AND drain = ?", runnerID, model.RunnerStatusHealthy, false).
		First(&runner).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.Runner{}, fmt.Errorf("snapshot runner is unavailable")
		}
		return model.Runner{}, err
	}
	if !runnerHasCapacity(runner, size) {
		return model.Runner{}, fmt.Errorf("snapshot runner has insufficient capacity")
	}
	return runner, nil
}

func selectRunnerForSnapshotSandbox(tx *gorm.DB, snapshot model.Snapshot, size api.Size) (model.Runner, error) {
	if snapshot.ArtifactURL == "" {
		return selectRunnerByIDForUpdate(tx, snapshot.RunnerID, size)
	}
	return selectRunnerForUpdate(tx, size)
}

func runnerHasCapacity(runner model.Runner, size api.Size) bool {
	cpuLimit := int(float64(runner.TotalCPU) * runner.CPUOvercommit)
	return runner.ReservedCPU+size.CPU <= cpuLimit &&
		runner.ReservedMemoryMB+size.MemoryMB <= runner.TotalMemoryMB &&
		runner.ReservedDiskGB+size.DiskGB <= runner.TotalDiskGB
}

func reserveRunner(tx *gorm.DB, runner *model.Runner, size api.Size) error {
	return tx.Model(runner).Updates(map[string]any{
		"reserved_cpu":       runner.ReservedCPU + size.CPU,
		"reserved_memory_mb": runner.ReservedMemoryMB + size.MemoryMB,
		"reserved_disk_gb":   runner.ReservedDiskGB + size.DiskGB,
	}).Error
}

func releaseRunner(tx *gorm.DB, runner *model.Runner, size api.Size) error {
	return tx.Model(runner).Updates(map[string]any{
		"reserved_cpu":       max(runner.ReservedCPU-size.CPU, 0),
		"reserved_memory_mb": max(runner.ReservedMemoryMB-size.MemoryMB, 0),
		"reserved_disk_gb":   max(runner.ReservedDiskGB-size.DiskGB, 0),
	}).Error
}

func (s *Server) previewURLs(sandboxID string, ports []model.SandboxPort) map[string]string {
	out := make(map[string]string, len(ports))
	for _, p := range ports {
		out[fmt.Sprintf("%d", p.GuestPort)] = fmt.Sprintf("https://%d-%s.%s", p.GuestPort, sandboxID, s.cfg.PreviewBaseDomain)
	}
	return out
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type runnerCreateSandboxRequest struct {
	ID                     string             `json:"id"`
	Name                   string             `json:"name"`
	ImageRef               string             `json:"image_ref"`
	SnapshotID             string             `json:"snapshot_id"`
	SnapshotArtifactURL    string             `json:"snapshot_artifact_url"`
	SnapshotArtifactDigest string             `json:"snapshot_artifact_digest"`
	SnapshotImageDigest    string             `json:"snapshot_image_digest"`
	CPU                    int                `json:"cpu"`
	MemoryMB               int                `json:"memory_mb"`
	DiskGB                 int                `json:"disk_gb"`
	Env                    map[string]string  `json:"env"`
	Labels                 map[string]string  `json:"labels"`
	PreviewPorts           []int              `json:"preview_ports"`
	Init                   *sandboxInitConfig `json:"init"`
}

type sandboxInitConfig struct {
	Cmd  string            `json:"cmd"`
	Args []string          `json:"args"`
	Env  map[string]string `json:"env,omitempty"`
}

type runnerCreateSandboxResponse struct {
	ID    string `json:"id"`
	Ports []struct {
		GuestPort int `json:"guest_port"`
		HostPort  int `json:"host_port"`
	} `json:"ports"`
}

func (s *Server) ensureOrgPassword(ctx context.Context, orgID, password string) (string, error) {
	var existing model.OrgPreviewSecret
	lookupErr := s.db.WithContext(ctx).First(&existing, "org_id = ?", orgID).Error
	if password == "" && lookupErr == nil {
		return security.DecryptString(s.cfg.PreviewPasswordKey, existing.PasswordCiphertext)
	}
	if password == "" {
		password = security.GeneratePreviewPassword()
	}
	ciphertext, err := security.EncryptString(s.cfg.PreviewPasswordKey, password)
	if err != nil {
		return "", err
	}
	row := model.OrgPreviewSecret{OrgID: orgID, PasswordCiphertext: ciphertext}
	if errors.Is(lookupErr, gorm.ErrRecordNotFound) {
		return password, s.db.WithContext(ctx).Create(&row).Error
	}
	if lookupErr != nil {
		return "", lookupErr
	}
	return password, s.db.WithContext(ctx).Model(&existing).Update("password_ciphertext", ciphertext).Error
}
