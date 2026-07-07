package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestCatalogInstalledAgentID_ActorScopedVisibility(t *testing.T) {
	db := connectTestDB(t)
	fx := seedVisFixture(t, db)
	h := newAgentHandlerForTest(db)

	catalog := model.AgentCatalog{
		ID:           uuid.New(),
		Slug:         "vis-catalog-" + uuid.NewString()[:8],
		Name:         "Vis Catalog " + uuid.NewString()[:8],
		Model:        "test",
		SandboxImage: model.SandboxImageDefault,
		Manifest:     model.RawJSON(`{}`),
		Status:       model.AgentCatalogStatusActive,
	}
	if err := db.Create(&catalog).Error; err != nil {
		t.Fatalf("create catalog: %v", err)
	}
	// The installed agent for this catalog is assigned only to the hidden
	// (teamB) channel, so the member must not learn its id.
	if err := db.Model(&model.Agent{}).Where("id = ?", fx.hiddenAgent.ID).
		Update("agent_catalog_id", catalog.ID).Error; err != nil {
		t.Fatalf("link agent to catalog: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", catalog.ID).Delete(&model.AgentCatalog{}) })

	installedID := func(c caller) *string {
		req := httptest.NewRequest(http.MethodGet, "/v1/agents/catalog/"+catalog.Slug, nil)
		req = c.apply(req, fx.org)
		req = withURLParam(req, "slug", catalog.Slug)
		rr := httptest.NewRecorder()
		h.GetCatalog(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("get catalog status=%d body=%s", rr.Code, rr.Body.String())
		}
		var resp struct {
			InstalledAgentID *string `json:"installed_agent_id"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return resp.InstalledAgentID
	}

	if got := installedID(memberCaller(fx)); got != nil {
		t.Fatalf("member installed_agent_id = %v, want nil (installed agent hidden)", *got)
	}
	if got := installedID(adminCaller(fx)); got == nil || *got != fx.hiddenAgent.ID.String() {
		t.Fatalf("admin installed_agent_id = %v, want %s", got, fx.hiddenAgent.ID)
	}
	if got := installedID(apiKeyCaller()); got == nil || *got != fx.hiddenAgent.ID.String() {
		t.Fatalf("api-key installed_agent_id = %v, want %s", got, fx.hiddenAgent.ID)
	}
}
