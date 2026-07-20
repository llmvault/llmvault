package control

import (
	"context"
	cryptorand "crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/microsandbox/api"
	"github.com/usehivy/hivy/internal/microsandbox/httpx"
	"github.com/usehivy/hivy/internal/microsandbox/model"
	"github.com/usehivy/hivy/internal/microsandbox/security"
)

const (
	defaultCPUOvercommit    = 1.5
	defaultMemoryOvercommit = 1
	defaultDiskOvercommit   = 4
	defaultPlacementTimeout = 5 * time.Second
	maximumPlacementBackoff = 25 * time.Millisecond
)

var (
	errNoRunnerCapacity     = errors.New("no runner has enough capacity")
	errRunnerLockContention = errors.New("eligible runners are temporarily locked")
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
		minSize := api.Sizes["nano"]
		return api.Size{
			CPU:      max(req.CPU, minSize.CPU),
			MemoryMB: max(req.MemoryMB, minSize.MemoryMB),
			DiskGB:   max(req.DiskGB, minSize.DiskGB),
		}
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
	var selected model.Runner
	err := runnerCandidates(tx, size).
		Clauses(runnerLoadOrder(size)).
		Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		Limit(1).
		Take(&selected).Error
	if err == nil {
		return selected, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Runner{}, err
	}

	// An unlocked read distinguishes exhausted capacity from a short-lived lock
	// collision. The caller retries collisions after ending this transaction, so
	// no database connection or row lock is held during backoff.
	var eligible model.Runner
	err = runnerCandidates(tx, size).Select("id").Limit(1).Take(&eligible).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Runner{}, errNoRunnerCapacity
	}
	if err != nil {
		return model.Runner{}, err
	}
	return model.Runner{}, errRunnerLockContention
}

func runnerCandidates(tx *gorm.DB, size api.Size) *gorm.DB {
	return tx.Model(&model.Runner{}).
		Where("status = ? AND drain = ?", model.RunnerStatusHealthy, false).
		Where(
			"reserved_cpu + ? <= total_cpu * CASE WHEN cpu_overcommit > 0 THEN cpu_overcommit ELSE ? END",
			size.CPU,
			defaultCPUOvercommit,
		).
		Where(
			"reserved_memory_mb + ? <= total_memory_mb * CASE WHEN memory_overcommit > 0 THEN memory_overcommit ELSE ? END",
			size.MemoryMB,
			defaultMemoryOvercommit,
		).
		Where(
			"reserved_disk_gb + ? <= total_disk_gb * CASE WHEN disk_overcommit > 0 THEN disk_overcommit ELSE ? END",
			size.DiskGB,
			defaultDiskOvercommit,
		)
}

func runnerLoadOrder(size api.Size) clause.OrderBy {
	cpuLoad := "((reserved_cpu + ?) * 1.0 / NULLIF(total_cpu * CASE WHEN cpu_overcommit > 0 THEN cpu_overcommit ELSE ? END, 0))"
	memoryLoad := "((reserved_memory_mb + ?) * 1.0 / NULLIF(total_memory_mb * CASE WHEN memory_overcommit > 0 THEN memory_overcommit ELSE ? END, 0))"
	return clause.OrderBy{Expression: clause.Expr{
		SQL: fmt.Sprintf("CASE WHEN %s >= %s THEN %s ELSE %s END ASC, id ASC", cpuLoad, memoryLoad, cpuLoad, memoryLoad),
		Vars: []any{
			size.CPU, defaultCPUOvercommit,
			size.MemoryMB, defaultMemoryOvercommit,
			size.CPU, defaultCPUOvercommit,
			size.MemoryMB, defaultMemoryOvercommit,
		},
	}}
}

func reserveRunnerForPlacement(
	ctx context.Context,
	db *gorm.DB,
	size api.Size,
	timeout time.Duration,
	persist func(*gorm.DB, model.Runner) error,
) (model.Runner, error) {
	if timeout <= 0 {
		timeout = defaultPlacementTimeout
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for attempt := 0; ; attempt++ {
		var selected model.Runner
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			runner, err := selectRunnerForUpdate(tx, size)
			if err != nil {
				return err
			}
			selected = runner
			if err := reserveRunner(tx, &selected, size); err != nil {
				return err
			}
			if persist == nil {
				return nil
			}
			return persist(tx, selected)
		})
		if err == nil {
			return selected, nil
		}
		if errors.Is(err, errNoRunnerCapacity) || !retryablePlacementError(err) {
			return model.Runner{}, err
		}
		lastErr = err
		if !time.Now().Before(deadline) {
			return model.Runner{}, lastErr
		}

		backoff := time.Millisecond << min(attempt, 5)
		backoff = min(backoff, maximumPlacementBackoff)
		wait := placementBackoff(backoff)
		if remaining := time.Until(deadline); wait > remaining {
			wait = remaining
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return model.Runner{}, ctx.Err()
		case <-timer.C:
		}
	}
}

func placementBackoff(maximum time.Duration) time.Duration {
	value, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(maximum)+1))
	if err != nil {
		return maximum / 2
	}
	return time.Duration(value.Int64())
}

func retryablePlacementError(err error) bool {
	if errors.Is(err, errRunnerLockContention) {
		return true
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "40P01" || pgErr.Code == "40001"
}

func runnerHasCapacity(runner model.Runner, size api.Size) bool {
	cpuLimit := overcommitLimit(runner.TotalCPU, runner.CPUOvercommit, defaultCPUOvercommit)
	memoryLimit := overcommitLimit(runner.TotalMemoryMB, runner.MemoryOvercommit, defaultMemoryOvercommit)
	diskLimit := overcommitLimit(runner.TotalDiskGB, runner.DiskOvercommit, defaultDiskOvercommit)
	return runner.ReservedCPU+size.CPU <= cpuLimit &&
		runner.ReservedMemoryMB+size.MemoryMB <= memoryLimit &&
		runner.ReservedDiskGB+size.DiskGB <= diskLimit
}

func overcommitLimit(total int, overcommit, fallback float64) int {
	if overcommit <= 0 {
		overcommit = fallback
	}
	return int(float64(total) * overcommit)
}

func reserveRunner(tx *gorm.DB, runner *model.Runner, size api.Size) error {
	return tx.Model(runner).Updates(map[string]any{
		"reserved_cpu":       gorm.Expr("reserved_cpu + ?", size.CPU),
		"reserved_memory_mb": gorm.Expr("reserved_memory_mb + ?", size.MemoryMB),
		"reserved_disk_gb":   gorm.Expr("reserved_disk_gb + ?", size.DiskGB),
	}).Error
}

func releaseRunner(tx *gorm.DB, runner *model.Runner, size api.Size) error {
	return tx.Model(runner).Updates(map[string]any{
		"reserved_cpu":       gorm.Expr("CASE WHEN reserved_cpu > ? THEN reserved_cpu - ? ELSE 0 END", size.CPU, size.CPU),
		"reserved_memory_mb": gorm.Expr("CASE WHEN reserved_memory_mb > ? THEN reserved_memory_mb - ? ELSE 0 END", size.MemoryMB, size.MemoryMB),
		"reserved_disk_gb":   gorm.Expr("CASE WHEN reserved_disk_gb > ? THEN reserved_disk_gb - ? ELSE 0 END", size.DiskGB, size.DiskGB),
	}).Error
}

func runtimeReservationSize(sb model.Sandbox) api.Size {
	return api.Size{CPU: sb.CPU, MemoryMB: sb.MemoryMB}
}

func runtimeReservationHeld(status string) bool {
	switch status {
	case model.SandboxStatusCreating, model.SandboxStatusRunning, model.SandboxStatusStopping:
		return true
	default:
		return false
	}
}

func diskReservationHeld(status string) bool {
	switch status {
	case model.SandboxStatusCreating, model.SandboxStatusRunning, model.SandboxStatusStopping, model.SandboxStatusStopped:
		return true
	default:
		return false
	}
}

func reserveRunnerReservationForSandbox(ctx context.Context, db *gorm.DB, sb model.Sandbox, size api.Size) error {
	if size.CPU == 0 && size.MemoryMB == 0 && size.DiskGB == 0 {
		return nil
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return reserveRunnerReservationForSandboxTx(tx, sb, size)
	})
}

func releaseRunnerReservationForSandbox(ctx context.Context, db *gorm.DB, sb model.Sandbox, size api.Size) error {
	if size.CPU == 0 && size.MemoryMB == 0 && size.DiskGB == 0 {
		return nil
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return releaseRunnerReservationForSandboxTx(tx, sb, size)
	})
}

func reserveRunnerReservationForSandboxTx(tx *gorm.DB, sb model.Sandbox, size api.Size) error {
	var runner model.Runner
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&runner, "id = ?", sb.RunnerID).Error; err != nil {
		return err
	}
	if !runnerHasCapacity(runner, size) {
		return fmt.Errorf("runner %s has insufficient capacity", runner.ID)
	}
	return reserveRunner(tx, &runner, size)
}

func releaseRunnerReservationForSandboxTx(tx *gorm.DB, sb model.Sandbox, size api.Size) error {
	var runner model.Runner
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&runner, "id = ?", sb.RunnerID).Error; err != nil {
		return err
	}
	return releaseRunner(tx, &runner, size)
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
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	ImageRef     string             `json:"image_ref"`
	CPU          int                `json:"cpu"`
	MemoryMB     int                `json:"memory_mb"`
	DiskGB       int                `json:"disk_gb"`
	Env          map[string]string  `json:"env"`
	Labels       map[string]string  `json:"labels"`
	PreviewPorts []int              `json:"preview_ports"`
	Init         *sandboxInitConfig `json:"init"`
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
