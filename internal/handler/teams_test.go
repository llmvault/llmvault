package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
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
	if detail.Team.MemberCount != 1 || len(detail.Members) != 1 || detail.Members[0].UserID != member.ID.String() {
		t.Fatalf("bad team members response: %+v", detail)
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
