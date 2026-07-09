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

// agentAuthzFixture holds the actors and teams used across the team-scoped agent
// authorization tests.
type agentAuthzFixture struct {
	org     model.Org
	owner   model.User // org owner => manager
	memberA model.User // org member, on team A only
	teamA   model.Team
	teamB   model.Team
	handler *handler.AgentHandler
	router  *chi.Mux
	db      *gorm.DB
}

func newAgentAuthzHarness(t *testing.T) agentAuthzFixture {
	t.Helper()
	db := connectTestDB(t)
	seedDefaultModelCredential(t, db)
	org := createTestOrg(t, db)

	owner := seedOrgUser(t, db, org.ID, "owner")
	memberA := seedOrgUser(t, db, org.ID, "member")
	teamA := seedTeam(t, db, org.ID, "team-a")
	teamB := seedTeam(t, db, org.ID, "team-b")
	seedTeamMember(t, db, org.ID, teamA.ID, memberA.ID)

	h := newAgentHandlerForTest(db)
	r := chi.NewRouter()
	r.Route("/v1", func(r chi.Router) {
		r.Post("/agents", h.Create)
		r.Patch("/agents/{id}", h.Update)
		r.Delete("/agents/{id}", h.Archive)
	})
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		db.Where("org_id = ?", org.ID).Delete(&model.TeamMember{})
		db.Where("org_id = ?", org.ID).Delete(&model.Team{})
		db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
	})
	return agentAuthzFixture{org: org, owner: owner, memberA: memberA, teamA: teamA, teamB: teamB, handler: h, router: r, db: db}
}

func seedOrgUser(t *testing.T, db *gorm.DB, orgID uuid.UUID, role string) model.User {
	t.Helper()
	user := model.User{ID: uuid.New(), Email: role + "-" + uuid.NewString()[:8] + "@authz.test", Name: role, PasswordHash: "x"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Create(&model.OrgMembership{UserID: user.ID, OrgID: orgID, Role: role}).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", user.ID).Delete(&model.User{}) })
	return user
}

func seedTeam(t *testing.T, db *gorm.DB, orgID uuid.UUID, name string) model.Team {
	t.Helper()
	team := model.Team{OrgID: orgID, Name: name + "-" + uuid.NewString()[:8]}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	return team
}

func seedTeamMember(t *testing.T, db *gorm.DB, orgID, teamID, userID uuid.UUID) {
	t.Helper()
	if err := db.Create(&model.TeamMember{OrgID: orgID, TeamID: teamID, UserID: userID}).Error; err != nil {
		t.Fatalf("create team member: %v", err)
	}
}

// seedTeamAgent inserts an active agent owned by a team.
func seedTeamAgent(t *testing.T, db *gorm.DB, orgID uuid.UUID, teamID uuid.UUID) model.Agent {
	t.Helper()
	agent := model.Agent{
		OrgID:         &orgID,
		TeamID:        teamID,
		Name:          "authz-agent-" + uuid.NewString()[:8],
		Model:         "deepseek-v4-flash",
		Tools:         model.JSON{},
		McpServers:    model.RawJSON("[]"),
		Skills:        model.JSON{},
		RuntimeConfig: model.JSON{},
		Permissions:   model.JSON{},
		Resources:     model.JSON{},
		Status:        "active",
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return agent
}

// doAgentReq issues a request against the agent router as the given user (nil =>
// no human actor, i.e. a system/trusted context). It sets both the org context
// and JWT auth claims, mirroring how the auth middleware populates the request.
func (fx agentAuthzFixture) doAgentReq(t *testing.T, method, path string, user *model.User, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	org := fx.org
	req = middleware.WithOrg(req, &org)
	if user != nil {
		req = middleware.WithAuthClaims(req, &auth.AuthClaims{UserID: user.ID.String(), OrgID: fx.org.ID.String()})
	}
	rr := httptest.NewRecorder()
	fx.router.ServeHTTP(rr, req)
	return rr
}

// --- create ------------------------------------------------------------------

func TestAgentCreate_MemberCreatesInOwnTeam(t *testing.T) {
	fx := newAgentAuthzHarness(t)
	rr := fx.doAgentReq(t, http.MethodPost, "/v1/agents", &fx.memberA,
		map[string]any{"name": "Team A Agent", "team_id": fx.teamA.ID.String()})
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d, want 201; body=%s", rr.Code, rr.Body.String())
	}
	var out struct {
		Agent struct {
			ID string `json:"id"`
		} `json:"agent"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	var stored model.Agent
	if err := fx.db.First(&stored, "id = ?", out.Agent.ID).Error; err != nil {
		t.Fatalf("load stored: %v", err)
	}
	if stored.TeamID != fx.teamA.ID {
		t.Fatalf("stored team_id=%v, want %v", stored.TeamID, fx.teamA.ID)
	}
}

func TestAgentCreate_MemberCannotCreateInForeignTeam(t *testing.T) {
	fx := newAgentAuthzHarness(t)
	rr := fx.doAgentReq(t, http.MethodPost, "/v1/agents", &fx.memberA,
		map[string]any{"name": "Team B Agent", "team_id": fx.teamB.ID.String()})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403; body=%s", rr.Code, rr.Body.String())
	}
}

func TestAgentCreate_MemberWithoutTeamRejected(t *testing.T) {
	fx := newAgentAuthzHarness(t)
	rr := fx.doAgentReq(t, http.MethodPost, "/v1/agents", &fx.memberA,
		map[string]any{"name": "No Team Agent"})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403; body=%s", rr.Code, rr.Body.String())
	}
}

func TestAgentCreate_ManagerCreatesAnywhere(t *testing.T) {
	fx := newAgentAuthzHarness(t)
	// In a team the manager is not a member of.
	inTeam := fx.doAgentReq(t, http.MethodPost, "/v1/agents", &fx.owner,
		map[string]any{"name": "Mgr Team B Agent", "team_id": fx.teamB.ID.String()})
	if inTeam.Code != http.StatusCreated {
		t.Fatalf("manager+team status=%d, want 201; body=%s", inTeam.Code, inTeam.Body.String())
	}
	// Agents always belong to a team: even a manager cannot create one without.
	noTeam := fx.doAgentReq(t, http.MethodPost, "/v1/agents", &fx.owner,
		map[string]any{"name": "Mgr No Team Agent"})
	if noTeam.Code != http.StatusUnprocessableEntity {
		t.Fatalf("manager+noteam status=%d, want 422; body=%s", noTeam.Code, noTeam.Body.String())
	}
}

func TestAgentCreate_UnknownTeamRejected(t *testing.T) {
	fx := newAgentAuthzHarness(t)
	rr := fx.doAgentReq(t, http.MethodPost, "/v1/agents", &fx.owner,
		map[string]any{"name": "Ghost Team Agent", "team_id": uuid.NewString()})
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d, want 422; body=%s", rr.Code, rr.Body.String())
	}
}

// --- update / archive --------------------------------------------------------

func TestAgentUpdate_MemberInTeamAllowed(t *testing.T) {
	fx := newAgentAuthzHarness(t)
	agent := seedTeamAgent(t, fx.db, fx.org.ID, fx.teamA.ID)
	rr := fx.doAgentReq(t, http.MethodPatch, "/v1/agents/"+agent.ID.String(), &fx.memberA,
		map[string]any{"description": "edited by team member"})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", rr.Code, rr.Body.String())
	}
}

func TestAgentUpdate_CrossTeamMemberDenied(t *testing.T) {
	fx := newAgentAuthzHarness(t)
	agent := seedTeamAgent(t, fx.db, fx.org.ID, fx.teamB.ID)
	rr := fx.doAgentReq(t, http.MethodPatch, "/v1/agents/"+agent.ID.String(), &fx.memberA,
		map[string]any{"description": "should be blocked"})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403; body=%s", rr.Code, rr.Body.String())
	}
}

func TestAgentArchive_CrossTeamMemberDenied(t *testing.T) {
	fx := newAgentAuthzHarness(t)
	agent := seedTeamAgent(t, fx.db, fx.org.ID, fx.teamB.ID)
	denied := fx.doAgentReq(t, http.MethodDelete, "/v1/agents/"+agent.ID.String(), &fx.memberA, nil)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("cross-team archive status=%d, want 403; body=%s", denied.Code, denied.Body.String())
	}
	// The owner (manager) may archive it.
	ok := fx.doAgentReq(t, http.MethodDelete, "/v1/agents/"+agent.ID.String(), &fx.owner, nil)
	if ok.Code != http.StatusOK {
		t.Fatalf("manager archive status=%d, want 200; body=%s", ok.Code, ok.Body.String())
	}
}
