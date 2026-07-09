package handler_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/model"
)

// catalogInstallFixture seeds an org with a member (belongs to teamA only) and
// an admin. teamB exists but the member does not belong to it.
type catalogInstallFixture struct {
	org    model.Org
	admin  model.User
	member model.User
	teamA  model.Team // member belongs
	teamB  model.Team // member does NOT belong
}

func seedCatalogInstallFixture(t *testing.T, db *gorm.DB) catalogInstallFixture {
	t.Helper()
	org := model.Org{ID: uuid.New(), Name: "cat-" + uuid.NewString()[:8], RateLimit: 1000, Active: true}
	admin := model.User{ID: uuid.New(), Email: "cadmin-" + uuid.NewString()[:8] + "@test.com", Name: "admin"}
	member := model.User{ID: uuid.New(), Email: "cmember-" + uuid.NewString()[:8] + "@test.com", Name: "member"}
	teamA := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "team-a-" + uuid.NewString()[:8]}
	teamB := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "team-b-" + uuid.NewString()[:8]}

	rows := []any{
		&org, &admin, &member,
		&model.OrgMembership{UserID: admin.ID, OrgID: org.ID, Role: "admin"},
		&model.OrgMembership{UserID: member.ID, OrgID: org.ID, Role: "member"},
		&teamA, &teamB,
		&model.TeamMember{OrgID: org.ID, TeamID: teamA.ID, UserID: member.ID, Role: "member"},
	}
	for _, r := range rows {
		if err := db.Create(r).Error; err != nil {
			t.Fatalf("seed catalog install fixture: %v", err)
		}
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.OrgPluginInstall{})
		db.Where("org_id = ?", org.ID).Delete(&model.TeamPlugin{})
		db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		db.Where("org_id = ?", org.ID).Delete(&model.TeamMember{})
		db.Where("org_id = ?", org.ID).Delete(&model.Team{})
		db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		db.Where("id IN ?", []uuid.UUID{admin.ID, member.ID}).Delete(&model.User{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})
	return catalogInstallFixture{org: org, admin: admin, member: member, teamA: teamA, teamB: teamB}
}

func seedInstallCatalog(t *testing.T, db *gorm.DB, required ...string) model.AgentCatalog {
	t.Helper()
	catalog := model.AgentCatalog{
		ID:              uuid.New(),
		Slug:            "install-cat-" + uuid.NewString()[:8],
		Name:            "Install Cat " + uuid.NewString()[:8],
		Model:           agentruntime.DefaultAgentModel,
		SandboxImage:    model.SandboxImageDefault,
		Instructions:    "Catalog live instructions.",
		Manifest:        model.RawJSON(`{}`),
		Status:          model.AgentCatalogStatusActive,
		RequiredPlugins: required,
	}
	if err := db.Create(&catalog).Error; err != nil {
		t.Fatalf("create install catalog: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", catalog.ID).Delete(&model.AgentCatalog{}) })
	return catalog
}

func doInstall(t *testing.T, h *handler.AgentHandler, c caller, org model.Org, slug, teamID string) *httptest.ResponseRecorder {
	t.Helper()
	var body io.Reader
	if teamID != "" {
		body = bytes.NewReader([]byte(`{"team_id":"` + teamID + `"}`))
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/agents/catalog/"+slug+"/install", body)
	req = c.apply(req, org)
	req = withURLParam(req, "slug", slug)
	rr := httptest.NewRecorder()
	h.InstallCatalog(rr, req)
	return rr
}

func TestInstallCatalog_SystemPluginOnly_IntoTeam(t *testing.T) {
	db := connectTestDB(t)
	seedDefaultModelCredential(t, db)
	fx := seedCatalogInstallFixture(t, db)
	catalog := seedInstallCatalog(t, db)
	h := newAgentHandlerForTest(db)

	rr := doInstall(t, h, caller{user: &fx.member}, fx.org, catalog.Slug, fx.teamA.ID.String())
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	var agent model.Agent
	if err := db.Where("org_id = ? AND agent_catalog_id = ?", fx.org.ID, catalog.ID).First(&agent).Error; err != nil {
		t.Fatalf("load clone: %v", err)
	}
	if agent.TeamID != fx.teamA.ID {
		t.Fatalf("clone team_id = %v, want %s", agent.TeamID, fx.teamA.ID)
	}
	if agent.AgentCatalogID == nil || *agent.AgentCatalogID != catalog.ID {
		t.Fatalf("clone agent_catalog_id = %v, want %s", agent.AgentCatalogID, catalog.ID)
	}
	if agent.Instructions != nil {
		t.Fatalf("clone instructions = %q, want nil (auto-sync from catalog)", *agent.Instructions)
	}
}

func TestInstallCatalog_MissingTeamPlugin_422(t *testing.T) {
	db := connectTestDB(t)
	seedDefaultModelCredential(t, db)
	fx := seedCatalogInstallFixture(t, db)

	// An optional (non auto-install) plugin the org has installed but the team
	// has NOT been provisioned: the team cannot use it.
	plugin := model.Plugin{ID: uuid.New(), Slug: "opt-" + uuid.NewString()[:8], Name: "Optional", Status: model.PluginStatusActive}
	if err := db.Create(&plugin).Error; err != nil {
		t.Fatalf("create plugin: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", plugin.ID).Delete(&model.Plugin{}) })
	if err := db.Create(&model.OrgPluginInstall{ID: uuid.New(), OrgID: fx.org.ID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("org install: %v", err)
	}
	catalog := seedInstallCatalog(t, db, plugin.Slug)
	h := newAgentHandlerForTest(db)

	rr := doInstall(t, h, caller{user: &fx.member}, fx.org, catalog.Slug, fx.teamA.ID.String())
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Error          string   `json:"error"`
		MissingPlugins []string `json:"missing_plugins"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error != "your team is missing required plugins" {
		t.Fatalf("error = %q", resp.Error)
	}
	if len(resp.MissingPlugins) != 1 || resp.MissingPlugins[0] != plugin.Slug {
		t.Fatalf("missing_plugins = %v, want [%s]", resp.MissingPlugins, plugin.Slug)
	}
	var count int64
	db.Model(&model.Agent{}).Where("org_id = ? AND agent_catalog_id = ?", fx.org.ID, catalog.ID).Count(&count)
	if count != 0 {
		t.Fatalf("agent created despite missing plugin: count=%d", count)
	}
}

func TestInstallCatalog_TeamProvisionedPlugin_EnablesOnClone(t *testing.T) {
	db := connectTestDB(t)
	seedDefaultModelCredential(t, db)
	fx := seedCatalogInstallFixture(t, db)

	plugin := model.Plugin{ID: uuid.New(), Slug: "prov-" + uuid.NewString()[:8], Name: "Provisioned", Status: model.PluginStatusActive}
	if err := db.Create(&plugin).Error; err != nil {
		t.Fatalf("create plugin: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", plugin.ID).Delete(&model.Plugin{}) })
	if err := db.Create(&model.OrgPluginInstall{ID: uuid.New(), OrgID: fx.org.ID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("org install: %v", err)
	}
	if err := db.Create(&model.TeamPlugin{ID: uuid.New(), OrgID: fx.org.ID, TeamID: fx.teamA.ID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("team provision: %v", err)
	}
	catalog := seedInstallCatalog(t, db, plugin.Slug)
	h := newAgentHandlerForTest(db)

	rr := doInstall(t, h, caller{user: &fx.member}, fx.org, catalog.Slug, fx.teamA.ID.String())
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	var agent model.Agent
	if err := db.Where("org_id = ? AND agent_catalog_id = ?", fx.org.ID, catalog.ID).First(&agent).Error; err != nil {
		t.Fatalf("load clone: %v", err)
	}
	assertAgentEffectivePlugin(t, db, agent.ID, plugin.ID)
}

func TestInstallCatalog_TwoTeams_TwoClones(t *testing.T) {
	db := connectTestDB(t)
	seedDefaultModelCredential(t, db)
	fx := seedCatalogInstallFixture(t, db)
	catalog := seedInstallCatalog(t, db)
	h := newAgentHandlerForTest(db)

	// Admin can install into any team.
	admin := caller{user: &fx.admin}
	if rr := doInstall(t, h, admin, fx.org, catalog.Slug, fx.teamA.ID.String()); rr.Code != http.StatusCreated {
		t.Fatalf("teamA status = %d body = %s", rr.Code, rr.Body.String())
	}
	if rr := doInstall(t, h, admin, fx.org, catalog.Slug, fx.teamB.ID.String()); rr.Code != http.StatusCreated {
		t.Fatalf("teamB status = %d body = %s", rr.Code, rr.Body.String())
	}
	var count int64
	db.Model(&model.Agent{}).
		Where("org_id = ? AND agent_catalog_id = ? AND status <> ?", fx.org.ID, catalog.ID, "archived").
		Count(&count)
	if count != 2 {
		t.Fatalf("clone count = %d, want 2 (one per team)", count)
	}

	// Re-installing into teamA is idempotent: still 2 clones.
	if rr := doInstall(t, h, admin, fx.org, catalog.Slug, fx.teamA.ID.String()); rr.Code != http.StatusCreated {
		t.Fatalf("re-install status = %d body = %s", rr.Code, rr.Body.String())
	}
	db.Model(&model.Agent{}).
		Where("org_id = ? AND agent_catalog_id = ? AND status <> ?", fx.org.ID, catalog.ID, "archived").
		Count(&count)
	if count != 2 {
		t.Fatalf("clone count after idempotent re-install = %d, want 2", count)
	}
}

func TestInstallCatalog_NonMemberOfTeam_403(t *testing.T) {
	db := connectTestDB(t)
	seedDefaultModelCredential(t, db)
	fx := seedCatalogInstallFixture(t, db)
	catalog := seedInstallCatalog(t, db)
	h := newAgentHandlerForTest(db)

	// The member does not belong to teamB.
	rr := doInstall(t, h, caller{user: &fx.member}, fx.org, catalog.Slug, fx.teamB.ID.String())
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	var count int64
	db.Model(&model.Agent{}).Where("org_id = ? AND agent_catalog_id = ?", fx.org.ID, catalog.ID).Count(&count)
	if count != 0 {
		t.Fatalf("agent created for unauthorized team: count=%d", count)
	}
}

func TestUninstallCatalog_DeletesOnlyThatTeamsClone(t *testing.T) {
	db := connectTestDB(t)
	seedDefaultModelCredential(t, db)
	fx := seedCatalogInstallFixture(t, db)
	catalog := seedInstallCatalog(t, db)
	h := newAgentHandlerForTest(db)
	admin := caller{user: &fx.admin}

	if rr := doInstall(t, h, admin, fx.org, catalog.Slug, fx.teamA.ID.String()); rr.Code != http.StatusCreated {
		t.Fatalf("teamA install status = %d body = %s", rr.Code, rr.Body.String())
	}
	if rr := doInstall(t, h, admin, fx.org, catalog.Slug, fx.teamB.ID.String()); rr.Code != http.StatusCreated {
		t.Fatalf("teamB install status = %d body = %s", rr.Code, rr.Body.String())
	}

	// Uninstall teamA's clone via query param.
	req := httptest.NewRequest(http.MethodDelete, "/v1/agents/catalog/"+catalog.Slug+"/install?team_id="+fx.teamA.ID.String(), nil)
	req = admin.apply(req, fx.org)
	req = withURLParam(req, "slug", catalog.Slug)
	rr := httptest.NewRecorder()
	h.UninstallCatalog(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("uninstall status = %d body = %s", rr.Code, rr.Body.String())
	}

	var teamACount, teamBCount int64
	db.Model(&model.Agent{}).
		Where("org_id = ? AND agent_catalog_id = ? AND team_id = ? AND status <> ?", fx.org.ID, catalog.ID, fx.teamA.ID, "archived").
		Count(&teamACount)
	db.Model(&model.Agent{}).
		Where("org_id = ? AND agent_catalog_id = ? AND team_id = ? AND status <> ?", fx.org.ID, catalog.ID, fx.teamB.ID, "archived").
		Count(&teamBCount)
	if teamACount != 0 {
		t.Fatalf("teamA clone still active after uninstall: count=%d", teamACount)
	}
	if teamBCount != 1 {
		t.Fatalf("teamB clone should remain: count=%d", teamBCount)
	}
}
