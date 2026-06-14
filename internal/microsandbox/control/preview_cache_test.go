package control

import (
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
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": req.ID,
			"ports": []map[string]int{
				{"guest_port": 3000, "host_port": 43000},
				{"guest_port": 5173, "host_port": 45173},
				{"guest_port": 8080, "host_port": 48080},
			},
		})
	}))
	defer runner.Close()

	routeCh := make(chan previewCacheRoute, 1)
	cache := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/routes/sbx_") {
			t.Fatalf("cache path = %s, want /v1/routes/{sandbox}", r.URL.Path)
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
		ID: "runner-1", Name: "runner-1", APIURL: runner.URL, AuthTokenHash: []byte("hash"),
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
	s := &Server{db: db, cfg: cfg, client: NewRunnerClient(cfg.RunnerAPIToken), previewCache: NewPreviewCacheClient(cfg)}

	req := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{
		"org_id":"org_1",
		"name":"cache-test",
		"image_ref":"ghcr.io/usehivy/runtime:test",
		"size":"small"
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

	select {
	case route := <-routeCh:
		if route.RunnerPrivateURL != runner.URL {
			t.Fatalf("route runner url = %q, want %q", route.RunnerPrivateURL, runner.URL)
		}
		if route.Status != model.SandboxStatusRunning {
			t.Fatalf("route status = %q, want running", route.Status)
		}
		if !equalInts(route.Ports, []int{3000, 5173, 8080}) {
			t.Fatalf("route ports = %v", route.Ports)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for preview cache route")
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
