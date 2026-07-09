package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestInstallCatalog_MemberOfTwoTeams_OnlyEligibleTeamInstalls(t *testing.T) {
	db := connectTestDB(t)
	seedDefaultModelCredential(t, db)
	fx := seedCatalogInstallFixture(t, db)

	if err := db.Create(&model.TeamMember{OrgID: fx.org.ID, TeamID: fx.teamB.ID, UserID: fx.member.ID, Role: "member"}).Error; err != nil {
		t.Fatalf("add member to teamB: %v", err)
	}
	plugin := model.Plugin{ID: uuid.New(), Slug: "req-" + uuid.NewString()[:8], Name: "Required", Status: model.PluginStatusActive}
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
	member := caller{user: &fx.member}

	if rr := doInstall(t, h, member, fx.org, catalog.Slug, fx.teamA.ID.String()); rr.Code != http.StatusCreated {
		t.Fatalf("eligible teamA install status = %d body = %s", rr.Code, rr.Body.String())
	}

	rr := doInstall(t, h, member, fx.org, catalog.Slug, fx.teamB.ID.String())
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("ineligible teamB status = %d body = %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		MissingPlugins []string `json:"missing_plugins"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.MissingPlugins) != 1 || resp.MissingPlugins[0] != plugin.Slug {
		t.Fatalf("missing_plugins = %v, want [%s]", resp.MissingPlugins, plugin.Slug)
	}

	var teamAClones, teamBClones int64
	db.Model(&model.Agent{}).Where("org_id = ? AND agent_catalog_id = ? AND team_id = ?", fx.org.ID, catalog.ID, fx.teamA.ID).Count(&teamAClones)
	db.Model(&model.Agent{}).Where("org_id = ? AND agent_catalog_id = ? AND team_id = ?", fx.org.ID, catalog.ID, fx.teamB.ID).Count(&teamBClones)
	if teamAClones != 1 {
		t.Fatalf("teamA clone count = %d, want 1", teamAClones)
	}
	if teamBClones != 0 {
		t.Fatalf("teamB clone count = %d, want 0", teamBClones)
	}
}
