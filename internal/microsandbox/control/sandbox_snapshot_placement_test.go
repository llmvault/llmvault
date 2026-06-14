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

func TestCreateSandboxFromExportedSnapshotCanUseAnotherRunner(t *testing.T) {
	const runnerToken = "runner-token"

	reqCh := make(chan runnerCreateSandboxRequest, 1)
	targetRunner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sandboxes" {
			t.Fatalf("runner path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer "+runnerToken {
			t.Fatalf("runner auth = %q", r.Header.Get("Authorization"))
		}
		var req runnerCreateSandboxRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		reqCh <- req
		_ = json.NewEncoder(w).Encode(map[string]any{"id": req.ID, "ports": []any{}})
	}))
	defer targetRunner.Close()

	originalRunner := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("original runner should not be called when snapshot has an artifact")
	}))
	defer originalRunner.Close()

	db, err := gorm.Open(sqlite.Open("file:snapshot-placement?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Runner{}, &model.OrgPreviewSecret{}, &model.Sandbox{}, &model.SandboxPort{}, &model.Snapshot{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Runner{
		ID: "runner-original", Name: "runner-original", APIURL: originalRunner.URL, AuthTokenHash: []byte("hash"),
		Status: model.RunnerStatusHealthy, TotalCPU: 64, TotalMemoryMB: 262144, TotalDiskGB: 2000, CPUOvercommit: 1.5,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Runner{
		ID: "runner-target", Name: "runner-target", APIURL: targetRunner.URL, AuthTokenHash: []byte("hash"),
		Status: model.RunnerStatusHealthy, TotalCPU: 4, TotalMemoryMB: 8192, TotalDiskGB: 100, CPUOvercommit: 1.5,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Snapshot{
		ID: "snp12345", OrgID: "org_1", RunnerID: "runner-original", Name: "snapshot",
		BaseImageRef: "ghcr.io/usehivy/runtime:test", Status: model.SnapshotStatusReady,
		ArtifactURL: "s3://bucket/snapshots/snp12345.tar.zst", ArtifactDigest: "sha256:abc",
		ImageManifestDigest: "sha256:image",
	}).Error; err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{APIToken: "api-token", RunnerAPIToken: runnerToken, PreviewPasswordKey: "preview-password-key"}
	s := &Server{db: db, cfg: cfg, client: NewRunnerClient(cfg.RunnerAPIToken)}
	req := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{
		"org_id":"org_1",
		"name":"from-snapshot",
		"snapshot_id":"snp12345",
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
	if got.SnapshotID != "snp12345" || got.SnapshotArtifactURL == "" || got.SnapshotArtifactDigest == "" {
		t.Fatalf("runner snapshot request = %+v", got)
	}
	if got.SnapshotImageDigest != "sha256:image" {
		t.Fatalf("runner image digest = %q", got.SnapshotImageDigest)
	}
	if got.ImageRef != "ghcr.io/usehivy/runtime:test" {
		t.Fatalf("runner image ref = %q", got.ImageRef)
	}
}
