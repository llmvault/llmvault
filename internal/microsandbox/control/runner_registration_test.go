package control

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/microsandbox/config"
	"github.com/usehivy/hivy/internal/microsandbox/model"
)

func TestRegisterRunnerPersistsMemoryAndDiskOvercommit(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:runner-registration-overcommit?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Runner{}); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{RunnerJoinSecret: "join-secret", HeartbeatInterval: time.Minute}
	s := &Server{db: db, cfg: cfg}
	req := httptest.NewRequest(http.MethodPost, "/v1/runners/register", strings.NewReader(`{
		"name": "runner-1",
		"api_url": "https://runner.example.com",
		"preview_base_url": "http://10.80.1.2",
		"capacity": {
			"cpu": 16,
			"memory_mb": 32768,
			"disk_gb": 500,
			"cpu_overcommit": 4,
			"memory_overcommit": 1.25,
			"disk_overcommit": 2.5
		}
	}`))
	req.Header.Set("Authorization", "Bearer join-secret")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("register status = %d body = %s", rec.Code, rec.Body.String())
	}

	var runner model.Runner
	if err := db.First(&runner, "name = ?", "runner-1").Error; err != nil {
		t.Fatal(err)
	}
	if runner.CPUOvercommit != 4 || runner.MemoryOvercommit != 1.25 || runner.DiskOvercommit != 2.5 {
		t.Fatalf("overcommit = cpu %.2f memory %.2f disk %.2f, want 4/1.25/2.5",
			runner.CPUOvercommit, runner.MemoryOvercommit, runner.DiskOvercommit)
	}
}
