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

type teamHarness struct {
	db     *gorm.DB
	router *chi.Mux
}

func newTeamHarness(t *testing.T) *teamHarness {
	t.Helper()
	db := connectTestDB(t)
	h := handler.NewTeamHandler(db)
	r := chi.NewRouter()
	r.Route("/v1", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(db))
		r.Get("/orgs/current/teams", h.List)
		r.Post("/orgs/current/teams", h.Create)
		r.Get("/orgs/current/teams/{id}", h.Get)
		r.Delete("/orgs/current/teams/{id}", h.Archive)
		r.Put("/orgs/current/teams/{id}/members/{userID}", h.PutMember)
	})
	return &teamHarness{db: db, router: r}
}

func TestIntegration_TeamsCreateListAndMembers(t *testing.T) {
	h := newTeamHarness(t)
	org := createTestOrg(t, h.db)
	owner := seedSessionUser(t, h.db, org.ID, "owner")
	member := seedSessionUser(t, h.db, org.ID, "member")
	t.Cleanup(func() {
		h.db.Where("org_id = ?", org.ID).Delete(&model.TeamMember{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.Team{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		h.db.Where("id = ? OR id = ?", owner.ID, member.ID).Delete(&model.User{})
	})

	create := h.doJSON(t, http.MethodPost, "/v1/orgs/current/teams", org, owner, map[string]any{
		"name":        "Engineering",
		"description": "Product engineering",
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	var created struct {
		Team struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"team"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v\n%s", err, create.Body.String())
	}
	if created.Team.Name != "Engineering" {
		t.Fatalf("team name=%q", created.Team.Name)
	}

	add := h.doJSON(t, http.MethodPut, "/v1/orgs/current/teams/"+created.Team.ID+"/members/"+member.ID.String(), org, owner, nil)
	if add.Code != http.StatusOK {
		t.Fatalf("add member status=%d body=%s", add.Code, add.Body.String())
	}
	var detail struct {
		Team struct {
			MemberCount int64 `json:"member_count"`
		} `json:"team"`
		Members []struct {
			UserID string `json:"user_id"`
		} `json:"members"`
	}
	if err := json.Unmarshal(add.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode add: %v\n%s", err, add.Body.String())
	}
	// The creator (owner) is auto-added as an owner member on team creation, so
	// after adding `member` the team has two members.
	if detail.Team.MemberCount != 2 || len(detail.Members) != 2 {
		t.Fatalf("bad team members response: %+v", detail)
	}
	seen := map[string]bool{}
	for _, m := range detail.Members {
		seen[m.UserID] = true
	}
	if !seen[owner.ID.String()] || !seen[member.ID.String()] {
		t.Fatalf("members missing owner/member: %+v", detail.Members)
	}
}

func TestIntegration_TeamsArchiveRejectsAssignedChannels(t *testing.T) {
	h := newTeamHarness(t)
	org := createTestOrg(t, h.db)
	owner := seedSessionUser(t, h.db, org.ID, "owner")
	agent := seedSessionAgent(t, h.db, org.ID)
	t.Cleanup(func() {
		h.db.Where("org_id = ?", org.ID).Delete(&model.Channel{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.TeamMember{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.Team{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		h.db.Where("id = ?", owner.ID).Delete(&model.User{})
	})
	team := model.Team{OrgID: org.ID, Name: "Support", CreatedBy: &owner.ID}
	if err := h.db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	// A second team so the last-team guard does not mask the channel guard.
	other := model.Team{OrgID: org.ID, Name: "Other", CreatedBy: &owner.ID}
	if err := h.db.Create(&other).Error; err != nil {
		t.Fatalf("create second team: %v", err)
	}
	channel := model.Channel{
		OrgID:          org.ID,
		Name:           "support",
		Kind:           "standard",
		Visibility:     "public",
		TeamID:         &team.ID,
		DefaultAgentID: agent.ID,
		Origin:         "native",
		CreatedBy:      &owner.ID,
	}
	if err := h.db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}

	archive := h.doJSON(t, http.MethodDelete, "/v1/orgs/current/teams/"+team.ID.String(), org, owner, nil)
	if archive.Code != http.StatusConflict {
		t.Fatalf("archive status=%d body=%s", archive.Code, archive.Body.String())
	}
}

// TestIntegration_TeamsListAndGetScopedToMember asserts the member-reachable
// scoping on the teams read surface: an org manager sees every team, a plain
// member sees only the teams they belong to, and a member cannot GET a team of
// which they are not a member (404, indistinguishable from nonexistent).
func TestIntegration_TeamsListAndGetScopedToMember(t *testing.T) {
	h := newTeamHarness(t)
	org := createTestOrg(t, h.db)
	owner := seedSessionUser(t, h.db, org.ID, "owner")
	m1 := seedSessionUser(t, h.db, org.ID, "member")
	m2 := seedSessionUser(t, h.db, org.ID, "member")
	t.Cleanup(func() {
		h.db.Where("org_id = ?", org.ID).Delete(&model.TeamMember{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.Team{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		h.db.Where("id IN ?", []uuid.UUID{owner.ID, m1.ID, m2.ID}).Delete(&model.User{})
	})

	teamA := model.Team{OrgID: org.ID, Name: "Team A", CreatedBy: &owner.ID}
	teamB := model.Team{OrgID: org.ID, Name: "Team B", CreatedBy: &owner.ID}
	for _, tm := range []*model.Team{&teamA, &teamB} {
		if err := h.db.Create(tm).Error; err != nil {
			t.Fatalf("create team: %v", err)
		}
	}
	if err := h.db.Create(&model.TeamMember{OrgID: org.ID, TeamID: teamA.ID, UserID: m1.ID, Role: "member"}).Error; err != nil {
		t.Fatalf("add m1 to teamA: %v", err)
	}
	if err := h.db.Create(&model.TeamMember{OrgID: org.ID, TeamID: teamB.ID, UserID: m2.ID, Role: "member"}).Error; err != nil {
		t.Fatalf("add m2 to teamB: %v", err)
	}

	listIDs := func(rr *httptest.ResponseRecorder) map[string]bool {
		t.Helper()
		if rr.Code != http.StatusOK {
			t.Fatalf("list status=%d body=%s", rr.Code, rr.Body.String())
		}
		var out struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode list: %v\n%s", err, rr.Body.String())
		}
		ids := map[string]bool{}
		for _, d := range out.Data {
			ids[d.ID] = true
		}
		return ids
	}

	// Manager sees every team in the org.
	ownerIDs := listIDs(h.doJSON(t, http.MethodGet, "/v1/orgs/current/teams", org, owner, nil))
	if !ownerIDs[teamA.ID.String()] || !ownerIDs[teamB.ID.String()] {
		t.Fatalf("owner should see both teams, got %v", ownerIDs)
	}

	// Plain member sees ONLY the team they belong to.
	m1IDs := listIDs(h.doJSON(t, http.MethodGet, "/v1/orgs/current/teams", org, m1, nil))
	if !m1IDs[teamA.ID.String()] || m1IDs[teamB.ID.String()] {
		t.Fatalf("m1 should see only teamA, got %v", m1IDs)
	}

	// Member may GET a team they belong to...
	if rr := h.doJSON(t, http.MethodGet, "/v1/orgs/current/teams/"+teamA.ID.String(), org, m1, nil); rr.Code != http.StatusOK {
		t.Fatalf("m1 GET own team: status=%d body=%s", rr.Code, rr.Body.String())
	}
	// ...but not a foreign team (404, not leaked).
	if rr := h.doJSON(t, http.MethodGet, "/v1/orgs/current/teams/"+teamB.ID.String(), org, m1, nil); rr.Code != http.StatusNotFound {
		t.Fatalf("m1 GET foreign team: status=%d want 404 body=%s", rr.Code, rr.Body.String())
	}
	// A manager may GET any team.
	if rr := h.doJSON(t, http.MethodGet, "/v1/orgs/current/teams/"+teamB.ID.String(), org, owner, nil); rr.Code != http.StatusOK {
		t.Fatalf("owner GET any team: status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func (h *teamHarness) doJSON(t *testing.T, method, path string, org model.Org, user model.User, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-ID", org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{UserID: user.ID.String(), OrgID: org.ID.String()})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}
