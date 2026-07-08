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

// newChannelRAGHarness mounts ONLY the read route. The channel-side grant path
// (PUT /channels/{id}/rag-sources) has been removed — knowledge grants are now
// team-level admin-only (team_rag_sources); see team_provisioning.go.
func newChannelRAGHarness(t *testing.T) *channelHarness {
	t.Helper()
	h := newChannelHarness(t)
	db := h.db
	ch := handler.NewChannelHandler(db)
	r := chi.NewRouter()
	r.Route("/v1", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(db))
		r.Use(middleware.RequireAPIKeyScopeOrJWT("channels"))
		r.Post("/channels", ch.Create)
		r.Get("/channels/{id}/rag-sources", ch.ListChannelRAGSources)
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
		ConfigValue: model.JSON{"url": "https://" + name + ".example"},
	}
	if err := h.db.Create(&src).Error; err != nil {
		t.Fatalf("seed rag source: %v", err)
	}
	t.Cleanup(func() { h.db.Where("id = ?", src.ID).Delete(&ragmodel.RAGSource{}) })
	return src
}

type channelRAGListOut struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// TestChannelRAGSources_TeamDerived: a channel's listed knowledge is exactly its
// team's grants (team_rag_sources), and a channel with no team has none.
func TestChannelRAGSources_TeamDerived(t *testing.T) {
	h := newChannelRAGHarness(t)
	seeder := &channelHarness{db: h.db}
	fx := seeder.seed(t)

	team := seedChannelTeam(t, h, fx, "kb-team-"+uuid.NewString()[:8])
	channelID := createTeamChannelForTest(t, h, fx, "#eng", team.ID)

	// Before any team grant: deny-by-default.
	rr := h.doJSON(t, http.MethodGet, "/v1/channels/"+channelID+"/rag-sources", fx, fx.owner, nil)
	var listed channelRAGListOut
	decodeInto(t, rr, &listed)
	if rr.Code != http.StatusOK || len(listed.Data) != 0 {
		t.Fatalf("fresh team channel: code=%d sources=%d, want 200/0", rr.Code, len(listed.Data))
	}

	// Grant a source to the TEAM (the new admin-only mechanism).
	srcA := seedWebsiteSource(t, h, fx.org.ID, "acme-docs")
	if err := h.db.Create(&model.TeamRagSource{OrgID: fx.org.ID, TeamID: team.ID, RagSourceID: srcA.ID}).Error; err != nil {
		t.Fatalf("grant source to team: %v", err)
	}

	rr = h.doJSON(t, http.MethodGet, "/v1/channels/"+channelID+"/rag-sources", fx, fx.owner, nil)
	decodeInto(t, rr, &listed)
	if len(listed.Data) != 1 || listed.Data[0].ID != srcA.ID.String() {
		t.Fatalf("team-derived list = %+v, want only %s", listed.Data, srcA.ID)
	}
}

// TestChannelRAGSources_OldChannelGrantNoLongerGrants proves the escalation
// fix: a legacy channel-scoped grant row (channel_rag_sources) confers NO
// knowledge access, because the read now derives solely from team grants.
func TestChannelRAGSources_OldChannelGrantNoLongerGrants(t *testing.T) {
	h := newChannelRAGHarness(t)
	seeder := &channelHarness{db: h.db}
	fx := seeder.seed(t)

	team := seedChannelTeam(t, h, fx, "legacy-team-"+uuid.NewString()[:8])
	channelID := createTeamChannelForTest(t, h, fx, "#legacy", team.ID)
	chUUID := uuid.MustParse(channelID)

	src := seedWebsiteSource(t, h, fx.org.ID, "legacy-docs")

	// Simulate the OLD escalation path: a direct channel-scoped grant.
	if err := h.db.Create(&model.ChannelRagSource{OrgID: fx.org.ID, ChannelID: chUUID, RagSourceID: src.ID}).Error; err != nil {
		t.Fatalf("seed legacy channel grant: %v", err)
	}

	// The channel-scoped grant must NOT surface — no team grant exists.
	rr := h.doJSON(t, http.MethodGet, "/v1/channels/"+channelID+"/rag-sources", fx, fx.owner, nil)
	var listed channelRAGListOut
	decodeInto(t, rr, &listed)
	if rr.Code != http.StatusOK || len(listed.Data) != 0 {
		t.Fatalf("legacy channel grant leaked access: code=%d sources=%d, want 200/0", rr.Code, len(listed.Data))
	}
}
