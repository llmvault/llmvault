package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	ragmodel "github.com/usehivy/hivy/internal/rag/model"
)

// L9: GET /v1/rag/sources/{id}/channels filters channel grants to those the
// caller may view.
func TestGetSourceChannels_ActorScopedVisibility(t *testing.T) {
	db := connectTestDB(t)
	fx := seedVisFixture(t, db)
	h := handler.NewRAGSourceHandler(db, nil, nil, nil, nil, nil)

	src := ragmodel.RAGSource{
		OrgIDValue:  fx.org.ID,
		KindValue:   ragmodel.RAGSourceKindWebsite,
		Name:        "vis-src-" + uuid.NewString()[:8],
		Status:      ragmodel.RAGSourceStatusActive,
		Enabled:     true,
		ConfigValue: model.JSON{"url": "https://vis.example"},
	}
	if err := db.Create(&src).Error; err != nil {
		t.Fatalf("seed rag source: %v", err)
	}
	// Grants are team-derived now: granting the source to a channel's team makes
	// that channel a searcher of the source. teamA backs visibleCh (member sees
	// it), teamB backs hiddenCh (only admin sees it).
	grants := []any{
		&model.TeamRagSource{OrgID: fx.org.ID, TeamID: *fx.visibleCh.TeamID, RagSourceID: src.ID},
		&model.TeamRagSource{OrgID: fx.org.ID, TeamID: *fx.hiddenCh.TeamID, RagSourceID: src.ID},
	}
	for _, g := range grants {
		if err := db.Create(g).Error; err != nil {
			t.Fatalf("seed grant: %v", err)
		}
	}
	t.Cleanup(func() {
		db.Where("rag_source_id = ?", src.ID).Delete(&model.TeamRagSource{})
		db.Where("id = ?", src.ID).Delete(&ragmodel.RAGSource{})
	})

	channels := func(c caller) map[string]bool {
		req := httptest.NewRequest(http.MethodGet, "/v1/rag/sources/"+src.ID.String()+"/channels", nil)
		req = c.apply(req, fx.org)
		req = withURLParam(req, "id", src.ID.String())
		rr := httptest.NewRecorder()
		h.GetSourceChannels(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("get source channels status=%d body=%s", rr.Code, rr.Body.String())
		}
		var resp struct {
			ChannelIDs []string `json:"channel_ids"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		ids := map[string]bool{}
		for _, id := range resp.ChannelIDs {
			ids[id] = true
		}
		return ids
	}

	member := channels(memberCaller(fx))
	if !member[fx.visibleCh.ID.String()] {
		t.Fatalf("member missing visible channel grant: %v", member)
	}
	if member[fx.hiddenCh.ID.String()] {
		t.Fatalf("member must not see hidden channel grant: %v", member)
	}
	admin := channels(adminCaller(fx))
	if !admin[fx.visibleCh.ID.String()] || !admin[fx.hiddenCh.ID.String()] {
		t.Fatalf("admin must see both channel grants: %v", admin)
	}
}

// L14: the /v1/audit route is admin-only when mounted behind RequireOrgAdmin.
func TestAuditRoute_AdminOnly(t *testing.T) {
	db := connectTestDB(t)
	fx := seedVisFixture(t, db)
	auditHandler := handler.NewAuditHandler(db)
	seedAuditEntries(t, db, fx.org.ID, 2, "api.request")

	router := chi.NewRouter()
	router.Group(func(r chi.Router) {
		r.Use(middleware.RequireOrgAdmin(db))
		r.Get("/v1/audit", auditHandler.List)
	})

	status := func(c caller) int {
		req := httptest.NewRequest(http.MethodGet, "/v1/audit", nil)
		req = c.apply(req, fx.org)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		return rr.Code
	}

	if code := status(memberCaller(fx)); code != http.StatusForbidden {
		t.Fatalf("member audit = %d, want 403", code)
	}
	if code := status(adminCaller(fx)); code != http.StatusOK {
		t.Fatalf("admin audit = %d, want 200", code)
	}
}
