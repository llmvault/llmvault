package control

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/microsandbox/config"
	"github.com/usehivy/hivy/internal/microsandbox/model"
	"github.com/usehivy/hivy/internal/microsandbox/runner"
)

func TestCreateTemplateStoresProviderLocalImageMetadata(t *testing.T) {
	const runnerToken = "runner-token"

	var buildReq runner.CreateTemplateRequest
	buildRunner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/templates" {
			t.Fatalf("runner path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer "+runnerToken {
			t.Fatalf("runner auth = %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&buildReq); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte(`{"type":"log","message":"installing java"}` + "\n"))
		_, _ = w.Write([]byte(`{"type":"result","id":"` + buildReq.ID + `","image_ref":"10.80.0.3:5000/images/org_1/` + buildReq.ID + `@sha256:abc","image_digest":"sha256:abc","validation_sandbox_id":"val-test"}` + "\n"))
	}))
	defer buildRunner.Close()

	db := newTemplateControlTestDB(t)
	if err := db.Create(&model.Runner{
		ID: "runner-build", Name: "runner-build", APIURL: buildRunner.URL, AuthTokenHash: []byte("hash"),
		Status: model.RunnerStatusHealthy, TotalCPU: 8, TotalMemoryMB: 16384, TotalDiskGB: 100, CPUOvercommit: 1.5,
		MemoryOvercommit: 1, DiskOvercommit: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{APIToken: "api-token", RunnerAPIToken: runnerToken, PreviewPasswordKey: "preview-password-key"}
	s := &Server{db: db, cfg: cfg, client: NewRunnerClient(cfg.RunnerAPIToken)}
	req := httptest.NewRequest(http.MethodPost, "/v1/templates", strings.NewReader(`{
		"org_id":"org_1",
		"name":"java",
		"base_image_ref":"ghcr.io/usehivy/runtime:test",
		"commands":["apt-get update","apt-get install -y openjdk-21-jdk"],
		"size":"small"
	}`))
	req.Header.Set("Authorization", "Bearer api-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create template status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !strings.HasPrefix(buildReq.ID, "tpl_") {
		t.Fatalf("template id = %q, want tpl_ prefix", buildReq.ID)
	}
	if buildReq.BaseImageRef != "ghcr.io/usehivy/runtime:test" {
		t.Fatalf("base image = %q", buildReq.BaseImageRef)
	}

	var tmpl model.Template
	if err := db.First(&tmpl, "id = ?", buildReq.ID).Error; err != nil {
		t.Fatal(err)
	}
	if tmpl.Status != model.TemplateStatusReady {
		t.Fatalf("status = %q, want ready", tmpl.Status)
	}
	if tmpl.ImageRef == "" || tmpl.ImageDigest != "sha256:abc" {
		t.Fatalf("image metadata = (%q, %q)", tmpl.ImageRef, tmpl.ImageDigest)
	}
	if !strings.Contains(tmpl.Logs, "installing java") {
		t.Fatalf("logs = %q", tmpl.Logs)
	}
	if tmpl.RunnerID != "runner-build" {
		t.Fatalf("runner id = %q", tmpl.RunnerID)
	}
}

func TestCreateSandboxFromTemplateImageCanUseAnyRunner(t *testing.T) {
	const runnerToken = "runner-token"

	reqCh := make(chan runnerCreateSandboxRequest, 1)
	targetRunner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sandboxes" {
			t.Fatalf("runner path = %s", r.URL.Path)
		}
		var req runnerCreateSandboxRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		reqCh <- req
		_ = json.NewEncoder(w).Encode(map[string]any{"id": req.ID, "ports": []any{}})
	}))
	defer targetRunner.Close()

	buildRunner := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("template build runner should not be used for sandbox placement")
	}))
	defer buildRunner.Close()

	db := newTemplateControlTestDB(t)
	if err := db.Create(&model.Runner{
		ID: "runner-build", Name: "runner-build", APIURL: buildRunner.URL, AuthTokenHash: []byte("hash"),
		Status: model.RunnerStatusHealthy, Drain: true, TotalCPU: 64, TotalMemoryMB: 262144, TotalDiskGB: 2000,
		CPUOvercommit: 1.5, MemoryOvercommit: 1, DiskOvercommit: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Runner{
		ID: "runner-target", Name: "runner-target", APIURL: targetRunner.URL, AuthTokenHash: []byte("hash"),
		Status: model.RunnerStatusHealthy, TotalCPU: 4, TotalMemoryMB: 8192, TotalDiskGB: 100,
		CPUOvercommit: 1.5, MemoryOvercommit: 1, DiskOvercommit: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Template{
		ID: "tpl_ready", OrgID: "org_1", RunnerID: "runner-build", Name: "java",
		BaseImageRef: "ghcr.io/usehivy/runtime:test", Status: model.TemplateStatusReady,
		ImageRef: "10.80.0.3:5000/images/org_1/tpl_ready@sha256:abc", ImageDigest: "sha256:abc",
	}).Error; err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{APIToken: "api-token", RunnerAPIToken: runnerToken, PreviewPasswordKey: "preview-password-key"}
	s := &Server{db: db, cfg: cfg, client: NewRunnerClient(cfg.RunnerAPIToken)}
	req := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{
		"org_id":"org_1",
		"name":"from-template",
		"template_id":"tpl_ready",
		"size":"small"
	}`))
	req.Header.Set("Authorization", "Bearer api-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create sandbox status = %d body = %s", rec.Code, rec.Body.String())
	}

	got := <-reqCh
	if got.ImageRef != "10.80.0.3:5000/images/org_1/tpl_ready@sha256:abc" {
		t.Fatalf("image ref = %q", got.ImageRef)
	}
	if got.SnapshotID != "" || got.SnapshotArtifactURL != "" {
		t.Fatalf("snapshot fields should be empty: %+v", got)
	}
}

func newTemplateControlTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Runner{}, &model.OrgPreviewSecret{}, &model.Sandbox{}, &model.SandboxPort{}, &model.Snapshot{}, &model.Template{}); err != nil {
		t.Fatal(err)
	}
	return db
}
