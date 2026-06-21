package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/microsandbox/api"
	"github.com/usehivy/hivy/internal/microsandbox/config"
	"github.com/usehivy/hivy/internal/microsandbox/model"
)

func TestCreateSandboxPushesPreviewCacheRoute(t *testing.T) {
	const runnerToken = "runner-token"
	const cacheToken = "cache-token"

	var runnerPreviewPorts []int
	var runnerInit *sandboxInitConfig
	var runnerAutoStartDocker *bool
	runner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sandboxes" {
			t.Fatalf("runner path = %s, want /v1/sandboxes", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer "+runnerToken {
			t.Fatalf("runner auth = %q", r.Header.Get("Authorization"))
		}
		var req runnerCreateSandboxRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		runnerPreviewPorts = req.PreviewPorts
		runnerInit = req.Init
		runnerAutoStartDocker = req.AutoStartDocker
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": req.ID,
			"ports": []map[string]int{
				{"guest_port": 3000, "host_port": 43000},
				{"guest_port": 5173, "host_port": 45173},
				{"guest_port": 7080, "host_port": 47080},
				{"guest_port": 8000, "host_port": 48000},
				{"guest_port": 8080, "host_port": 48080},
			},
		})
	}))
	defer runner.Close()

	routeCh := make(chan previewCacheRoute, 1)
	cache := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/routes/") {
			t.Fatalf("cache path = %s, want /v1/routes/{sandbox}", r.URL.Path)
		}
		sandboxID := strings.TrimPrefix(r.URL.Path, "/v1/routes/")
		if len(sandboxID) != 8 {
			t.Fatalf("cache route sandbox id = %q, want 8 characters", sandboxID)
		}
		if r.Method != http.MethodPut {
			t.Fatalf("cache method = %s, want PUT", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer "+cacheToken {
			t.Fatalf("cache auth = %q", r.Header.Get("Authorization"))
		}
		var route previewCacheRoute
		if err := json.NewDecoder(r.Body).Decode(&route); err != nil {
			t.Fatal(err)
		}
		routeCh <- route
		_ = json.NewEncoder(w).Encode(route)
	}))
	defer cache.Close()

	db, err := gorm.Open(sqlite.Open("file:create-sandbox-cache?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Runner{}, &model.OrgPreviewSecret{}, &model.Sandbox{}, &model.SandboxPort{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Runner{
		ID: "runner-1", Name: "runner-1", APIURL: runner.URL, PreviewBaseURL: "http://10.80.1.2", AuthTokenHash: []byte("hash"),
		Status: model.RunnerStatusHealthy, TotalCPU: 8, TotalMemoryMB: 16384, TotalDiskGB: 200, CPUOvercommit: 1.5,
	}).Error; err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{
		APIToken:           "api-token",
		RunnerAPIToken:     runnerToken,
		PreviewPasswordKey: "preview-password-key",
		PreviewBaseDomain:  "preview.test",
		PreviewCacheURL:    cache.URL,
		PreviewCacheToken:  cacheToken,
	}
	s := &Server{db: db, cfg: cfg, client: NewRunnerClient(cfg.RunnerAPIToken), previewCache: NewPreviewCacheClient(context.Background(), cfg)}

	req := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{
			"org_id":"org_1",
			"name":"cache-test",
			"image_ref":"ghcr.io/usehivy/runtime:test",
			"size":"small",
			"auto_start_docker":false,
			"health_checks":[{"guest_port":7080,"type":"http","method":"GET","path":"/healthz","expected_status":200}],
			"init":{"cmd":"/usr/local/bin/hivy-runtime-entrypoint","args":["/usr/local/bin/hivy-sandboxes-runtime"]}
		}`))
	req.Header.Set("Authorization", "Bearer api-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d body = %s", rec.Code, rec.Body.String())
	}
	if got, want := runnerPreviewPorts, api.DefaultPreviewPorts(); !equalInts(got, want) {
		t.Fatalf("runner preview ports = %v, want %v", got, want)
	}
	if runnerInit == nil {
		t.Fatal("runner init is nil")
	}
	if runnerInit.Cmd != "/usr/local/bin/hivy-runtime-entrypoint" {
		t.Fatalf("runner init cmd = %q", runnerInit.Cmd)
	}
	if got, want := runnerInit.Args, []string{"/usr/local/bin/hivy-sandboxes-runtime"}; !equalStrings(got, want) {
		t.Fatalf("runner init args = %v, want %v", got, want)
	}
	if runnerAutoStartDocker == nil || *runnerAutoStartDocker {
		t.Fatalf("runner auto_start_docker = %v, want false", runnerAutoStartDocker)
	}
	var runtimePort model.SandboxPort
	if err := db.First(&runtimePort, "guest_port = ?", 7080).Error; err != nil {
		t.Fatal(err)
	}
	if runtimePort.HealthCheckType != "http" || runtimePort.HealthCheckMethod != "GET" ||
		runtimePort.HealthCheckPath != "/healthz" || runtimePort.HealthCheckExpectedStatus != 200 ||
		runtimePort.HealthCheckTimeoutSeconds != defaultHealthCheckTimeoutSeconds ||
		runtimePort.HealthCheckIntervalMS != defaultHealthCheckIntervalMS {
		t.Fatalf("runtime port health check = %+v", runtimePort)
	}

	select {
	case route := <-routeCh:
		if route.Status != model.SandboxStatusRunning {
			t.Fatalf("route status = %q, want running", route.Status)
		}
		if route.AutoSleepAfterSeconds != defaultAutoSleepAfter || route.SleepAfterAt == nil || route.LeaseExpiresAt.IsZero() || route.NextActivityAfter.IsZero() {
			t.Fatalf("route lease metadata missing: %+v", route)
		}
		want := map[string]string{
			"3000": "http://10.80.1.2:43000",
			"5173": "http://10.80.1.2:45173",
			"7080": "http://10.80.1.2:47080",
			"8000": "http://10.80.1.2:48000",
			"8080": "http://10.80.1.2:48080",
		}
		if len(route.Upstreams) != len(want) {
			t.Fatalf("route upstreams = %#v", route.Upstreams)
		}
		for port, upstream := range want {
			if route.Upstreams[port] != upstream {
				t.Fatalf("route upstream %s = %q, want %q", port, route.Upstreams[port], upstream)
			}
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for preview cache route")
	}
}

func TestCreateSandboxRejectsHealthCheckForUnpreviewedPort(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:create-sandbox-health-invalid?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Runner{}, &model.OrgPreviewSecret{}, &model.Sandbox{}, &model.SandboxPort{}); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{APIToken: "api-token", RunnerAPIToken: "runner-token", PreviewPasswordKey: "preview-password-key"}
	s := &Server{db: db, cfg: cfg, client: NewRunnerClient(cfg.RunnerAPIToken)}

	req := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{
		"org_id":"org_1",
		"name":"invalid-health",
		"image_ref":"ghcr.io/usehivy/runtime:test",
		"preview_ports":[7080],
		"health_checks":[{"guest_port":9999,"type":"http","method":"GET","path":"/healthz","expected_status":200}]
	}`))
	req.Header.Set("Authorization", "Bearer api-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "not previewable") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestPreviewCacheRouteBuildsDirectUpstreams(t *testing.T) {
	route, err := previewCacheRouteFor(
		model.Sandbox{ID: "abc123xy", Status: model.SandboxStatusRunning},
		model.Runner{ID: "runner-1", PreviewBaseURL: "http://10.80.1.2"},
		[]model.SandboxPort{
			{GuestPort: 3000, HostPort: 43122},
			{GuestPort: 5173, HostPort: 45173},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if route.Upstreams["3000"] != "http://10.80.1.2:43122" || route.Upstreams["5173"] != "http://10.80.1.2:45173" {
		t.Fatalf("upstreams = %#v", route.Upstreams)
	}
	if route.LeaseExpiresAt.IsZero() || route.NextActivityAfter.IsZero() {
		t.Fatalf("lease metadata missing: %+v", route)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
