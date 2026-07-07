package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// memoryHTTPHarness mounts the memory list routes behind the same auth stack the
// real router uses, so the channel-visibility filtering can be exercised over
// HTTP with member / owner / API-key actors.
type memoryHTTPHarness struct {
	db     *gorm.DB
	router *chi.Mux
}

func newMemoryHTTPHarness(t *testing.T) *memoryHTTPHarness {
	t.Helper()
	db := connectTestDB(t)
	memoryHandler := handler.NewMemoryHandler(db, nil, nil, nil)
	r := chi.NewRouter()
	r.Route("/v1", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(db))
		r.Use(middleware.RequireAPIKeyScopeOrJWT("memories"))
		r.Get("/memories", memoryHandler.List)
		r.Get("/memories/grouped", memoryHandler.Grouped)
		r.Get("/memories/channels/{channelId}", memoryHandler.ListChannel)
	})
	return &memoryHTTPHarness{db: db, router: r}
}

func (h *memoryHTTPHarness) getJWT(t *testing.T, path string, fx channelFixture, user model.User) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, &bytes.Buffer{})
	req.Header.Set("X-Org-ID", fx.org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: user.ID.String(),
		OrgID:  fx.org.ID.String(),
	})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

// visibilityChannels is the set of channels used across the memory-control
// visibility tests: one usable by the member (their team), one not usable
// (another team), plus a global (channel-less) row for baseline.
type visibilityChannels struct {
	usable   model.Channel // member's team
	unusable model.Channel // other team
}

// seedTeamVisibility gives fx.member membership of teamA, then builds a channel
// scoped to teamA (usable) and one scoped to teamB (unusable to the member).
func seedTeamVisibility(t *testing.T, db *gorm.DB, fx channelFixture) visibilityChannels {
	t.Helper()
	teamA := model.Team{OrgID: fx.org.ID, Name: "vis-team-a-" + uuid.NewString()[:8]}
	teamB := model.Team{OrgID: fx.org.ID, Name: "vis-team-b-" + uuid.NewString()[:8]}
	if err := db.Create(&teamA).Error; err != nil {
		t.Fatalf("create teamA: %v", err)
	}
	if err := db.Create(&teamB).Error; err != nil {
		t.Fatalf("create teamB: %v", err)
	}
	if err := db.Create(&model.TeamMember{OrgID: fx.org.ID, TeamID: teamA.ID, UserID: fx.member.ID, Role: "member"}).Error; err != nil {
		t.Fatalf("add member to teamA: %v", err)
	}
	usable := model.Channel{OrgID: fx.org.ID, Name: "vis-usable-" + uuid.NewString()[:8], DefaultAgentID: fx.agent.ID, TeamID: &teamA.ID}
	unusable := model.Channel{OrgID: fx.org.ID, Name: "vis-unusable-" + uuid.NewString()[:8], DefaultAgentID: fx.agent.ID, TeamID: &teamB.ID}
	if err := db.Create(&usable).Error; err != nil {
		t.Fatalf("create usable channel: %v", err)
	}
	if err := db.Create(&unusable).Error; err != nil {
		t.Fatalf("create unusable channel: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", fx.org.ID).Delete(&model.TeamMember{})
	})
	return visibilityChannels{usable: usable, unusable: unusable}
}

func seedMemoryRow(t *testing.T, db *gorm.DB, orgID uuid.UUID, channelID *uuid.UUID, content string) uuid.UUID {
	t.Helper()
	mem := model.AgentMemory{
		OrgID:           orgID,
		ChannelID:       channelID,
		Content:         content,
		EmbeddingModel:  "test",
		EmbeddingStatus: model.AgentMemoryEmbeddingPending,
	}
	if err := db.Create(&mem).Error; err != nil {
		t.Fatalf("seed memory row: %v", err)
	}
	t.Cleanup(func() { db.Where("org_id = ?", orgID).Delete(&model.AgentMemory{}) })
	return mem.ID
}

// TestIntegration_MemoriesListChannelVisibility covers L3: the flat list must
// hide memories in channels the member cannot use, while global and usable rows
// remain; owners see everything.
func TestIntegration_MemoriesListChannelVisibility(t *testing.T) {
	h := newMemoryHTTPHarness(t)
	fx := (&channelHarness{db: h.db}).seed(t)
	ch := seedTeamVisibility(t, h.db, fx)

	global := seedMemoryRow(t, h.db, fx.org.ID, nil, "global memory")
	usable := seedMemoryRow(t, h.db, fx.org.ID, &ch.usable.ID, "usable channel memory")
	unusable := seedMemoryRow(t, h.db, fx.org.ID, &ch.unusable.ID, "unusable channel memory")

	decode := func(rr *httptest.ResponseRecorder) map[string]bool {
		if rr.Code != http.StatusOK {
			t.Fatalf("list status=%d body=%s", rr.Code, rr.Body.String())
		}
		var out struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode list: %v", err)
		}
		got := map[string]bool{}
		for _, d := range out.Data {
			got[d.ID] = true
		}
		return got
	}

	memberGot := decode(h.getJWT(t, "/v1/memories?limit=100", fx, fx.member))
	if !memberGot[global.String()] || !memberGot[usable.String()] {
		t.Fatalf("member should see global + usable memories: %v", memberGot)
	}
	if memberGot[unusable.String()] {
		t.Fatalf("LEAK: member saw memory in unusable channel")
	}

	ownerGot := decode(h.getJWT(t, "/v1/memories?limit=100", fx, fx.owner))
	if !ownerGot[global.String()] || !ownerGot[usable.String()] || !ownerGot[unusable.String()] {
		t.Fatalf("owner should see all memories: %v", ownerGot)
	}
}

// TestIntegration_MemoriesGroupedVisibility covers L4: the grouped view must not
// surface an unusable channel (or its name) to a member.
func TestIntegration_MemoriesGroupedVisibility(t *testing.T) {
	h := newMemoryHTTPHarness(t)
	fx := (&channelHarness{db: h.db}).seed(t)
	ch := seedTeamVisibility(t, h.db, fx)

	seedMemoryRow(t, h.db, fx.org.ID, nil, "global memory")
	seedMemoryRow(t, h.db, fx.org.ID, &ch.usable.ID, "usable channel memory")
	seedMemoryRow(t, h.db, fx.org.ID, &ch.unusable.ID, "unusable channel memory")

	decode := func(rr *httptest.ResponseRecorder) map[string]bool {
		if rr.Code != http.StatusOK {
			t.Fatalf("grouped status=%d body=%s", rr.Code, rr.Body.String())
		}
		var out struct {
			Groups []struct {
				ChannelID   *string `json:"channel_id"`
				ChannelName string  `json:"channel_name"`
			} `json:"groups"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode grouped: %v", err)
		}
		got := map[string]bool{}
		for _, g := range out.Groups {
			if g.ChannelID != nil {
				got[*g.ChannelID] = true
			}
		}
		return got
	}

	memberGot := decode(h.getJWT(t, "/v1/memories/grouped", fx, fx.member))
	if !memberGot[ch.usable.ID.String()] {
		t.Fatalf("member grouped should include usable channel: %v", memberGot)
	}
	if memberGot[ch.unusable.ID.String()] {
		t.Fatalf("LEAK: member grouped surfaced unusable channel")
	}

	ownerGot := decode(h.getJWT(t, "/v1/memories/grouped", fx, fx.owner))
	if !ownerGot[ch.unusable.ID.String()] {
		t.Fatalf("owner grouped should include unusable channel: %v", ownerGot)
	}
}

// TestIntegration_MemoriesByChannel403 covers L5: a by-channel read of a channel
// the member cannot use is denied (403), while a usable channel and the global
// pseudo-channel return 200.
func TestIntegration_MemoriesByChannel403(t *testing.T) {
	h := newMemoryHTTPHarness(t)
	fx := (&channelHarness{db: h.db}).seed(t)
	ch := seedTeamVisibility(t, h.db, fx)
	seedMemoryRow(t, h.db, fx.org.ID, &ch.usable.ID, "usable channel memory")
	seedMemoryRow(t, h.db, fx.org.ID, &ch.unusable.ID, "unusable channel memory")

	// Member is denied the other team's channel.
	denied := h.getJWT(t, "/v1/memories/channels/"+ch.unusable.ID.String(), fx, fx.member)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("member unusable-channel status=%d body=%s, want 403", denied.Code, denied.Body.String())
	}

	// Member may read their own team's channel.
	okUsable := h.getJWT(t, "/v1/memories/channels/"+ch.usable.ID.String(), fx, fx.member)
	if okUsable.Code != http.StatusOK {
		t.Fatalf("member usable-channel status=%d body=%s, want 200", okUsable.Code, okUsable.Body.String())
	}

	// Global (channel-less) is open to everyone in the org.
	okGlobal := h.getJWT(t, "/v1/memories/channels/global", fx, fx.member)
	if okGlobal.Code != http.StatusOK {
		t.Fatalf("member global status=%d body=%s, want 200", okGlobal.Code, okGlobal.Body.String())
	}

	// Owner may read the unusable channel.
	okOwner := h.getJWT(t, "/v1/memories/channels/"+ch.unusable.ID.String(), fx, fx.owner)
	if okOwner.Code != http.StatusOK {
		t.Fatalf("owner unusable-channel status=%d body=%s, want 200", okOwner.Code, okOwner.Body.String())
	}
}
