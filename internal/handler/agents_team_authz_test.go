package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/pluginresolve"
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

func TestAgentUpdate_MemberDisablesInheritedPluginForOnlyThatAgent(t *testing.T) {
	fx := newAgentAuthzHarness(t)
	plugin := model.Plugin{
		ID:          uuid.New(),
		OrgID:       &fx.org.ID,
		Slug:        "agent-override-" + uuid.NewString()[:8],
		Name:        "Agent Override",
		Status:      model.PluginStatusActive,
		Description: "test plugin",
	}
	if err := fx.db.Create(&plugin).Error; err != nil {
		t.Fatalf("create plugin: %v", err)
	}
	if err := fx.db.Create(&model.OrgPluginInstall{ID: uuid.New(), OrgID: fx.org.ID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("install plugin: %v", err)
	}
	if err := fx.db.Create(&model.TeamPlugin{OrgID: fx.org.ID, TeamID: fx.teamA.ID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("grant plugin to team: %v", err)
	}
	t.Cleanup(func() { fx.db.Where("id = ?", plugin.ID).Delete(&model.Plugin{}) })

	disabledAgent := seedTeamAgent(t, fx.db, fx.org.ID, fx.teamA.ID)
	enabledAgent := seedTeamAgent(t, fx.db, fx.org.ID, fx.teamA.ID)
	rr := fx.doAgentReq(t, http.MethodPatch, "/v1/agents/"+disabledAgent.ID.String(), &fx.memberA,
		map[string]any{"disabled_plugin_ids": []string{plugin.ID.String()}})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	var override model.AgentPluginOverride
	if err := fx.db.Where("agent_id = ? AND plugin_id = ?", disabledAgent.ID, plugin.ID).First(&override).Error; err != nil {
		t.Fatalf("load override: %v", err)
	}
	if override.OrgID != fx.org.ID || override.DisabledBy == nil || *override.DisabledBy != fx.memberA.ID {
		t.Fatalf("override = %#v, want org and disabling member", override)
	}

	disabledIDs, err := pluginresolve.EffectivePluginIDs(t.Context(), fx.db, disabledAgent)
	if err != nil {
		t.Fatalf("resolve disabled agent: %v", err)
	}
	if containsPluginID(disabledIDs, plugin.ID) {
		t.Fatal("disabled agent still has inherited plugin")
	}
	enabledIDs, err := pluginresolve.EffectivePluginIDs(t.Context(), fx.db, enabledAgent)
	if err != nil {
		t.Fatalf("resolve enabled agent: %v", err)
	}
	if !containsPluginID(enabledIDs, plugin.ID) {
		t.Fatal("agent override leaked to another team agent")
	}

	var response struct {
		Agent struct {
			DisabledPluginIDs []string `json:"disabled_plugin_ids"`
		} `json:"agent"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Agent.DisabledPluginIDs) != 1 || response.Agent.DisabledPluginIDs[0] != plugin.ID.String() {
		t.Fatalf("disabled_plugin_ids = %v, want [%s]", response.Agent.DisabledPluginIDs, plugin.ID)
	}
}

func TestAgentUpdate_CannotDisableCatalogRequiredPlugin(t *testing.T) {
	fx := newAgentAuthzHarness(t)
	plugin := model.Plugin{
		ID:          uuid.New(),
		OrgID:       &fx.org.ID,
		Slug:        "required-override-" + uuid.NewString()[:8],
		Name:        "Required Override",
		Status:      model.PluginStatusActive,
		Description: "test plugin",
	}
	if err := fx.db.Create(&plugin).Error; err != nil {
		t.Fatalf("create plugin: %v", err)
	}
	if err := fx.db.Create(&model.OrgPluginInstall{ID: uuid.New(), OrgID: fx.org.ID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("install plugin: %v", err)
	}
	if err := fx.db.Create(&model.TeamPlugin{OrgID: fx.org.ID, TeamID: fx.teamA.ID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("grant plugin to team: %v", err)
	}
	catalog := model.AgentCatalog{
		ID:              uuid.New(),
		Slug:            "required-catalog-" + uuid.NewString()[:8],
		Name:            "Required Catalog",
		Status:          model.AgentCatalogStatusActive,
		Manifest:        model.RawJSON(`{}`),
		RequiredPlugins: pq.StringArray{plugin.Slug},
	}
	if err := fx.db.Create(&catalog).Error; err != nil {
		t.Fatalf("create catalog: %v", err)
	}
	t.Cleanup(func() { fx.db.Where("id = ?", plugin.ID).Delete(&model.Plugin{}) })
	t.Cleanup(func() { fx.db.Where("id = ?", catalog.ID).Delete(&model.AgentCatalog{}) })

	agent := seedTeamAgent(t, fx.db, fx.org.ID, fx.teamA.ID)
	if err := fx.db.Model(&model.Agent{}).Where("id = ?", agent.ID).Update("agent_catalog_id", catalog.ID).Error; err != nil {
		t.Fatalf("attach catalog to agent: %v", err)
	}
	rr := fx.doAgentReq(t, http.MethodPatch, "/v1/agents/"+agent.ID.String(), &fx.memberA,
		map[string]any{"disabled_plugin_ids": []string{plugin.ID.String()}})
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d, want 422; body=%s", rr.Code, rr.Body.String())
	}
	var count int64
	if err := fx.db.Model(&model.AgentPluginOverride{}).
		Where("agent_id = ? AND plugin_id = ?", agent.ID, plugin.ID).
		Count(&count).Error; err != nil {
		t.Fatalf("count overrides: %v", err)
	}
	if count != 0 {
		t.Fatalf("stored %d overrides for required plugin", count)
	}
}

func containsPluginID(ids []uuid.UUID, want uuid.UUID) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
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
