package handler_test

import (
	"bytes"
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

type pluginAgentAuthzFixture struct {
	org     model.Org
	owner   model.User
	memberA model.User
	teamA   model.Team
	teamB   model.Team
	plugin  model.Plugin
	router  *chi.Mux
	db      *gorm.DB
}

func newPluginAgentAuthzHarness(t *testing.T) pluginAgentAuthzFixture {
	t.Helper()
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	owner := seedOrgUser(t, db, org.ID, "owner")
	memberA := seedOrgUser(t, db, org.ID, "member")
	teamA := seedTeam(t, db, org.ID, "pa-teamA")
	teamB := seedTeam(t, db, org.ID, "pa-teamB")
	seedTeamMember(t, db, org.ID, teamA.ID, memberA.ID)

	// An org-owned plugin, installed on the org.
	plugin := model.Plugin{
		ID:     uuid.New(),
		OrgID:  &org.ID,
		Slug:   "pa-plugin-" + uuid.NewString()[:8],
		Name:   "PA Plugin",
		Status: model.PluginStatusActive,
	}
	if err := db.Create(&plugin).Error; err != nil {
		t.Fatalf("create plugin: %v", err)
	}
	if err := db.Create(&model.OrgPluginInstall{ID: uuid.New(), OrgID: org.ID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("install plugin for org: %v", err)
	}

	ph := handler.NewPluginHandler(db)
	r := chi.NewRouter()
	r.Route("/v1", func(r chi.Router) {
		r.Post("/agents/{id}/plugins/{slug}", ph.EnableForAgent)
		r.Delete("/agents/{id}/plugins/{slug}", ph.DisableForAgent)
	})
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.AgentPluginInstall{})
		db.Where("org_id = ?", org.ID).Delete(&model.OrgPluginInstall{})
		db.Where("org_id = ?", org.ID).Delete(&model.TeamPlugin{})
		db.Where("org_id = ?", org.ID).Delete(&model.Plugin{})
		db.Where("org_id = ?", org.ID).Delete(&model.Channel{})
		db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		db.Where("org_id = ?", org.ID).Delete(&model.TeamMember{})
		db.Where("org_id = ?", org.ID).Delete(&model.Team{})
		db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
	})
	return pluginAgentAuthzFixture{org: org, owner: owner, memberA: memberA, teamA: teamA, teamB: teamB, plugin: plugin, router: r, db: db}
}

func (fx pluginAgentAuthzFixture) grantPluginToTeam(t *testing.T, teamID uuid.UUID) {
	t.Helper()
	if err := fx.db.Create(&model.TeamPlugin{OrgID: fx.org.ID, TeamID: teamID, PluginID: fx.plugin.ID}).Error; err != nil {
		t.Fatalf("grant plugin to team: %v", err)
	}
}

func (fx pluginAgentAuthzFixture) do(t *testing.T, method, path string, user model.User) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, &bytes.Buffer{})
	org := fx.org
	req = middleware.WithOrg(req, &org)
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{UserID: user.ID.String(), OrgID: fx.org.ID.String()})
	rr := httptest.NewRecorder()
	fx.router.ServeHTTP(rr, req)
	return rr
}

func (fx pluginAgentAuthzFixture) enablePath(agentID uuid.UUID) string {
	return "/v1/agents/" + agentID.String() + "/plugins/" + fx.plugin.Slug
}

// Enabling a plugin on a team agent is bounded by the team's plugin grant.
func TestPluginEnable_BoundedByTeamGrant(t *testing.T) {
	fx := newPluginAgentAuthzHarness(t)
	agent := seedTeamAgent(t, fx.db, fx.org.ID, &fx.teamA.ID)

	// Not granted to the agent's team yet -> 422 even for a manager.
	blocked := fx.do(t, http.MethodPost, fx.enablePath(agent.ID), fx.owner)
	if blocked.Code != http.StatusUnprocessableEntity {
		t.Fatalf("ungranted enable status=%d, want 422; body=%s", blocked.Code, blocked.Body.String())
	}

	// Grant the plugin to the team -> enable succeeds.
	fx.grantPluginToTeam(t, fx.teamA.ID)
	ok := fx.do(t, http.MethodPost, fx.enablePath(agent.ID), fx.owner)
	if ok.Code != http.StatusOK {
		t.Fatalf("granted enable status=%d, want 200; body=%s", ok.Code, ok.Body.String())
	}
}

// An auto-install system plugin is always usable by any team's agents: enabling
// it on a team-scoped agent succeeds WITHOUT a team_plugins grant, unlike an
// optional plugin which requires the grant (see TestPluginEnable_BoundedByTeamGrant).
func TestPluginEnable_AutoInstallSkipsTeamGrant(t *testing.T) {
	fx := newPluginAgentAuthzHarness(t)
	agent := seedTeamAgent(t, fx.db, fx.org.ID, &fx.teamA.ID)

	// A global auto-install plugin, installed for the org, with no team grant.
	sys := model.Plugin{
		ID:       uuid.New(),
		Slug:     "pa-sys-" + uuid.NewString()[:8],
		Name:     "PA System Plugin",
		Status:   model.PluginStatusActive,
		Manifest: model.RawJSON(`{"auto_install": true, "locked": true}`),
	}
	if err := fx.db.Create(&sys).Error; err != nil {
		t.Fatalf("create system plugin: %v", err)
	}
	if err := fx.db.Create(&model.OrgPluginInstall{ID: uuid.New(), OrgID: fx.org.ID, PluginID: sys.ID}).Error; err != nil {
		t.Fatalf("install system plugin for org: %v", err)
	}
	t.Cleanup(func() {
		fx.db.Where("plugin_id = ?", sys.ID).Delete(&model.OrgPluginInstall{})
		fx.db.Where("plugin_id = ?", sys.ID).Delete(&model.AgentPluginInstall{})
		fx.db.Where("id = ?", sys.ID).Delete(&model.Plugin{})
	})

	path := "/v1/agents/" + agent.ID.String() + "/plugins/" + sys.Slug
	rr := fx.do(t, http.MethodPost, path, fx.owner)
	if rr.Code != http.StatusOK {
		t.Fatalf("auto-install enable without grant status=%d, want 200; body=%s", rr.Code, rr.Body.String())
	}
}

// A member may enable a team-granted plugin on an agent in their own team.
func TestPluginEnable_MemberInTeamAllowed(t *testing.T) {
	fx := newPluginAgentAuthzHarness(t)
	agent := seedTeamAgent(t, fx.db, fx.org.ID, &fx.teamA.ID)
	fx.grantPluginToTeam(t, fx.teamA.ID)
	// Make the agent visible to the member via a no-team channel.
	fx.assignAgentToVisibleChannel(t, agent.ID)

	rr := fx.do(t, http.MethodPost, fx.enablePath(agent.ID), fx.memberA)
	if rr.Code != http.StatusOK {
		t.Fatalf("member enable status=%d, want 200; body=%s", rr.Code, rr.Body.String())
	}
}

// A member cannot enable a plugin on an agent whose team they are not in. Under
// the team-primary model such an agent is not even visible to the member, so the
// route returns 404 (never leaking the agent's existence) rather than 403.
func TestPluginEnable_CrossTeamMemberDenied(t *testing.T) {
	fx := newPluginAgentAuthzHarness(t)
	agent := seedTeamAgent(t, fx.db, fx.org.ID, &fx.teamB.ID)
	fx.grantPluginToTeam(t, fx.teamB.ID)

	rr := fx.do(t, http.MethodPost, fx.enablePath(agent.ID), fx.memberA)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("cross-team member enable status=%d, want 404; body=%s", rr.Code, rr.Body.String())
	}
}

// Org-level agents (team_id IS NULL, e.g. the Hivy default) skip the team-grant
// bound and are manager-only; a manager may enable any org-installed plugin.
func TestPluginEnable_OrgLevelAgentSkipsGrant(t *testing.T) {
	fx := newPluginAgentAuthzHarness(t)
	agent := seedTeamAgent(t, fx.db, fx.org.ID, nil)
	rr := fx.do(t, http.MethodPost, fx.enablePath(agent.ID), fx.owner)
	if rr.Code != http.StatusOK {
		t.Fatalf("org-level enable status=%d, want 200; body=%s", rr.Code, rr.Body.String())
	}
}

// Disabling a plugin is gated on managing the agent's team; a cross-team member
// cannot see the agent, so the route returns 404 (non-leaking) not 403.
func TestPluginDisable_CrossTeamMemberDenied(t *testing.T) {
	fx := newPluginAgentAuthzHarness(t)
	agent := seedTeamAgent(t, fx.db, fx.org.ID, &fx.teamB.ID)
	fx.grantPluginToTeam(t, fx.teamB.ID)
	// Enable it directly so there is something to disable.
	if err := fx.db.Create(&model.AgentPluginInstall{OrgID: fx.org.ID, AgentID: agent.ID, PluginID: fx.plugin.ID}).Error; err != nil {
		t.Fatalf("seed agent plugin install: %v", err)
	}

	rr := fx.do(t, http.MethodDelete, fx.enablePath(agent.ID), fx.memberA)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("cross-team member disable status=%d, want 404; body=%s", rr.Code, rr.Body.String())
	}
}

// assignAgentToVisibleChannel is a no-op under the team-primary model: the test
// agents belong to teamA and memberA is a member of teamA, so agent visibility
// (team-membership based) already holds without any per-channel assignment. It
// is retained so the call sites read as "this agent is member-visible".
func (fx pluginAgentAuthzFixture) assignAgentToVisibleChannel(t *testing.T, agentID uuid.UUID) {
	t.Helper()
	_ = agentID
}
