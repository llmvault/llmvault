package control

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/microsandbox/config"
	"github.com/usehivy/hivy/internal/microsandbox/model"
)

func TestUpgradeSandboxPreservesRouteAndUpdatesImage(t *testing.T) {
	const runnerToken = "runner-token"
	var gotReq runnerUpgradeSandboxRequest
	runnerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sandboxes/sbx_upgrade/upgrade" {
			t.Fatalf("runner path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer "+runnerToken {
			t.Fatalf("runner auth = %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decode runner request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(runnerUpgradeSandboxResponse{
			ID: "sbx_upgrade", Status: "running", Ports: gotReq.PortBindings,
		})
	}))
	defer runnerServer.Close()

	db := newTemplateControlTestDB(t)
	if err := db.Create(&model.Runner{
		ID: "runner-1", Name: "runner-1", APIURL: runnerServer.URL, AuthTokenHash: []byte("hash"),
		Status: model.RunnerStatusHealthy, TotalCPU: 8, TotalMemoryMB: 16384, TotalDiskGB: 100,
		CPUOvercommit: 1, MemoryOvercommit: 1, DiskOvercommit: 1,
		ReservedCPU: 1, ReservedMemoryMB: 2048, ReservedDiskGB: 40,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Sandbox{
		ID: "sbx_upgrade", OrgID: "org_1", RunnerID: "runner-1", Name: "test",
		ImageRef: "image:old", Status: model.SandboxStatusRunning, CPU: 1, MemoryMB: 2048,
		DiskGB: 40, ActivityToken: "activity-token",
	}).Error; err != nil {
		t.Fatal(err)
	}
	ports := []model.SandboxPort{
		{ID: "port-runtime", SandboxID: "sbx_upgrade", GuestPort: 7080, HostPort: 47080, Protocol: "http"},
		{ID: "port-preview", SandboxID: "sbx_upgrade", GuestPort: 3000, HostPort: 43000, Protocol: "http"},
	}
	if err := db.Create(&ports).Error; err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{APIToken: "api-token", RunnerAPIToken: runnerToken, PreviewPasswordKey: "preview-password-key"}
	s := &Server{db: db, cfg: cfg, client: NewRunnerClient(cfg.RunnerAPIToken)}
	req := httptest.NewRequest(http.MethodPost, "/v1/sandboxes/sbx_upgrade/upgrade", strings.NewReader(`{
		"image_ref":"image:new",
		"cpu":2,
		"memory_mb":4096,
		"disk_gb":40,
		"preview_ports":[7080,3000],
		"env":{"CUSTOM":"value"},
		"metadata":{"agent_id":"agent-1","sandbox_size":"large"}
	}`))
	req.Header.Set("Authorization", "Bearer api-token")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if gotReq.ImageRef != "image:new" || gotReq.PreviousImageRef != "image:old" {
		t.Fatalf("runner image refs = new %q previous %q", gotReq.ImageRef, gotReq.PreviousImageRef)
	}
	if gotReq.Env["HIVY_MICROSANDBOX_ID"] != "sbx_upgrade" || gotReq.Env["HIVY_MICROSANDBOX_ACTIVITY_TOKEN"] != "activity-token" {
		t.Fatalf("runner env missing microsandbox identity: %+v", gotReq.Env)
	}
	if gotReq.Labels["sandbox_id"] != "sbx_upgrade" || gotReq.Labels["agent_id"] != "agent-1" {
		t.Fatalf("runner labels = %+v", gotReq.Labels)
	}
	bindings := map[int]int{}
	for _, binding := range gotReq.PortBindings {
		bindings[binding.GuestPort] = binding.HostPort
	}
	if len(bindings) != 2 || bindings[7080] != 47080 || bindings[3000] != 43000 {
		t.Fatalf("runner port bindings = %+v", gotReq.PortBindings)
	}

	var sb model.Sandbox
	if err := db.First(&sb, "id = ?", "sbx_upgrade").Error; err != nil {
		t.Fatal(err)
	}
	if sb.ImageRef != "image:new" || sb.Status != model.SandboxStatusRunning || sb.CPU != 2 || sb.MemoryMB != 4096 {
		t.Fatalf("sandbox after upgrade = %+v", sb)
	}
	var runner model.Runner
	if err := db.First(&runner, "id = ?", "runner-1").Error; err != nil {
		t.Fatal(err)
	}
	if runner.ReservedCPU != 2 || runner.ReservedMemoryMB != 4096 || runner.ReservedDiskGB != 40 {
		t.Fatalf("runner reserved = cpu %d memory %d disk %d", runner.ReservedCPU, runner.ReservedMemoryMB, runner.ReservedDiskGB)
	}
}
