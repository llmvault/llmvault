package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/canvas"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type canvasProjectHarness struct {
	db     *gorm.DB
	router *chi.Mux
}

func newCanvasProjectHarness(t *testing.T) *canvasProjectHarness {
	t.Helper()
	db := connectTestDB(t)
	svc := canvas.NewService(db, &canvas.Client{
		PublicURL:       "https://canvas.test",
		APIBaseURL:      "https://canvas-api.test",
		ControlPlaneKey: "test-key",
	})
	h := handler.NewCanvasHandler(db, nil, svc)
	r := chi.NewRouter()
	r.Get("/v1/canvas/projects", h.ListProjects)
	return &canvasProjectHarness{db: db, router: r}
}

func TestIntegration_CanvasProjectsListIncludesFiles(t *testing.T) {
	h := newCanvasProjectHarness(t)
	org := createTestOrg(t, h.db)
	otherOrg := createTestOrg(t, h.db)
	now := time.Now().UTC()

	project := seedCanvasProject(t, h.db, org.ID, "Launch redesign", now)
	emptyProject := seedCanvasProject(t, h.db, org.ID, "Empty project", now.Add(time.Minute))
	pageID := uuid.New()
	file := seedCanvasFile(t, h.db, org.ID, project, "Homepage", &pageID, now.Add(2*time.Minute))
	otherProject := seedCanvasProject(t, h.db, otherOrg.ID, "Other org", now.Add(3*time.Minute))
	seedCanvasFile(t, h.db, otherOrg.ID, otherProject, "Private file", nil, now.Add(4*time.Minute))

	rr := h.doCanvasProjects(t, org)
	if rr.Code != http.StatusOK {
		t.Fatalf("list canvas projects status=%d body=%s", rr.Code, rr.Body.String())
	}

	type canvasProjectFileResponse struct {
		FileID       string  `json:"file_id"`
		ProjectID    string  `json:"project_id"`
		PageID       *string `json:"page_id"`
		Name         string  `json:"name"`
		WorkspaceURL string  `json:"workspace_url"`
	}
	var resp struct {
		Projects []struct {
			ProjectID string                      `json:"project_id"`
			Name      string                      `json:"name"`
			Files     []canvasProjectFileResponse `json:"files"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode canvas projects: %v\n%s", err, rr.Body.String())
	}
	if len(resp.Projects) != 2 {
		t.Fatalf("project count=%d want 2: %+v", len(resp.Projects), resp.Projects)
	}

	projects := map[string]struct {
		Name  string
		Files int
	}{}
	var listedFile canvasProjectFileResponse
	for _, item := range resp.Projects {
		projects[item.ProjectID] = struct {
			Name  string
			Files int
		}{Name: item.Name, Files: len(item.Files)}
		if item.ProjectID == project.PenpotProjectID.String() && len(item.Files) == 1 {
			listedFile = item.Files[0]
		}
	}
	if projects[project.PenpotProjectID.String()].Name != "Launch redesign" || projects[project.PenpotProjectID.String()].Files != 1 {
		t.Fatalf("created project missing files: %+v", projects)
	}
	if projects[emptyProject.PenpotProjectID.String()].Name != "Empty project" || projects[emptyProject.PenpotProjectID.String()].Files != 0 {
		t.Fatalf("empty project not listed correctly: %+v", projects)
	}
	if listedFile.FileID != file.PenpotFileID.String() || listedFile.ProjectID != project.PenpotProjectID.String() || listedFile.Name != "Homepage" {
		t.Fatalf("unexpected file response: %+v", listedFile)
	}
	if listedFile.PageID == nil || *listedFile.PageID != pageID.String() {
		t.Fatalf("page id = %v, want %s", listedFile.PageID, pageID)
	}
	if !strings.Contains(listedFile.WorkspaceURL, "https://canvas.test/#/workspace?") ||
		!strings.Contains(listedFile.WorkspaceURL, "file-id="+file.PenpotFileID.String()) ||
		!strings.Contains(listedFile.WorkspaceURL, "page-id="+pageID.String()) {
		t.Fatalf("workspace url missing target ids: %s", listedFile.WorkspaceURL)
	}
}

func (h *canvasProjectHarness) doCanvasProjects(t *testing.T, org model.Org) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/canvas/projects", nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func seedCanvasProject(t *testing.T, db *gorm.DB, orgID uuid.UUID, name string, updatedAt time.Time) model.CanvasProject {
	t.Helper()
	project := model.CanvasProject{
		ID:              uuid.New(),
		OrgID:           orgID,
		PenpotProjectID: uuid.New(),
		Name:            name,
		CreatedAt:       updatedAt.Add(-time.Minute),
		UpdatedAt:       updatedAt,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("seed canvas project: %v", err)
	}
	return project
}

func seedCanvasFile(t *testing.T, db *gorm.DB, orgID uuid.UUID, project model.CanvasProject, name string, pageID *uuid.UUID, updatedAt time.Time) model.CanvasFile {
	t.Helper()
	file := model.CanvasFile{
		ID:              uuid.New(),
		OrgID:           orgID,
		CanvasProjectID: &project.ID,
		PenpotProjectID: project.PenpotProjectID,
		PenpotFileID:    uuid.New(),
		PenpotPageID:    pageID,
		Name:            name,
		CreatedAt:       updatedAt.Add(-time.Minute),
		UpdatedAt:       updatedAt,
	}
	if err := db.Create(&file).Error; err != nil {
		t.Fatalf("seed canvas file: %v", err)
	}
	return file
}
