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
	if err := db.AutoMigrate(&model.Runner{}); err != nil {
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
