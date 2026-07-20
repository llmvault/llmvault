package control

import (
	"context"
	cryptorand "crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/microsandbox/api"
	"github.com/usehivy/hivy/internal/microsandbox/model"
)

const (
	defaultPlacementTimeout = 5 * time.Second
	maximumPlacementBackoff = 25 * time.Millisecond
)

var (
	errNoRunnerCapacity     = errors.New("no runner has enough capacity")
	errRunnerLockContention = errors.New("eligible runners are temporarily locked")
)

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
	hostLoad := "(load1 / NULLIF(total_cpu, 0))"
	runningLoad := "(reported_running_sandboxes * 1.0 / NULLIF(total_cpu, 0))"
	creatingLoad := "((SELECT COUNT(1) FROM microsandbox_sandboxes pressure_s WHERE pressure_s.runner_id = microsandbox_runners.id AND pressure_s.status = 'creating') * 1.0 / NULLIF(total_cpu, 0))"
	reportedStartLoad := "(starting_operations * 1.0 / NULLIF(total_cpu, 0))"
	startPressure := fmt.Sprintf("CASE WHEN %s >= %s THEN %s ELSE %s END", creatingLoad, reportedStartLoad, creatingLoad, reportedStartLoad)
	busyPressure := fmt.Sprintf("CASE WHEN (cpu_utilization / 100.0) >= %s AND (cpu_utilization / 100.0) >= %s THEN (cpu_utilization / 100.0) WHEN %s >= %s THEN %s ELSE %s END", hostLoad, runningLoad, hostLoad, runningLoad, hostLoad, runningLoad)
	hostPressure := fmt.Sprintf("(%s + %s)", busyPressure, startPressure)
	return clause.OrderBy{Expression: clause.Expr{
		// Live pressure only changes preference. runnerCandidates remains the
		// authoritative additive-capacity gate inside the row-lock transaction.
		SQL: fmt.Sprintf("%s ASC, runnable_processes ASC, CASE WHEN %s >= %s THEN %s ELSE %s END ASC, id ASC", hostPressure, cpuLoad, memoryLoad, cpuLoad, memoryLoad),
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
