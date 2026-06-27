package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/canvasartifact"
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
	canvasHandler := handler.NewCanvasHandler(db, nil).
		WithArtifactService(canvasartifact.NewService(db, nil))
	router := chi.NewRouter()
	router.Get("/v1/canvas/projects", canvasHandler.ListProjects)
	return &canvasProjectHarness{db: db, router: router}
}

func TestIntegration_CanvasProjectsListUsesArtifacts(t *testing.T) {
	h := newCanvasProjectHarness(t)
	org := createTestOrg(t, h.db)
	otherOrg := createTestOrg(t, h.db)
	now := time.Now().UTC()

	project := seedCanvasArtifactProject(t, h.db, org.ID, "launch-redesign", "Launch redesign", now)
	emptyProject := seedCanvasArtifactProject(t, h.db, org.ID, "empty-project", "Empty project", now.Add(time.Minute))
	otherProject := seedCanvasArtifactProject(t, h.db, otherOrg.ID, "other-org", "Other org", now.Add(2*time.Minute))
	seedCanvasArtifact(t, h.db, org.ID, project.ID, "landing-page", now.Add(3*time.Minute))
	seedCanvasArtifact(t, h.db, otherOrg.ID, otherProject.ID, "private-page", now.Add(4*time.Minute))

	rr := h.doCanvasProjects(t, org, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("list canvas projects status=%d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Projects []struct {
			ID            string `json:"id"`
			ProjectID     string `json:"project_id"`
			Slug          string `json:"slug"`
			Name          string `json:"name"`
			ArtifactCount int64  `json:"artifact_count"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode canvas projects: %v\n%s", err, rr.Body.String())
	}
	if len(resp.Projects) != 2 {
		t.Fatalf("project count=%d want 2: %+v", len(resp.Projects), resp.Projects)
	}

	projects := map[string]struct {
		Name          string
		ProjectID     string
		ArtifactCount int64
	}{}
	for _, item := range resp.Projects {
		projects[item.ID] = struct {
			Name          string
			ProjectID     string
			ArtifactCount int64
		}{Name: item.Name, ProjectID: item.ProjectID, ArtifactCount: item.ArtifactCount}
	}
	if projects[project.ID.String()].Name != "Launch redesign" ||
		projects[project.ID.String()].ProjectID != project.ID.String() ||
		projects[project.ID.String()].ArtifactCount != 1 {
		t.Fatalf("project missing artifact count: %+v", projects)
	}
	if projects[emptyProject.ID.String()].Name != "Empty project" ||
		projects[emptyProject.ID.String()].ArtifactCount != 0 {
		t.Fatalf("empty project not listed correctly: %+v", projects)
	}
	if _, ok := projects[otherProject.ID.String()]; ok {
		t.Fatalf("listed project from another org: %+v", projects)
	}
}

func (h *canvasProjectHarness) doCanvasProjects(t *testing.T, org model.Org, query string) *httptest.ResponseRecorder {
	t.Helper()
	path := "/v1/canvas/projects"
	if query != "" {
		path += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func seedCanvasArtifactProject(t *testing.T, db *gorm.DB, orgID uuid.UUID, slug, name string, updatedAt time.Time) model.CanvasProject {
	t.Helper()
	project := model.CanvasProject{
		ID:        uuid.New(),
		OrgID:     orgID,
		Slug:      slug,
		Name:      name,
		CreatedAt: updatedAt.Add(-time.Minute),
		UpdatedAt: updatedAt,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("seed canvas project: %v", err)
	}
	return project
}

func seedCanvasArtifact(t *testing.T, db *gorm.DB, orgID, projectID uuid.UUID, slug string, updatedAt time.Time) model.CanvasArtifact {
	t.Helper()
	artifact := model.CanvasArtifact{
		ID:              uuid.New(),
		OrgID:           orgID,
		CanvasProjectID: projectID,
		Slug:            slug,
		Type:            "web_page",
		Name:            "Landing page",
		ManifestJSON:    model.RawJSON(`{"schema_version":1}`),
		CreatedAt:       updatedAt.Add(-time.Minute),
		UpdatedAt:       updatedAt,
	}
	if err := db.Create(&artifact).Error; err != nil {
		t.Fatalf("seed canvas artifact: %v", err)
	}
	return artifact
}
