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

func decodeInto(t *testing.T, rr *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rr.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode response: %v\n%s", err, rr.Body.String())
	}
}

func newChannelRAGHarness(t *testing.T) *channelHarness {
	t.Helper()
	h := newChannelHarness(t)
	// Re-mount with the rag-source routes included.
	db := h.db
	ch := handler.NewChannelHandler(db)
	r := chi.NewRouter()
	r.Route("/v1", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(db))
		r.Use(middleware.RequireAPIKeyScopeOrJWT("channels"))
		r.Post("/channels", ch.Create)
		r.Get("/channels/{id}/rag-sources", ch.ListChannelRAGSources)
		r.Put("/channels/{id}/rag-sources", ch.SetChannelRAGSources)
	})
	h.router = r
	return h
}

func seedWebsiteSource(t *testing.T, h *channelHarness, orgID uuid.UUID, name string) ragmodel.RAGSource {
	t.Helper()
	src := ragmodel.RAGSource{
		OrgIDValue:  orgID,
		KindValue:   ragmodel.RAGSourceKindWebsite,
		Name:        name,
		Status:      ragmodel.RAGSourceStatusActive,
		Enabled:     true,
		AccessType:  ragmodel.AccessTypePublic,
		ConfigValue: model.JSON{"url": "https://" + name + ".example"},
	}
	if err := h.db.Create(&src).Error; err != nil {
		t.Fatalf("seed rag source: %v", err)
	}
	t.Cleanup(func() { h.db.Where("id = ?", src.ID).Delete(&ragmodel.RAGSource{}) })
	return src
}

func createTestChannel(t *testing.T, h *channelHarness, fx channelFixture) string {
	t.Helper()
	rr := h.doJSON(t, http.MethodPost, "/v1/channels", fx, fx.owner, map[string]any{
		"name":             "#eng",
		"default_agent_id": fx.agent.ID.String(),
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create channel: got %d body=%s", rr.Code, rr.Body.String())
	}
	return decodeChannelCreate(t, rr).Channel.ID
}

func TestChannelRAGSources_DenyByDefaultThenGrant(t *testing.T) {
	h := newChannelRAGHarness(t)
	seeder := &channelHarness{db: h.db}
	fx := seeder.seed(t)
	channelID := createTestChannel(t, h, fx)

	// Fresh channel: no knowledge sources (deny-by-default).
	rr := h.doJSON(t, http.MethodGet, "/v1/channels/"+channelID+"/rag-sources", fx, fx.owner, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("list empty: got %d body=%s", rr.Code, rr.Body.String())
	}
	var listed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	decodeInto(t, rr, &listed)
	if len(listed.Data) != 0 {
		t.Fatalf("fresh channel sources = %d, want 0", len(listed.Data))
	}

	srcA := seedWebsiteSource(t, h, fx.org.ID, "acme-docs")
	srcB := seedWebsiteSource(t, h, fx.org.ID, "acme-blog")

	// Grant both.
	rr = h.doJSON(t, http.MethodPut, "/v1/channels/"+channelID+"/rag-sources", fx, fx.owner, map[string]any{
		"source_ids": []string{srcA.ID.String(), srcB.ID.String()},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("put grant: got %d body=%s", rr.Code, rr.Body.String())
	}
	decodeInto(t, rr, &listed)
	if len(listed.Data) != 2 {
		t.Fatalf("granted sources = %d, want 2", len(listed.Data))
	}

	// Verify persisted via GET.
	rr = h.doJSON(t, http.MethodGet, "/v1/channels/"+channelID+"/rag-sources", fx, fx.owner, nil)
	decodeInto(t, rr, &listed)
	if len(listed.Data) != 2 {
		t.Fatalf("listed after grant = %d, want 2", len(listed.Data))
	}

	// PUT replaces the full set (down to one).
	rr = h.doJSON(t, http.MethodPut, "/v1/channels/"+channelID+"/rag-sources", fx, fx.owner, map[string]any{
		"source_ids": []string{srcA.ID.String()},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("put replace: got %d body=%s", rr.Code, rr.Body.String())
	}
	decodeInto(t, rr, &listed)
	if len(listed.Data) != 1 || listed.Data[0].ID != srcA.ID.String() {
		t.Fatalf("after replace = %+v, want only %s", listed.Data, srcA.ID)
	}

	// Empty set clears all access.
	rr = h.doJSON(t, http.MethodPut, "/v1/channels/"+channelID+"/rag-sources", fx, fx.owner, map[string]any{
		"source_ids": []string{},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("put clear: got %d body=%s", rr.Code, rr.Body.String())
	}
	decodeInto(t, rr, &listed)
	if len(listed.Data) != 0 {
		t.Fatalf("after clear = %d, want 0", len(listed.Data))
	}
}

func TestChannelRAGSources_RejectsCrossOrgSource(t *testing.T) {
	h := newChannelRAGHarness(t)
	seeder := &channelHarness{db: h.db}
	fx := seeder.seed(t)
	channelID := createTestChannel(t, h, fx)

	// A source belonging to a different org must be rejected.
	otherOrg := model.Org{Name: "other-org-" + uuid.NewString()[:8], Active: true}
	if err := h.db.Create(&otherOrg).Error; err != nil {
		t.Fatalf("create other org: %v", err)
	}
	t.Cleanup(func() { h.db.Where("id = ?", otherOrg.ID).Delete(&model.Org{}) })
	foreign := seedWebsiteSource(t, h, otherOrg.ID, "foreign-docs")

	rr := h.doJSON(t, http.MethodPut, "/v1/channels/"+channelID+"/rag-sources", fx, fx.owner, map[string]any{
		"source_ids": []string{foreign.ID.String()},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("cross-org grant: got %d body=%s, want 400", rr.Code, rr.Body.String())
	}
}
