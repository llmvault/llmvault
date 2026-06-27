package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/canvasartifact"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/storage"
)

type canvasArtifactHarness struct {
	db            *gorm.DB
	router        *chi.Mux
	store         *fakeCanvasArtifactStore
	org           model.Org
	agentID       uuid.UUID
	sessionID     uuid.UUID
	runtimeSecret string
}

type fakeCanvasArtifactStore struct {
	objects map[string][]byte
}

func (s *fakeCanvasArtifactStore) Stream(ctx context.Context, key, contentType string, body io.Reader) (*storage.StoredAsset, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	s.objects[key] = data
	return &storage.StoredAsset{Key: key, Bytes: int64(len(data))}, nil
}

func (s *fakeCanvasArtifactStore) Delete(ctx context.Context, key string) error {
	delete(s.objects, key)
	return nil
}

func (s *fakeCanvasArtifactStore) PresignGet(ctx context.Context, key string) (string, error) {
	return "https://storage.test/" + key, nil
}

func newCanvasArtifactHarness(t *testing.T) *canvasArtifactHarness {
	t.Helper()
	db := connectTestDB(t)
	encKey := testSymmetricKey(t)
	org := createTestOrg(t, db)
	agent := seedSessionAgent(t, db, org.ID)
	channel := model.Channel{ID: uuid.New(), OrgID: org.ID, Name: "Canvas", Kind: "standard", DefaultAgentID: agent.ID}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	runtimeSecret := "runtime-canvas-secret" // #nosec G101 -- test runtime bearer secret.
	encrypted, err := encKey.EncryptString(runtimeSecret)
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}
	sandbox := model.Sandbox{
		ID:                     uuid.New(),
		OrgID:                  &org.ID,
		AgentID:                &agent.ID,
		Status:                 "running",
		RuntimeURL:             "http://runtime.test",
		EncryptedRuntimeSecret: encrypted,
	}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	session := model.Session{
		ID:        uuid.New(),
		OrgID:     org.ID,
		ChannelID: channel.ID,
		AgentID:   agent.ID,
		SandboxID: &sandbox.ID,
		Status:    "active",
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}

	store := &fakeCanvasArtifactStore{objects: map[string][]byte{}}
	canvasHandler := handler.NewCanvasHandler(db, encKey, nil).
		WithArtifactService(canvasartifact.NewService(db, store))
	router := chi.NewRouter()
	router.Get("/internal/agents/{agentID}/canvas/projects", canvasHandler.ListAgentProjects)
	router.Post("/internal/agents/{agentID}/canvas/projects", canvasHandler.CreateAgentProject)
	router.Get("/internal/agents/{agentID}/canvas/artifacts", canvasHandler.ListAgentArtifacts)
	router.Get("/internal/agents/{agentID}/canvas/snapshot", canvasHandler.SnapshotAgentCanvas)
	router.Post("/internal/agents/{agentID}/canvas/artifacts/sync", canvasHandler.SyncAgentArtifact)
	router.Get("/v1/canvas/projects", canvasHandler.ListProjects)
	router.Get("/v1/canvas/artifacts", canvasHandler.ListArtifacts)
	router.Get("/v1/canvas/artifacts/{artifactID}", canvasHandler.GetArtifact)
	router.Post("/v1/canvas/artifacts/{artifactID}/preview-url", canvasHandler.PreviewArtifactURL)
	return &canvasArtifactHarness{
		db:            db,
		router:        router,
		store:         store,
		org:           org,
		agentID:       agent.ID,
		sessionID:     session.ID,
		runtimeSecret: runtimeSecret,
	}
}

func TestIntegration_CanvasArtifactsRuntimeSyncAndPublicPreview(t *testing.T) {
	h := newCanvasArtifactHarness(t)

	create := h.doRuntimeJSON(t, http.MethodPost, "/internal/agents/"+h.agentID.String()+"/canvas/projects", map[string]any{
		"name": "Marketing pages",
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("create project status=%d body=%s", create.Code, create.Body.String())
	}
	var createdProject struct {
		Slug string `json:"slug"`
		ID   string `json:"id"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &createdProject); err != nil {
		t.Fatalf("decode create project: %v", err)
	}
	if createdProject.Slug != "marketing-pages" {
		t.Fatalf("project slug=%q", createdProject.Slug)
	}

	sync := h.doRuntimeJSON(t, http.MethodPost, "/internal/agents/"+h.agentID.String()+"/canvas/artifacts/sync", map[string]any{
		"session_id": h.sessionID.String(),
		"project": map[string]any{
			"id":   createdProject.ID,
			"slug": createdProject.Slug,
			"name": "Marketing pages",
		},
		"artifact": map[string]any{
			"slug": "landing-page",
			"type": "web_page",
			"name": "Landing page",
			"manifest": map[string]any{
				"schema_version": 1,
				"kind":           "hivy.canvas.artifact",
				"type":           "web_page",
				"files": []any{
					map[string]any{"path": "index.html", "role": "entrypoint"},
				},
			},
		},
		"files": []any{
			map[string]any{
				"path":         "index.html",
				"role":         "entrypoint",
				"content_type": "text/html; charset=utf-8",
				"content":      "<!doctype html><html><body data-canvas-id=\"hero\">Hello</body></html>",
			},
		},
	})
	if sync.Code != http.StatusOK {
		t.Fatalf("sync status=%d body=%s", sync.Code, sync.Body.String())
	}
	if len(h.store.objects) != 1 {
		t.Fatalf("stored objects=%d want 1", len(h.store.objects))
	}
	var synced struct {
		Artifact struct {
			ID    string `json:"id"`
			Files []struct {
				ObjectKey string `json:"object_key"`
			} `json:"files"`
		} `json:"artifact"`
	}
	if err := json.Unmarshal(sync.Body.Bytes(), &synced); err != nil {
		t.Fatalf("decode sync: %v", err)
	}
	if synced.Artifact.ID == "" || len(synced.Artifact.Files) != 1 {
		t.Fatalf("unexpected sync response: %s", sync.Body.String())
	}

	snapshot := h.doRuntimeJSON(t, http.MethodGet, "/internal/agents/"+h.agentID.String()+"/canvas/snapshot", nil)
	if snapshot.Code != http.StatusOK {
		t.Fatalf("snapshot status=%d body=%s", snapshot.Code, snapshot.Body.String())
	}
	if !strings.Contains(snapshot.Body.String(), "https://storage.test/pub/o/") {
		t.Fatalf("snapshot missing presigned download URL: %s", snapshot.Body.String())
	}

	publicList := h.doOrgJSON(t, http.MethodGet, "/v1/canvas/artifacts?project_id="+createdProject.ID+"&session_id="+h.sessionID.String(), nil)
	if publicList.Code != http.StatusOK {
		t.Fatalf("public artifact list status=%d body=%s", publicList.Code, publicList.Body.String())
	}
	if !strings.Contains(publicList.Body.String(), "landing-page") {
		t.Fatalf("public list missing artifact: %s", publicList.Body.String())
	}

	preview := h.doOrgJSON(t, http.MethodPost, "/v1/canvas/artifacts/"+synced.Artifact.ID+"/preview-url", map[string]any{
		"session_id": h.sessionID.String(),
	})
	if preview.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", preview.Code, preview.Body.String())
	}
	if !strings.Contains(preview.Body.String(), "http://runtime.test/canvas/preview/projects/marketing-pages/artifacts/landing-page/index.html") {
		t.Fatalf("preview URL mismatch: %s", preview.Body.String())
	}
}

func (h *canvasArtifactHarness) doRuntimeJSON(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+h.runtimeSecret)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func (h *canvasArtifactHarness) doOrgJSON(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req = middleware.WithOrg(req, &h.org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}
