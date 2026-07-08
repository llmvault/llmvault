package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type agentVisFixture struct {
	org         model.Org
	admin       model.User
	member      model.User
	visibleTeam model.Agent // owned by the member's team; visible
	otherTeam   model.Agent // owned by another team; hidden
	teamNull    model.Agent // no team scope; hidden from the member
}

func seedAgentVisFixture(t *testing.T, db *gorm.DB) agentVisFixture {
	t.Helper()
	org := model.Org{ID: uuid.New(), Name: "agvis-" + uuid.NewString()[:8], RateLimit: 1000, Active: true}
	admin := model.User{ID: uuid.New(), Email: "admin-" + uuid.NewString()[:8] + "@test.com", Name: "admin"}
	member := model.User{ID: uuid.New(), Email: "member-" + uuid.NewString()[:8] + "@test.com", Name: "member"}

	teamA := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "team-a-" + uuid.NewString()[:8]}
	teamB := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "team-b-" + uuid.NewString()[:8]}

	mkAgent := func(name string, team *uuid.UUID) model.Agent {
		return model.Agent{ID: uuid.New(), OrgID: &org.ID, Name: name, Model: "test", Status: "active", TeamID: team}
	}
	// Agent visibility is team-membership based: a non-manager sees exactly the
	// agents belonging to teams they are on.
	visibleTeam := mkAgent("VisibleTeam", &teamA.ID)
	otherTeam := mkAgent("OtherTeam", &teamB.ID)
	teamNull := mkAgent("TeamNull", nil)

	rows := []any{
		&org, &admin, &member,
		&model.OrgMembership{UserID: admin.ID, OrgID: org.ID, Role: "admin"},
		&model.OrgMembership{UserID: member.ID, OrgID: org.ID, Role: "member"},
		&teamA, &teamB,
		&visibleTeam, &otherTeam, &teamNull,
		&model.TeamMember{OrgID: org.ID, TeamID: teamA.ID, UserID: member.ID, Role: "member"},
	}
	for _, r := range rows {
		if err := db.Create(r).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.TeamMember{})
		db.Where("org_id = ?", org.ID).Delete(&model.Team{})
		db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		db.Where("id IN ?", []uuid.UUID{admin.ID, member.ID}).Delete(&model.User{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})
	return agentVisFixture{
		org: org, admin: admin, member: member,
		visibleTeam: visibleTeam, otherTeam: otherTeam, teamNull: teamNull,
	}
}

type agentListVisResp struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
	HasMore bool `json:"has_more"`
}

// listAgentIDs drives AgentHandler.List for the given caller and returns the set
// of agent ids in the response. Exactly one of user / apiKey identifies the
// caller (nil user + apiKey=false is an anonymous-ish member with no role).
func listAgentIDs(t *testing.T, h *handler.AgentHandler, org model.Org, user *model.User, apiKey bool) map[string]bool {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/agents", nil)
	req = middleware.WithOrg(req, &org)
	if apiKey {
		req = middleware.WithAPIKeyClaims(req, &middleware.APIKeyClaims{OrgID: org.ID.String()})
	} else if user != nil {
		req = middleware.WithUser(req, user)
	}
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("List status = %d, body=%s", rr.Code, rr.Body.String())
	}
	var resp agentListVisResp
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode list: %v (body=%s)", err, rr.Body.String())
	}
	ids := make(map[string]bool, len(resp.Data))
	for _, item := range resp.Data {
		ids[item.ID] = true
	}
	return ids
}

func getAgentStatus(t *testing.T, h *handler.AgentHandler, org model.Org, user *model.User, agentID uuid.UUID) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/agents/"+agentID.String(), nil)
	req = middleware.WithOrg(req, &org)
	if user != nil {
		req = middleware.WithUser(req, user)
	}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", agentID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.Get(rr, req)
	return rr.Code
}

func TestAgentList_ActorScopedVisibility(t *testing.T) {
	db := connectTestDB(t)
	h := newAgentHandlerForTest(db)
	fx := seedAgentVisFixture(t, db)

	// A plain member sees only agents owned by teams they belong to.
	memberIDs := listAgentIDs(t, h, fx.org, &fx.member, false)
	if !memberIDs[fx.visibleTeam.ID.String()] {
		t.Fatalf("member list missing %s (%s); got %v", fx.visibleTeam.Name, fx.visibleTeam.ID, memberIDs)
	}
	for _, hidden := range []model.Agent{fx.otherTeam, fx.teamNull} {
		if memberIDs[hidden.ID.String()] {
			t.Fatalf("member list must NOT include %s (%s); got %v", hidden.Name, hidden.ID, memberIDs)
		}
	}

	all := []model.Agent{fx.visibleTeam, fx.otherTeam, fx.teamNull}

	// An org manager sees every agent.
	adminIDs := listAgentIDs(t, h, fx.org, &fx.admin, false)
	// An API-key caller sees every agent.
	apiKeyIDs := listAgentIDs(t, h, fx.org, nil, true)
	for _, a := range all {
		if !adminIDs[a.ID.String()] {
			t.Fatalf("admin list missing %s (%s)", a.Name, a.ID)
		}
		if !apiKeyIDs[a.ID.String()] {
			t.Fatalf("api-key list missing %s (%s)", a.Name, a.ID)
		}
	}
}

func TestAgentGet_ActorScopedVisibility(t *testing.T) {
	db := connectTestDB(t)
	h := newAgentHandlerForTest(db)
	fx := seedAgentVisFixture(t, db)

	// The member can fetch a visible agent.
	if code := getAgentStatus(t, h, fx.org, &fx.member, fx.visibleTeam.ID); code != http.StatusOK {
		t.Fatalf("member get visible agent = %d, want 200", code)
	}
	// A hidden agent returns 404 (not 403) so its existence never leaks.
	for _, hidden := range []model.Agent{fx.otherTeam, fx.teamNull} {
		if code := getAgentStatus(t, h, fx.org, &fx.member, hidden.ID); code != http.StatusNotFound {
			t.Fatalf("member get hidden %s = %d, want 404", hidden.Name, code)
		}
	}
	// A manager can fetch any agent.
	if code := getAgentStatus(t, h, fx.org, &fx.admin, fx.otherTeam.ID); code != http.StatusOK {
		t.Fatalf("admin get any agent = %d, want 200", code)
	}
}
