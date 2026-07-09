package handler_test

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/model"
)

// templateZipKeptFiles is a buildable-looking template tree that must ship
// to builders exactly as on disk.
var templateZipKeptFiles = []string{
	"README.md",
	"Makefile",
	"app.json",
	"go.mod",
	"go.sum",
	"main.go",
	"api/api.go",
	"hivycore/config.go",
	"scripts/preview.sh",
	"web/index.html",
	"web/package.json",
	"web/package-lock.json",
	"web/vite.config.ts",
	"web/src/App.tsx",
}

// templateZipStrippedFiles exist on disk but must never appear in the zip:
// build outputs, dependency caches, repo metadata, and the template's own
// test machinery (Go tests, Vitest tests + config, fixture dirs).
var templateZipStrippedFiles = []string{
	"node_modules/x/index.js",
	"dist/server",
	"public/index.html",
	".git/config",
	".gitignore",
	"web/.gitignore",
	"hivycore/auth_test.go",
	"hivycore/session_test.go",
	"api/api_test.go",
	"web/src/App.test.tsx",
	"web/src/lib/realtime.test.ts",
	"web/vitest.config.ts",
	"hivycore/testdata/fixture.json",
	"web/src/fixtures/rows.json",
	"api/__tests__/api.spec.ts",
}

// templateZipContent gives each kept file plausible content so the archive
// round-trip check has something real to compare.
func templateZipContent(name string) string {
	if name == "app.json" {
		return "{\"name\":\"template\"}"
	}
	return "content of " + name
}

// templateZipHarness wires the template-zip endpoint with a synthetic
// template dir and a real agent+sandbox for the runtime-secret auth.
type templateZipHarness struct {
	db      *gorm.DB
	router  *chi.Mux
	agent   model.Agent
	sandbox model.Sandbox
	secret  string
}

func newTemplateZipHarness(t *testing.T) *templateZipHarness {
	t.Helper()
	db := connectTestDB(t)
	org := createTestOrg(t, db)

	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		t.Fatalf("generate enc key: %v", err)
	}
	encKey, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(keyBytes))
	if err != nil {
		t.Fatalf("build enc key: %v", err)
	}

	h := &templateZipHarness{db: db}
	h.agent = model.Agent{ID: uuid.New(), OrgID: &org.ID, TeamID: firstTeamID(t, db, org.ID), Name: "Tmpl Agent " + uuid.NewString(), Model: "test", Status: "active"}
	if err := db.Create(&h.agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	h.secret = "runtime-secret-" + uuid.NewString()
	encrypted, err := encKey.EncryptString(h.secret)
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}
	h.sandbox = model.Sandbox{
		OrgID:                  &org.ID,
		AgentID:                &h.agent.ID,
		ProviderID:             "stub",
		ExternalID:             "ext-tmpl",
		RuntimeURL:             "http://127.0.0.1:1",
		EncryptedRuntimeSecret: encrypted,
		Status:                 "running",
	}
	if err := db.Create(&h.sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	t.Cleanup(func() {
		db.Delete(&model.Sandbox{}, "id = ?", h.sandbox.ID)
		db.Delete(&model.Agent{}, "id = ?", h.agent.ID)
	})

	// Synthetic template mirroring the real layout: a buildable tree that
	// must ship, plus build outputs, repo metadata, and the template's own
	// CI tests that must never reach a builder agent.
	dir := t.TempDir()
	files := map[string]string{}
	for _, name := range templateZipKeptFiles {
		files[name] = templateZipContent(name)
	}
	for _, name := range templateZipStrippedFiles {
		files[name] = "must-not-ship"
	}
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	uploadsHandler := handler.NewUploadsHandler(db, nil).
		WithStreamer(nil, encKey).
		WithAppsTemplateDir(dir)
	router := chi.NewRouter()
	router.Get("/internal/agents/{agentID}/sandboxes/{sandboxID}/apps-template.zip", uploadsHandler.StreamAppsTemplateZip)
	h.router = router
	return h
}

func (h *templateZipHarness) fetch(t *testing.T, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	path := "/internal/agents/" + h.agent.ID.String() + "/sandboxes/" + h.sandbox.ID.String() + "/apps-template.zip"
	req := httptest.NewRequest("GET", path, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func TestAppsTemplateZipAuth(t *testing.T) {
	h := newTemplateZipHarness(t)

	if rr := h.fetch(t, ""); rr.Code != 401 {
		t.Fatalf("missing bearer status = %d", rr.Code)
	}
	if rr := h.fetch(t, "wrong-secret"); rr.Code != 401 {
		t.Fatalf("wrong bearer status = %d", rr.Code)
	}
}

func TestAppsTemplateZipContent(t *testing.T) {
	h := newTemplateZipHarness(t)

	rr := h.fetch(t, h.secret)
	if rr.Code != 200 {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "application/zip" {
		t.Fatalf("content type = %q", got)
	}

	reader, err := zip.NewReader(bytes.NewReader(rr.Body.Bytes()), int64(rr.Body.Len()))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	names := map[string]bool{}
	for _, file := range reader.File {
		names[file.Name] = true
	}
	// The kept set ships in full — spot-checking the paths a build needs
	// (go.mod, main.go, Makefile, web/package.json, web/vite.config.ts, …).
	for _, want := range templateZipKeptFiles {
		if !names[want] {
			t.Fatalf("zip is missing %q (has %v)", want, names)
		}
	}
	// Files present on disk but excluded — tests, fixtures, build outputs,
	// repo metadata — never appear.
	for _, excluded := range templateZipStrippedFiles {
		if names[excluded] {
			t.Fatalf("zip must not contain %q", excluded)
		}
	}
	if len(names) != len(templateZipKeptFiles) {
		t.Fatalf("zip has %d entries, want exactly the %d kept files: %v", len(names), len(templateZipKeptFiles), names)
	}

	// Content round-trips.
	for _, file := range reader.File {
		if file.Name != "app.json" {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open entry: %v", err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read entry: %v", err)
		}
		if string(data) != "{\"name\":\"template\"}" {
			t.Fatalf("app.json content = %q", data)
		}
	}
}
