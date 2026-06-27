package control

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/microsandbox/config"
	"github.com/usehivy/hivy/internal/microsandbox/model"
)

func TestDeleteStoppedSandboxOnlyReleasesDiskReservation(t *testing.T) {
	const runnerToken = "runner-token"
	runnerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sandboxes/sbx_delete/delete" {
			t.Fatalf("runner path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
	}))
	defer runnerServer.Close()

	db := newTemplateControlTestDB(t)
	if err := db.Create(&model.Runner{
		ID: "runner-1", Name: "runner-1", APIURL: runnerServer.URL, PreviewBaseURL: "http://10.80.1.2", AuthTokenHash: []byte("hash"),
		Status: model.RunnerStatusHealthy, TotalCPU: 8, TotalMemoryMB: 16384, TotalDiskGB: 100,
		CPUOvercommit: 1.5, MemoryOvercommit: 1, DiskOvercommit: 1,
		ReservedCPU: 2, ReservedMemoryMB: 4096, ReservedDiskGB: 80,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Sandbox{
		ID: "sbx_delete", OrgID: "org_1", RunnerID: "runner-1", Name: "test",
		ImageRef: "image:test", Status: model.SandboxStatusStopped, CPU: 4, MemoryMB: 8192, DiskGB: 40,
	}).Error; err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{APIToken: "api-token", RunnerAPIToken: runnerToken, PreviewPasswordKey: "preview-password-key"}
	s := &Server{db: db, cfg: cfg, client: NewRunnerClient(cfg.RunnerAPIToken)}
	req := httptest.NewRequest(http.MethodDelete, "/v1/sandboxes/sbx_delete", nil)
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
	if runner.ReservedCPU != 2 || runner.ReservedMemoryMB != 4096 || runner.ReservedDiskGB != 40 {
		t.Fatalf("runner reserved = cpu %d memory %d disk %d, want cpu 2 memory 4096 disk 40",
			runner.ReservedCPU, runner.ReservedMemoryMB, runner.ReservedDiskGB)
	}
}

func TestBulkActivityExtendsSleepAndReturnsLease(t *testing.T) {
	db := newTemplateControlTestDB(t)
	old := time.Now().UTC().Add(-10 * time.Minute)
	due := time.Now().UTC().Add(-time.Minute)
	if err := db.Create(&model.Runner{
		ID: "runner-1", Name: "runner-1", APIURL: "http://runner", PreviewBaseURL: "http://10.80.1.2", AuthTokenHash: []byte("hash"),
		Status: model.RunnerStatusHealthy, TotalCPU: 8, TotalMemoryMB: 16384, TotalDiskGB: 100,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Sandbox{
		ID: "sbx_activity", OrgID: "org_1", RunnerID: "runner-1", Name: "test",
		ImageRef: "image:test", Status: model.SandboxStatusRunning, CPU: 1, MemoryMB: 2048, DiskGB: 40,
		AutoSleepAfterSeconds: 300, LastGatewayActivityAt: &old, CreatedAt: old, SleepAfterAt: &due,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.SandboxPort{ID: "port-1", SandboxID: "sbx_activity", GuestPort: 3000, HostPort: 43000, Protocol: "http"}).Error; err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{APIToken: "api-token", RunnerAPIToken: "runner-token", PreviewPasswordKey: "preview-password-key"}
	s := &Server{db: db, cfg: cfg, client: NewRunnerClient(cfg.RunnerAPIToken)}
	req := httptest.NewRequest(http.MethodPost, "/v1/sandboxes/activity/bulk", strings.NewReader(`{
		"source":"gateway",
		"sandbox_ids":["sbx_activity"]
	}`))
	req.Header.Set("Authorization", "Bearer api-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var body sandboxActivityBulkResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Routes) != 1 || body.Routes[0].LeaseExpiresAt.IsZero() || body.Routes[0].NextActivityAfter.IsZero() {
		t.Fatalf("routes = %+v", body.Routes)
	}
	var sb model.Sandbox
	if err := db.First(&sb, "id = ?", "sbx_activity").Error; err != nil {
		t.Fatal(err)
	}
	if sb.SleepAfterAt == nil || !sb.SleepAfterAt.After(time.Now()) {
		t.Fatalf("sleep_after_at not extended: %+v", sb)
	}
}
