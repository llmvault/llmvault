package control

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/microsandbox/api"
	"github.com/usehivy/hivy/internal/microsandbox/model"
)

func TestSelectRunnerUsesLowestNormalizedActiveLoad(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:scheduler-fit?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Runner{}); err != nil {
		t.Fatal(err)
	}
	runners := []model.Runner{
		{ID: "big", Name: "big", APIURL: "http://big", AuthTokenHash: []byte("hash"), Status: model.RunnerStatusHealthy, TotalCPU: 16, TotalMemoryMB: 32768, TotalDiskGB: 500, CPUOvercommit: 1.5},
		{ID: "small", Name: "small", APIURL: "http://small", AuthTokenHash: []byte("hash"), Status: model.RunnerStatusHealthy, TotalCPU: 4, TotalMemoryMB: 8192, TotalDiskGB: 120, CPUOvercommit: 1.5},
	}
	if err := db.Create(&runners).Error; err != nil {
		t.Fatal(err)
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		selected, err := selectRunnerForUpdate(tx, api.Size{CPU: 2, MemoryMB: 4096, DiskGB: 60})
		if err != nil {
			return err
		}
		if selected.ID != "big" {
			t.Fatalf("selected runner = %q, want big", selected.ID)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSelectRunnerSpreadsActiveReservationsAcrossHealthyRunners(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:scheduler-spread?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Runner{}); err != nil {
		t.Fatal(err)
	}
	runners := []model.Runner{
		{ID: "runner-a", Name: "runner-a", APIURL: "http://runner-a", AuthTokenHash: []byte("hash"), Status: model.RunnerStatusHealthy, TotalCPU: 8, TotalMemoryMB: 16384, TotalDiskGB: 200},
		{ID: "runner-b", Name: "runner-b", APIURL: "http://runner-b", AuthTokenHash: []byte("hash"), Status: model.RunnerStatusHealthy, TotalCPU: 8, TotalMemoryMB: 16384, TotalDiskGB: 200},
	}
	if err := db.Create(&runners).Error; err != nil {
		t.Fatal(err)
	}

	size := api.Size{CPU: 1, MemoryMB: 1024, DiskGB: 5}
	selectedIDs := make([]string, 0, 2)
	for range 2 {
		err := db.Transaction(func(tx *gorm.DB) error {
			selected, err := selectRunnerForUpdate(tx, size)
			if err != nil {
				return err
			}
			selectedIDs = append(selectedIDs, selected.ID)
			return reserveRunner(tx, &selected, size)
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	if selectedIDs[0] == selectedIDs[1] {
		t.Fatalf("selected runners = %v, want consecutive cold starts spread across both runners", selectedIDs)
	}
}

func TestSelectRunnerRejectsInsufficientCapacity(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:scheduler-insufficient?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Runner{}); err != nil {
		t.Fatal(err)
	}
	runner := model.Runner{ID: "full", Name: "full", APIURL: "http://full", AuthTokenHash: []byte("hash"), Status: model.RunnerStatusHealthy, TotalCPU: 2, TotalMemoryMB: 2048, TotalDiskGB: 20, CPUOvercommit: 1}
	if err := db.Create(&runner).Error; err != nil {
		t.Fatal(err)
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		_, err := selectRunnerForUpdate(tx, api.Size{CPU: 4, MemoryMB: 4096, DiskGB: 40})
		return err
	})
	if err == nil {
		t.Fatal("expected capacity error")
	}
}

func TestSelectRunnerUsesMemoryAndDiskOvercommit(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:scheduler-memory-disk-overcommit?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Runner{}); err != nil {
		t.Fatal(err)
	}
	runners := []model.Runner{
		{
			ID: "strict", Name: "strict", APIURL: "http://strict", AuthTokenHash: []byte("hash"),
			Status: model.RunnerStatusHealthy, TotalCPU: 8, TotalMemoryMB: 4096, TotalDiskGB: 40,
			CPUOvercommit: 1, MemoryOvercommit: 1, DiskOvercommit: 1,
			ReservedMemoryMB: 4096, ReservedDiskGB: 40,
		},
		{
			ID: "overcommit", Name: "overcommit", APIURL: "http://overcommit", AuthTokenHash: []byte("hash"),
			Status: model.RunnerStatusHealthy, TotalCPU: 8, TotalMemoryMB: 4096, TotalDiskGB: 40,
			CPUOvercommit: 1, MemoryOvercommit: 2, DiskOvercommit: 2,
			ReservedMemoryMB: 4096, ReservedDiskGB: 40,
		},
	}
	if err := db.Create(&runners).Error; err != nil {
		t.Fatal(err)
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		selected, err := selectRunnerForUpdate(tx, api.Size{CPU: 1, MemoryMB: 4096, DiskGB: 40})
		if err != nil {
			return err
		}
		if selected.ID != "overcommit" {
			t.Fatalf("selected runner = %q, want overcommit", selected.ID)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunnerHasCapacityDefaultsToFourXDiskOvercommit(t *testing.T) {
	runner := model.Runner{
		ID: "runner-1", Status: model.RunnerStatusHealthy,
		TotalCPU: 8, TotalMemoryMB: 16384, TotalDiskGB: 100,
		ReservedDiskGB: 390,
	}
	if !runnerHasCapacity(runner, api.Size{CPU: 1, MemoryMB: 1024, DiskGB: 10}) {
		t.Fatal("expected 4x default disk overcommit to allow 400GB reserved on a 100GB runner")
	}
	if runnerHasCapacity(runner, api.Size{CPU: 1, MemoryMB: 1024, DiskGB: 11}) {
		t.Fatal("expected 4x default disk overcommit to reject more than 400GB reserved on a 100GB runner")
	}
}

func TestReleaseRunnerUsesCurrentReservedCapacity(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:runner-release-current?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Runner{}); err != nil {
		t.Fatal(err)
	}
	runner := model.Runner{
		ID: "runner-1", Name: "runner-1", APIURL: "http://runner", AuthTokenHash: []byte("hash"),
		Status: model.RunnerStatusHealthy, TotalCPU: 16, TotalMemoryMB: 32768, TotalDiskGB: 500, CPUOvercommit: 1.5,
		ReservedCPU: 13, ReservedMemoryMB: 26624, ReservedDiskGB: 400,
	}
	if err := db.Create(&runner).Error; err != nil {
		t.Fatal(err)
	}

	var staleA, staleB model.Runner
	if err := db.First(&staleA, "id = ?", runner.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&staleB, "id = ?", runner.ID).Error; err != nil {
		t.Fatal(err)
	}

	if err := releaseRunner(db, &staleA, api.Size{CPU: 6, MemoryMB: 12288, DiskGB: 160}); err != nil {
		t.Fatal(err)
	}
	if err := releaseRunner(db, &staleB, api.Size{CPU: 5, MemoryMB: 10240, DiskGB: 160}); err != nil {
		t.Fatal(err)
	}

	var got model.Runner
	if err := db.First(&got, "id = ?", runner.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.ReservedCPU != 2 || got.ReservedMemoryMB != 4096 || got.ReservedDiskGB != 80 {
		t.Fatalf("reserved capacity = cpu %d memory %d disk %d, want cpu 2 memory 4096 disk 80",
			got.ReservedCPU, got.ReservedMemoryMB, got.ReservedDiskGB)
	}
}
