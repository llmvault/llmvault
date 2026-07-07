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
)

func newVisCanvasHandler(db *gorm.DB) *handler.CanvasHandler {
	return handler.NewCanvasHandler(db, nil).WithArtifactService(canvasartifact.NewService(db, nil))
}

func TestCanvasArtifacts_ActorScopedVisibility(t *testing.T) {
	db := connectTestDB(t)
	fx := seedVisFixture(t, db)
	h := newVisCanvasHandler(db)
	now := time.Now().UTC()

	project := seedCanvasArtifactProject(t, db, fx.org.ID, "proj-"+uuid.NewString()[:8], "Proj", now)
	visArt := seedCanvasArtifact(t, db, fx.org.ID, project.ID, "vis-art", now.Add(time.Minute), &fx.visibleSess)
	hidArt := seedCanvasArtifact(t, db, fx.org.ID, project.ID, "hid-art", now.Add(2*time.Minute), &fx.hiddenSess)

	listIDs := func(c caller) map[string]bool {
		req := httptest.NewRequest(http.MethodGet, "/v1/canvas/artifacts", nil)
		req = c.apply(req, fx.org)
		rr := httptest.NewRecorder()
		h.ListArtifacts(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("list artifacts status=%d body=%s", rr.Code, rr.Body.String())
		}
		var resp struct {
			Artifacts []struct {
				ID string `json:"id"`
			} `json:"artifacts"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		ids := map[string]bool{}
		for _, a := range resp.Artifacts {
			ids[a.ID] = true
		}
		return ids
	}

	member := listIDs(memberCaller(fx))
	if !member[visArt.ID.String()] {
		t.Fatalf("member missing visible artifact: %v", member)
	}
	if member[hidArt.ID.String()] {
		t.Fatalf("member must not see hidden-session artifact: %v", member)
	}
	admin := listIDs(adminCaller(fx))
	if !admin[visArt.ID.String()] || !admin[hidArt.ID.String()] {
		t.Fatalf("admin must see both artifacts: %v", admin)
	}

	get := func(c caller, id uuid.UUID) int {
		req := httptest.NewRequest(http.MethodGet, "/v1/canvas/artifacts/"+id.String(), nil)
		req = c.apply(req, fx.org)
		req = withURLParam(req, "artifactID", id.String())
		rr := httptest.NewRecorder()
		h.GetArtifact(rr, req)
		return rr.Code
	}
	if code := get(memberCaller(fx), visArt.ID); code != http.StatusOK {
		t.Fatalf("member get visible artifact = %d, want 200", code)
	}
	if code := get(memberCaller(fx), hidArt.ID); code != http.StatusNotFound {
		t.Fatalf("member get hidden artifact = %d, want 404", code)
	}
	if code := get(adminCaller(fx), hidArt.ID); code != http.StatusOK {
		t.Fatalf("admin get hidden artifact = %d, want 200", code)
	}
}

func TestCanvasProjects_ActorScopedVisibility(t *testing.T) {
	db := connectTestDB(t)
	fx := seedVisFixture(t, db)
	h := newVisCanvasHandler(db)
	router := chi.NewRouter()
	router.Get("/v1/canvas/projects", h.ListProjects)
	now := time.Now().UTC()

	visProj := seedCanvasArtifactProject(t, db, fx.org.ID, "vproj-"+uuid.NewString()[:8], "Visible proj", now)
	hidProj := seedCanvasArtifactProject(t, db, fx.org.ID, "hproj-"+uuid.NewString()[:8], "Hidden proj", now.Add(time.Minute))
	seedCanvasArtifact(t, db, fx.org.ID, visProj.ID, "vis-art", now.Add(2*time.Minute), &fx.visibleSess)
	seedCanvasArtifact(t, db, fx.org.ID, hidProj.ID, "hid-art", now.Add(3*time.Minute), &fx.hiddenSess)

	listIDs := func(c caller) map[string]bool {
		req := httptest.NewRequest(http.MethodGet, "/v1/canvas/projects", nil)
		req = c.apply(req, fx.org)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("list projects status=%d body=%s", rr.Code, rr.Body.String())
		}
		var resp struct {
			Projects []struct {
				ID string `json:"id"`
			} `json:"projects"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		ids := map[string]bool{}
		for _, p := range resp.Projects {
			ids[p.ID] = true
		}
		return ids
	}

	member := listIDs(memberCaller(fx))
	if !member[visProj.ID.String()] {
		t.Fatalf("member missing visible project: %v", member)
	}
	if member[hidProj.ID.String()] {
		t.Fatalf("member must not see project with only hidden artifacts: %v", member)
	}
	admin := listIDs(adminCaller(fx))
	if !admin[visProj.ID.String()] || !admin[hidProj.ID.String()] {
		t.Fatalf("admin must see both projects: %v", admin)
	}
}
