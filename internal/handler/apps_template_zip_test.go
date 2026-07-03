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
	h.agent = model.Agent{ID: uuid.New(), OrgID: &org.ID, Name: "Tmpl Agent " + uuid.NewString(), Model: "test", Status: "active"}
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

	// Synthetic template with excluded directories that must never ship.
	dir := t.TempDir()
	files := map[string]string{
		"app.json":                "{\"name\":\"template\"}",
		"hivycore/config.go":      "package hivycore",
		"web/src/App.tsx":         "export default function App() {}",
		"node_modules/x/index.js": "module.exports = 1",
		"dist/server":             "binary",
		"public/index.html":       "<html></html>",
		".git/config":             "[core]",
	}
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
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
	for _, want := range []string{"app.json", "hivycore/config.go", "web/src/App.tsx"} {
		if !names[want] {
			t.Fatalf("zip is missing %q (has %v)", want, names)
		}
	}
	for _, excluded := range []string{"node_modules/x/index.js", "dist/server", "public/index.html", ".git/config"} {
		if names[excluded] {
			t.Fatalf("zip must not contain %q", excluded)
		}
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
