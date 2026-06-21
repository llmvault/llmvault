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
		t.Fatalf("runner reserved = cpu %d memory %d disk %d, want cpu 1 memory 2048 disk 40",
			runner.ReservedCPU, runner.ReservedMemoryMB, runner.ReservedDiskGB)
	}
}

func TestFailedStopDoesNotMarkSandboxStopped(t *testing.T) {
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
	if sb.Status != model.SandboxStatusRunning {
		t.Fatalf("status = %q, want still running", sb.Status)
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

func TestEnsureReadyCallsRunnerAndReturnsRoute(t *testing.T) {
	const runnerToken = "runner-token"
	var gotReq runnerEnsureReadyRequest
	runnerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sandboxes/sbx_ready/ensure-ready" {
			t.Fatalf("runner path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer "+runnerToken {
			t.Fatalf("runner auth = %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(runnerEnsureReadyResponse{Status: "running", HostPort: 47080})
	}))
	defer runnerServer.Close()

	db := newTemplateControlTestDB(t)
	if err := db.Create(&model.Runner{
		ID: "runner-1", Name: "runner-1", APIURL: runnerServer.URL, PreviewBaseURL: "http://10.80.1.2", AuthTokenHash: []byte("hash"),
		Status: model.RunnerStatusHealthy, TotalCPU: 8, TotalMemoryMB: 16384, TotalDiskGB: 100,
		CPUOvercommit: 1.5, MemoryOvercommit: 1, DiskOvercommit: 1, ReservedDiskGB: 40,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Sandbox{
		ID: "sbx_ready", OrgID: "org_1", RunnerID: "runner-1", Name: "test",
		ImageRef: "image:test", Status: model.SandboxStatusStopped, CPU: 1, MemoryMB: 2048, DiskGB: 40,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.SandboxPort{
		ID: "port-1", SandboxID: "sbx_ready", GuestPort: 7080, HostPort: 47080, Protocol: "http",
		HealthCheckType: "http", HealthCheckMethod: "GET", HealthCheckPath: "/healthz",
		HealthCheckExpectedStatus: 200, HealthCheckTimeoutSeconds: 30, HealthCheckIntervalMS: 250,
	}).Error; err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{APIToken: "api-token", RunnerAPIToken: runnerToken, PreviewPasswordKey: "preview-password-key"}
	s := &Server{db: db, cfg: cfg, client: NewRunnerClient(cfg.RunnerAPIToken)}
	req := httptest.NewRequest(http.MethodPost, "/v1/sandboxes/sbx_ready/ensure-ready", strings.NewReader(`{
		"guest_port":7080,
		"readiness":"runtime_ready",
		"timeout_seconds":5
	}`))
	req.Header.Set("Authorization", "Bearer api-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if gotReq.GuestPort != 7080 || gotReq.TimeoutSeconds != 5 {
		t.Fatalf("runner ensure request = %+v", gotReq)
	}
	if gotReq.HealthCheck == nil || gotReq.HealthCheck.Path != "/healthz" || gotReq.HealthCheck.ExpectedStatus != 200 {
		t.Fatalf("runner health check = %+v", gotReq.HealthCheck)
	}
	var body sandboxRouteResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Route.Upstreams["7080"] != "http://10.80.1.2:47080" {
		t.Fatalf("route = %+v", body.Route)
	}
	var sb model.Sandbox
	if err := db.First(&sb, "id = ?", "sbx_ready").Error; err != nil {
		t.Fatal(err)
	}
	if sb.Status != model.SandboxStatusRunning || sb.LastWakeAt == nil || sb.LastGatewayActivityAt == nil {
		t.Fatalf("sandbox not marked ready: %+v", sb)
	}
	var runner model.Runner
	if err := db.First(&runner, "id = ?", "runner-1").Error; err != nil {
		t.Fatal(err)
	}
	if runner.ReservedCPU != 1 || runner.ReservedMemoryMB != 2048 || runner.ReservedDiskGB != 40 {
		t.Fatalf("runner reserved = cpu %d memory %d disk %d, want cpu 1 memory 2048 disk 40",
			runner.ReservedCPU, runner.ReservedMemoryMB, runner.ReservedDiskGB)
	}
}

func TestEnsureReadyHealthCheckFailureRecordsWakeError(t *testing.T) {
	const runnerToken = "runner-token"
	runnerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sandboxes/sbx_health_fail/ensure-ready" {
			t.Fatalf("runner path = %s", r.URL.Path)
		}
		http.Error(w, `{"error":"health check GET /healthz did not return 200: status=503"}`, http.StatusInternalServerError)
	}))
	defer runnerServer.Close()

	db := newTemplateControlTestDB(t)
	if err := db.Create(&model.Runner{
		ID: "runner-1", Name: "runner-1", APIURL: runnerServer.URL, PreviewBaseURL: "http://10.80.1.2", AuthTokenHash: []byte("hash"),
		Status: model.RunnerStatusHealthy, TotalCPU: 8, TotalMemoryMB: 16384, TotalDiskGB: 100,
		CPUOvercommit: 1.5, MemoryOvercommit: 1, DiskOvercommit: 1, ReservedDiskGB: 40,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Sandbox{
		ID: "sbx_health_fail", OrgID: "org_1", RunnerID: "runner-1", Name: "test",
		ImageRef: "image:test", Status: model.SandboxStatusStopped, CPU: 1, MemoryMB: 2048, DiskGB: 40,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.SandboxPort{
		ID: "port-1", SandboxID: "sbx_health_fail", GuestPort: 7080, HostPort: 47080, Protocol: "http",
		HealthCheckType: "http", HealthCheckMethod: "GET", HealthCheckPath: "/healthz",
		HealthCheckExpectedStatus: 200, HealthCheckTimeoutSeconds: 30, HealthCheckIntervalMS: 250,
	}).Error; err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{APIToken: "api-token", RunnerAPIToken: runnerToken, PreviewPasswordKey: "preview-password-key"}
	s := &Server{db: db, cfg: cfg, client: NewRunnerClient(cfg.RunnerAPIToken)}
	req := httptest.NewRequest(http.MethodPost, "/v1/sandboxes/sbx_health_fail/ensure-ready", strings.NewReader(`{
		"guest_port":7080,
		"timeout_seconds":5
	}`))
	req.Header.Set("Authorization", "Bearer api-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var sb model.Sandbox
	if err := db.First(&sb, "id = ?", "sbx_health_fail").Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sb.LastWakeError, "health check") {
		t.Fatalf("last wake error = %q", sb.LastWakeError)
	}
}
