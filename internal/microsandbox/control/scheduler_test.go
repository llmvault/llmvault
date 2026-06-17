package control

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/microsandbox/api"
	"github.com/usehivy/hivy/internal/microsandbox/model"
)

func TestSelectRunnerUsesTightestFitWithOvercommit(t *testing.T) {
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
		if selected.ID != "small" {
			t.Fatalf("selected runner = %q, want small", selected.ID)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
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
