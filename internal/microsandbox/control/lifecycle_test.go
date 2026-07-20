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

func TestConcurrentStopCallsRunnerOnce(t *testing.T) {
	const runnerToken = "runner-token"
	var runnerCalls int32
	runnerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sandboxes/sbx_concurrent_stop/stop" {
			t.Fatalf("runner path = %s", r.URL.Path)
		}
		atomic.AddInt32(&runnerCalls, 1)
		time.Sleep(40 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
	}))
	defer runnerServer.Close()

	db := newTemplateControlTestDB(t)
	if err := db.Create(&model.Runner{
		ID: "runner-stop", Name: "runner-stop", APIURL: runnerServer.URL, AuthTokenHash: []byte("hash"),
		Status: model.RunnerStatusHealthy, TotalCPU: 8, TotalMemoryMB: 16384, TotalDiskGB: 100,
		ReservedCPU: 1, ReservedMemoryMB: 2048, ReservedDiskGB: 40,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Sandbox{
		ID: "sbx_concurrent_stop", OrgID: "org_1", RunnerID: "runner-stop", Name: "test",
		ImageRef: "image:test", Status: model.SandboxStatusRunning, CPU: 1, MemoryMB: 2048, DiskGB: 40,
	}).Error; err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{APIToken: "api-token", RunnerAPIToken: runnerToken, PreviewPasswordKey: "preview-password-key"}
	s := &Server{db: db, cfg: cfg, client: NewRunnerClient(cfg.RunnerAPIToken)}
	handler := s.Routes()
	p := pool.New().WithErrors().WithMaxGoroutines(8)
	for i := 0; i < 8; i++ {
		p.Go(func() error {
			req := httptest.NewRequest(http.MethodPost, "/v1/sandboxes/sbx_concurrent_stop/stop", nil)
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
		t.Fatalf("runner stop calls = %d, want 1", got)
	}
}

func TestFailedStopLeavesSandboxNonRoutable(t *testing.T) {
	const runnerToken = "runner-token"
	runnerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "disk still attached", http.StatusInternalServerError)
	}))
	defer runnerServer.Close()

	db := newTemplateControlTestDB(t)
	if err := db.Create(&model.Runner{
		ID: "runner-1", Name: "runner-1", APIURL: runnerServer.URL, AuthTokenHash: []byte("hash"),
		Status: model.RunnerStatusHealthy, TotalCPU: 8, TotalMemoryMB: 16384, TotalDiskGB: 100,
		CPUOvercommit: 1.5, MemoryOvercommit: 1, DiskOvercommit: 1,
		ReservedCPU: 1, ReservedMemoryMB: 2048, ReservedDiskGB: 40,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Sandbox{
		ID: "sbx_stop", OrgID: "org_1", RunnerID: "runner-1", Name: "test",
		ImageRef: "image:test", Status: model.SandboxStatusRunning, CPU: 1, MemoryMB: 2048, DiskGB: 40,
	}).Error; err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{APIToken: "api-token", RunnerAPIToken: runnerToken, PreviewPasswordKey: "preview-password-key"}
	s := &Server{db: db, cfg: cfg, client: NewRunnerClient(cfg.RunnerAPIToken)}
	req := httptest.NewRequest(http.MethodPost, "/v1/sandboxes/sbx_stop/stop", nil)
	req.Header.Set("Authorization", "Bearer api-token")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}

	var sb model.Sandbox
	if err := db.First(&sb, "id = ?", "sbx_stop").Error; err != nil {
		t.Fatal(err)
	}
	if sb.Status != model.SandboxStatusStopping {
		t.Fatalf("status = %q, want stopping tombstone", sb.Status)
	}
	var runner model.Runner
	if err := db.First(&runner, "id = ?", "runner-1").Error; err != nil {
		t.Fatal(err)
	}
	if runner.ReservedCPU != 1 || runner.ReservedMemoryMB != 2048 || runner.ReservedDiskGB != 40 {
		t.Fatalf("runner reservation changed after failed stop: %+v", runner)
	}
}

func TestStopSandboxReleasesRuntimeReservationKeepsDisk(t *testing.T) {
	const runnerToken = "runner-token"
	runnerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sandboxes/sbx_stop_success/stop" {
			t.Fatalf("runner path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
	}))
	defer runnerServer.Close()

	db := newTemplateControlTestDB(t)
	if err := db.Create(&model.Runner{
		ID: "runner-1", Name: "runner-1", APIURL: runnerServer.URL, AuthTokenHash: []byte("hash"),
		Status: model.RunnerStatusHealthy, TotalCPU: 8, TotalMemoryMB: 16384, TotalDiskGB: 100,
		CPUOvercommit: 1.5, MemoryOvercommit: 1, DiskOvercommit: 1,
		ReservedCPU: 1, ReservedMemoryMB: 2048, ReservedDiskGB: 40,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Sandbox{
		ID: "sbx_stop_success", OrgID: "org_1", RunnerID: "runner-1", Name: "test",
		ImageRef: "image:test", Status: model.SandboxStatusRunning, CPU: 1, MemoryMB: 2048, DiskGB: 40,
	}).Error; err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{APIToken: "api-token", RunnerAPIToken: runnerToken, PreviewPasswordKey: "preview-password-key"}
	s := &Server{db: db, cfg: cfg, client: NewRunnerClient(cfg.RunnerAPIToken)}
	req := httptest.NewRequest(http.MethodPost, "/v1/sandboxes/sbx_stop_success/stop", nil)
	req.Header.Set("Authorization", "Bearer api-token")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}

	var runner model.Runner
	if err := db.First(&runner, "id = ?", "runner-1").Error; err != nil {
		t.Fatal(err)
	}
	if runner.ReservedCPU != 0 || runner.ReservedMemoryMB != 0 || runner.ReservedDiskGB != 40 {
		t.Fatalf("runner reserved = cpu %d memory %d disk %d, want cpu 0 memory 0 disk 40",
			runner.ReservedCPU, runner.ReservedMemoryMB, runner.ReservedDiskGB)
	}
}

func TestStopPublishesTombstoneBeforeStoppingRunner(t *testing.T) {
	const runnerToken = "runner-token"
	var tombstonePublished atomic.Bool
	cacheServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var route previewCacheRoute
		if err := json.NewDecoder(r.Body).Decode(&route); err != nil {
			t.Fatalf("decode route: %v", err)
		}
		if route.Status == model.SandboxStatusStopping {
			tombstonePublished.Store(true)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer cacheServer.Close()
	runnerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !tombstonePublished.Load() {
			t.Fatal("runner stop called before preview tombstone was published")
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
	}))
	defer runnerServer.Close()

	db := newTemplateControlTestDB(t)
	if err := db.Create(&model.Runner{
		ID: "runner-1", Name: "runner-1", APIURL: runnerServer.URL, PreviewBaseURL: "http://10.80.1.2",
		AuthTokenHash: []byte("hash"), Status: model.RunnerStatusHealthy,
		TotalCPU: 8, TotalMemoryMB: 16384, TotalDiskGB: 100, CPUOvercommit: 1.5, MemoryOvercommit: 1, DiskOvercommit: 1,
		ReservedCPU: 1, ReservedMemoryMB: 2048, ReservedDiskGB: 40,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Sandbox{
		ID: "sbx_ordered_stop", OrgID: "org_1", RunnerID: "runner-1", Name: "test",
		ImageRef: "image:test", Status: model.SandboxStatusRunning, CPU: 1, MemoryMB: 2048, DiskGB: 40,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.SandboxPort{
		ID: "port-ordered-stop", SandboxID: "sbx_ordered_stop", GuestPort: 7080, HostPort: 30418, Protocol: "http",
	}).Error; err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{
		APIToken: "api-token", RunnerAPIToken: runnerToken, PreviewPasswordKey: "preview-password-key",
		PreviewCacheURL: cacheServer.URL, PreviewCacheToken: "cache-token",
	}
	s := &Server{
		db: db, cfg: cfg, client: NewRunnerClient(cfg.RunnerAPIToken),
		previewCache: NewPreviewCacheClient(t.Context(), cfg),
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/sandboxes/sbx_ordered_stop/stop", nil)
	req.Header.Set("Authorization", "Bearer api-token")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !tombstonePublished.Load() {
		t.Fatal("stopping tombstone was not published")
	}
}

func TestStopAbortsWhenTombstonePublicationFails(t *testing.T) {
	var runnerCalls atomic.Int32
	cacheServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "cache unavailable", http.StatusServiceUnavailable)
	}))
	defer cacheServer.Close()
	runnerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		runnerCalls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer runnerServer.Close()

	db := newTemplateControlTestDB(t)
	if err := db.Create(&model.Runner{
		ID: "runner-1", Name: "runner-1", APIURL: runnerServer.URL, PreviewBaseURL: "http://10.80.1.2",
		AuthTokenHash: []byte("hash"), Status: model.RunnerStatusHealthy,
		TotalCPU: 8, TotalMemoryMB: 16384, TotalDiskGB: 100,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Sandbox{
		ID: "sbx_cache_failure", OrgID: "org_1", RunnerID: "runner-1", Name: "test",
		ImageRef: "image:test", Status: model.SandboxStatusRunning, CPU: 1, MemoryMB: 2048, DiskGB: 40,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.SandboxPort{
		ID: "port-cache-failure", SandboxID: "sbx_cache_failure", GuestPort: 7080, HostPort: 30418, Protocol: "http",
	}).Error; err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		APIToken: "api-token", RunnerAPIToken: "runner-token", PreviewPasswordKey: "preview-password-key",
		PreviewCacheURL: cacheServer.URL, PreviewCacheToken: "cache-token",
	}
	s := &Server{
		db: db, cfg: cfg, client: NewRunnerClient(cfg.RunnerAPIToken),
		previewCache: NewPreviewCacheClient(t.Context(), cfg),
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/sandboxes/sbx_cache_failure/stop", nil)
	req.Header.Set("Authorization", "Bearer api-token")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if runnerCalls.Load() != 0 {
		t.Fatalf("runner calls = %d, want 0", runnerCalls.Load())
	}
	var sb model.Sandbox
	if err := db.First(&sb, "id = ?", "sbx_cache_failure").Error; err != nil {
		t.Fatal(err)
	}
	if sb.Status != model.SandboxStatusStopping {
		t.Fatalf("status = %q, want stopping", sb.Status)
	}
}
