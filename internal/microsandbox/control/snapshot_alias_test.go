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
)

func TestValidateSnapshotAlias(t *testing.T) {
	valid := []string{"runtime", "runtime-latest", "hivy-runtime-v3-1-18-amd64-medium-v1", "a1-b2"}
	for _, alias := range valid {
		if err := validateSnapshotAlias(alias); err != nil {
			t.Fatalf("validateSnapshotAlias(%q): %v", alias, err)
		}
	}
	invalid := []string{"Runtime", "runtime_latest", "runtime--latest", "-runtime", "runtime-", "runtime.latest"}
	for _, alias := range invalid {
		if err := validateSnapshotAlias(alias); err == nil {
			t.Fatalf("validateSnapshotAlias(%q) unexpectedly passed", alias)
		}
	}
}

func TestCreateSandboxCanUseGlobalSnapshotAliasAcrossOrgs(t *testing.T) {
	const runnerToken = "runner-token"

	reqCh := make(chan runnerCreateSandboxRequest, 1)
	runner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	defer runner.Close()

	db := newSnapshotAliasTestDB(t)
	createSnapshotAliasRunner(t, db, runner.URL)
	if err := db.Create(&model.Snapshot{
		ID: "snp-global", OrgID: "org_system", RunnerID: "runner-1", Name: "global runtime",
		Alias: "hivy-runtime-latest", Global: true, BaseImageRef: "ghcr.io/usehivy/runtime:test",
		Status: model.SnapshotStatusReady, ArtifactURL: "s3://bucket/snp-global.tar.zst",
		ArtifactDigest: "sha256:artifact", ImageManifestDigest: "sha256:image",
	}).Error; err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{APIToken: "api-token", RunnerAPIToken: runnerToken, PreviewPasswordKey: "preview-password-key"}
	s := &Server{db: db, cfg: cfg, client: NewRunnerClient(cfg.RunnerAPIToken)}
	req := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{
		"org_id":"org_customer",
		"name":"from-global-alias",
		"snapshot_id":"hivy-runtime-latest",
		"size":"small"
	}`))
	req.Header.Set("Authorization", "Bearer api-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d body = %s", rec.Code, rec.Body.String())
	}

	got := <-reqCh
	if got.SnapshotID != "snp-global" {
		t.Fatalf("runner snapshot id = %q, want snp-global", got.SnapshotID)
	}
	if got.ImageRef != "ghcr.io/usehivy/runtime:test" {
		t.Fatalf("runner image ref = %q", got.ImageRef)
	}
}

func TestCreateSandboxRejectsScopedSnapshotAliasForAnotherOrg(t *testing.T) {
	runner := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("runner should not be called for forbidden snapshot")
	}))
	defer runner.Close()

	db := newSnapshotAliasTestDB(t)
	createSnapshotAliasRunner(t, db, runner.URL)
	if err := db.Create(&model.Snapshot{
		ID: "snp-scoped", OrgID: "org_owner", RunnerID: "runner-1", Name: "scoped runtime",
		Alias: "owner-runtime", BaseImageRef: "ghcr.io/usehivy/runtime:test", Status: model.SnapshotStatusReady,
	}).Error; err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{APIToken: "api-token", RunnerAPIToken: "runner-token", PreviewPasswordKey: "preview-password-key"}
	s := &Server{db: db, cfg: cfg, client: NewRunnerClient(cfg.RunnerAPIToken)}
	req := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{
		"org_id":"org_customer",
		"name":"from-scoped-alias",
		"snapshot_id":"owner-runtime",
		"size":"small"
	}`))
	req.Header.Set("Authorization", "Bearer api-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("create status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestCreateSnapshotRejectsInvalidAndDuplicateAliases(t *testing.T) {
	db := newSnapshotAliasTestDB(t)
	if err := db.Create(&model.Snapshot{
		ID: "snp-existing", OrgID: "org_1", RunnerID: "runner-1", Name: "existing",
		Alias: "runtime-latest", BaseImageRef: "ghcr.io/usehivy/runtime:test", Status: model.SnapshotStatusReady,
	}).Error; err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{APIToken: "api-token", PreviewPasswordKey: "preview-password-key"}
	s := &Server{db: db, cfg: cfg}

	for _, tc := range []struct {
		name string
		body string
		want int
	}{
		{name: "invalid", body: `{"org_id":"org_1","base_image_ref":"image:test","alias":"Runtime_Latest"}`, want: http.StatusBadRequest},
		{name: "duplicate", body: `{"org_id":"org_1","base_image_ref":"image:test","alias":"runtime-latest"}`, want: http.StatusConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/snapshots", strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer api-token")
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			s.Routes().ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("create snapshot status = %d body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func newSnapshotAliasTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Runner{}, &model.OrgPreviewSecret{}, &model.Sandbox{}, &model.SandboxPort{}, &model.Snapshot{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func createSnapshotAliasRunner(t *testing.T, db *gorm.DB, apiURL string) {
	t.Helper()
	if err := db.Create(&model.Runner{
		ID: "runner-1", Name: "runner-1", APIURL: apiURL, PreviewBaseURL: "http://10.80.1.2", AuthTokenHash: []byte("hash"),
		Status: model.RunnerStatusHealthy, TotalCPU: 8, TotalMemoryMB: 16384, TotalDiskGB: 200, CPUOvercommit: 1.5,
	}).Error; err != nil {
		t.Fatal(err)
	}
}
