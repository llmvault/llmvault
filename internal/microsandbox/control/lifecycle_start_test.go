package control

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc/pool"

	"github.com/usehivy/hivy/internal/microsandbox/config"
	"github.com/usehivy/hivy/internal/microsandbox/model"
)

func TestConcurrentStartCallsRunnerOnce(t *testing.T) {
	const runnerToken = "runner-token"
	var runnerCalls int32
	runnerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sandboxes/sbx_concurrent/start" {
			t.Fatalf("runner path = %s", r.URL.Path)
		}
		atomic.AddInt32(&runnerCalls, 1)
		time.Sleep(40 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "running"})
	}))
	defer runnerServer.Close()

	db := newTemplateControlTestDB(t)
	if err := db.Create(&model.Runner{
		ID: "runner-1", Name: "runner-1", APIURL: runnerServer.URL, AuthTokenHash: []byte("hash"),
		Status: model.RunnerStatusHealthy, TotalCPU: 8, TotalMemoryMB: 16384, TotalDiskGB: 100,
		CPUOvercommit: 1.5, MemoryOvercommit: 1, DiskOvercommit: 1, ReservedDiskGB: 40,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Sandbox{
		ID: "sbx_concurrent", OrgID: "org_1", RunnerID: "runner-1", Name: "test",
		ImageRef: "image:test", Status: model.SandboxStatusStopped, CPU: 1, MemoryMB: 2048, DiskGB: 40,
	}).Error; err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{APIToken: "api-token", RunnerAPIToken: runnerToken, PreviewPasswordKey: "preview-password-key"}
	s := &Server{db: db, cfg: cfg, client: NewRunnerClient(cfg.RunnerAPIToken)}
	handler := s.Routes()

	p := pool.New().WithErrors().WithMaxGoroutines(8)
	for i := 0; i < 8; i++ {
		p.Go(func() error {
			req := httptest.NewRequest(http.MethodPost, "/v1/sandboxes/sbx_concurrent/start", nil)
			req.Header.Set("Authorization", "Bearer api-token")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				return fmt.Errorf("status = %d body = %s", rec.Code, strings.TrimSpace(rec.Body.String()))
			}
			return nil
		})
	}
	if err := p.Wait(); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&runnerCalls); got != 1 {
		t.Fatalf("runner start calls = %d, want 1", got)
	}
	var sb model.Sandbox
	if err := db.First(&sb, "id = ?", "sbx_concurrent").Error; err != nil {
		t.Fatal(err)
	}
	if sb.Status != model.SandboxStatusRunning {
		t.Fatalf("status = %q, want running", sb.Status)
	}
	var runner model.Runner
	if err := db.First(&runner, "id = ?", "runner-1").Error; err != nil {
		t.Fatal(err)
	}
	if runner.ReservedCPU != 1 || runner.ReservedMemoryMB != 2048 || runner.ReservedDiskGB != 40 {
		t.Fatalf("runner reserved = cpu %d memory %d disk %d, want cpu 1 memory 2048 disk 40", runner.ReservedCPU, runner.ReservedMemoryMB, runner.ReservedDiskGB)
	}
}
