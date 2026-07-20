package control

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/usehivy/hivy/internal/microsandbox/api"
	"github.com/usehivy/hivy/internal/microsandbox/model"
	"github.com/usehivy/hivy/internal/testdb"
)

func TestSelectRunnerSkipsLockedRunnerInsteadOfLockingFleet(t *testing.T) {
	db := newSchedulerPostgresTestDB(t)
	createSchedulerTestRunners(t, db, 100)

	locker := db.Begin()
	if locker.Error != nil {
		t.Fatal(locker.Error)
	}
	t.Cleanup(func() { _ = locker.Rollback().Error })
	if err := locker.Exec(
		"SELECT id FROM microsandbox_runners WHERE id = ? FOR UPDATE",
		"runner-a",
	).Error; err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	started := time.Now()
	var selected model.Runner
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		selected, err = selectRunnerForUpdate(tx, api.Size{CPU: 1, MemoryMB: 512, DiskGB: 1})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != "runner-b" {
		t.Fatalf("selected runner = %q, want unlocked runner-b", selected.ID)
	}
	if elapsed := time.Since(started); elapsed >= 500*time.Millisecond {
		t.Fatalf("selection waited %s for a locked runner", elapsed)
	}
}

func TestConcurrentPlacementsDoNotDeadlockOrOverReserve(t *testing.T) {
	db := newSchedulerPostgresTestDB(t)
	createSchedulerTestRunners(t, db, 500)

	const placements = 500
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	errorsCh := make(chan error, placements)
	var wg sync.WaitGroup
	for range placements {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := reserveRunnerForPlacement(
				ctx,
				db,
				api.Size{CPU: 1, MemoryMB: 512, DiskGB: 1},
				5*time.Second,
				nil,
			)
			if err != nil {
				errorsCh <- err
			}
		}()
	}
	wg.Wait()
	close(errorsCh)
	for err := range errorsCh {
		t.Errorf("placement failed: %v", err)
	}
	if t.Failed() {
		return
	}

	var runners []model.Runner
	if err := db.Order("id").Find(&runners).Error; err != nil {
		t.Fatal(err)
	}
	if len(runners) != 2 {
		t.Fatalf("runner count = %d, want 2", len(runners))
	}
	totalReservedCPU := 0
	for _, runner := range runners {
		if runner.ReservedCPU > runner.TotalCPU {
			t.Fatalf("runner %s reserved %d CPU with capacity %d", runner.ID, runner.ReservedCPU, runner.TotalCPU)
		}
		totalReservedCPU += runner.ReservedCPU
	}
	if totalReservedCPU != placements {
		t.Fatalf("reserved CPU = %d, want %d", totalReservedCPU, placements)
	}
}

func TestConcurrentSandboxStopsReleaseSharedRunnerReservations(t *testing.T) {
	db := newSchedulerPostgresTestDB(t)
	createSchedulerTestRunners(t, db, 100)
	if err := db.Model(&model.Runner{}).Where("id IN ?", []string{"runner-a", "runner-b"}).Updates(map[string]any{
		"reserved_cpu": 50, "reserved_memory_mb": 50 * 512, "reserved_disk_gb": 50,
	}).Error; err != nil {
		t.Fatal(err)
	}

	sandboxes := make([]model.Sandbox, 0, 100)
	for i := 0; i < 100; i++ {
		runnerID := "runner-a"
		if i%2 == 1 {
			runnerID = "runner-b"
		}
		sandboxes = append(sandboxes, model.Sandbox{
			ID: fmt.Sprintf("sbx-stop-%03d", i), OrgID: "org-test", RunnerID: runnerID,
			Name: fmt.Sprintf("stop-%03d", i), ImageRef: "image:test", Status: model.SandboxStatusStopping,
			CPU: 1, MemoryMB: 512, DiskGB: 1,
		})
	}
	if err := db.Create(&sandboxes).Error; err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	errorsCh := make(chan error, len(sandboxes))
	var wg sync.WaitGroup
	for i := range sandboxes {
		sb := sandboxes[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				if err := releaseRunnerReservationForSandboxTx(tx, sb, runtimeReservationSize(sb)); err != nil {
					return err
				}
				return tx.Model(&model.Sandbox{}).Where("id = ?", sb.ID).Update("status", model.SandboxStatusStopped).Error
			})
			if err != nil {
				errorsCh <- err
			}
		}()
	}
	wg.Wait()
	close(errorsCh)
	for err := range errorsCh {
		t.Errorf("concurrent stop update failed: %v", err)
	}
	if t.Failed() {
		return
	}

	var runners []model.Runner
	if err := db.Order("id").Find(&runners).Error; err != nil {
		t.Fatal(err)
	}
	for _, runner := range runners {
		if runner.ReservedCPU != 0 || runner.ReservedMemoryMB != 0 || runner.ReservedDiskGB != 50 {
			t.Fatalf("runner %s reservations = cpu %d memory %d disk %d, want 0/0/50",
				runner.ID, runner.ReservedCPU, runner.ReservedMemoryMB, runner.ReservedDiskGB)
		}
	}
	var stopped int64
	if err := db.Model(&model.Sandbox{}).Where("status = ?", model.SandboxStatusStopped).Count(&stopped).Error; err != nil {
		t.Fatal(err)
	}
	if stopped != 100 {
		t.Fatalf("stopped sandboxes=%d want 100", stopped)
	}
}

func newSchedulerPostgresTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := testdb.DatabaseURL("TEST_DATABASE_URL", "DATABASE_URL", "HIVY_DATABASE_URL")
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Skipf("PostgreSQL is unavailable: %v", err)
	}
	schema := "msb_scheduler_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Skipf("cannot create PostgreSQL test schema: %v", err)
	}

	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	db, err := gorm.Open(postgres.Open(parsed.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(10)
	adminSQL, err := admin.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
		_ = admin.Exec(fmt.Sprintf("DROP SCHEMA %s CASCADE", schema)).Error
		_ = adminSQL.Close()
	})
	if err := db.AutoMigrate(&model.Runner{}, &model.Sandbox{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func createSchedulerTestRunners(t *testing.T, db *gorm.DB, totalCPU int) {
	t.Helper()
	runners := []model.Runner{
		{
			ID: "runner-a", Name: "runner-a", APIURL: "http://runner-a", AuthTokenHash: []byte("hash"),
			Status: model.RunnerStatusHealthy, TotalCPU: totalCPU, TotalMemoryMB: 1024 * totalCPU,
			TotalDiskGB: totalCPU, CPUOvercommit: 1, MemoryOvercommit: 1, DiskOvercommit: 1,
		},
		{
			ID: "runner-b", Name: "runner-b", APIURL: "http://runner-b", AuthTokenHash: []byte("hash"),
			Status: model.RunnerStatusHealthy, TotalCPU: totalCPU, TotalMemoryMB: 1024 * totalCPU,
			TotalDiskGB: totalCPU, CPUOvercommit: 1, MemoryOvercommit: 1, DiskOvercommit: 1,
		},
	}
	if err := db.Create(&runners).Error; err != nil {
		t.Fatal(err)
	}
}
