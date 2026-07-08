package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// The org's last non-archived team cannot be archived; a non-last team can.
func TestIntegration_TeamsArchiveKeepsLastTeam(t *testing.T) {
	h := newTeamHarness(t)
	org := createTestOrg(t, h.db)
	owner := seedSessionUser(t, h.db, org.ID, "owner")
	t.Cleanup(func() {
		h.db.Where("org_id = ?", org.ID).Delete(&model.TeamMember{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.Team{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		h.db.Where("id = ?", owner.ID).Delete(&model.User{})
	})
	teamA := model.Team{OrgID: org.ID, Name: "Alpha", CreatedBy: &owner.ID}
	teamB := model.Team{OrgID: org.ID, Name: "Beta", CreatedBy: &owner.ID}
	for _, tm := range []*model.Team{&teamA, &teamB} {
		if err := h.db.Create(tm).Error; err != nil {
			t.Fatalf("create team: %v", err)
		}
	}
	// Archiving a non-last team succeeds (both are channel-free).
	first := h.doJSON(t, http.MethodDelete, "/v1/orgs/current/teams/"+teamA.ID.String(), org, owner, nil)
	if first.Code != http.StatusOK {
		t.Fatalf("archive non-last team status=%d body=%s", first.Code, first.Body.String())
	}
	// Archiving the now-last team is rejected.
	last := h.doJSON(t, http.MethodDelete, "/v1/orgs/current/teams/"+teamB.ID.String(), org, owner, nil)
	if last.Code != http.StatusConflict {
		t.Fatalf("archive last team status=%d body=%s", last.Code, last.Body.String())
	}
}

// Creating a team auto-provisions its own default Hivy (IsDefault, team-scoped)
// and its #general channel (team-scoped, default agent = that Hivy). Two teams
// yield two independent #general + Hivy clones with no unique-constraint clash.
func TestIntegration_TeamsCreateProvisionsHivyAndGeneral(t *testing.T) {
	h := newTeamHarness(t)
	org := createTestOrg(t, h.db)
	owner := seedSessionUser(t, h.db, org.ID, "owner")
	t.Cleanup(func() {
		h.db.Exec("DELETE FROM channel_members WHERE channel_id IN (SELECT id FROM channels WHERE org_id = ?)", org.ID)
		h.db.Where("org_id = ?", org.ID).Delete(&model.Channel{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.TeamMember{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.Team{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		h.db.Where("id = ?", owner.ID).Delete(&model.User{})
	})

	create := h.doJSON(t, http.MethodPost, "/v1/orgs/current/teams", org, owner, map[string]any{"name": "Growth"})
	if create.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	var created struct {
		Team struct {
			ID string `json:"id"`
		} `json:"team"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	teamID := uuid.MustParse(created.Team.ID)

	var hivy model.Agent
	if err := h.db.Where("org_id = ? AND team_id = ? AND is_default = true", org.ID, teamID).First(&hivy).Error; err != nil {
		t.Fatalf("load team Hivy: %v", err)
	}
	if hivy.Name != "Hivy" {
		t.Fatalf("team Hivy name = %q, want %q", hivy.Name, "Hivy")
	}
	var general model.Channel
	if err := h.db.Where("org_id = ? AND team_id = ? AND name = ?", org.ID, teamID, "general").First(&general).Error; err != nil {
		t.Fatalf("load team #general: %v", err)
	}
	if !general.IsDefault || general.DefaultAgentID != hivy.ID {
		t.Fatalf("#general = %#v, want default agent %s", general, hivy.ID)
	}

	create2 := h.doJSON(t, http.MethodPost, "/v1/orgs/current/teams", org, owner, map[string]any{"name": "Growth Two"})
	if create2.Code != http.StatusCreated {
		t.Fatalf("second create status=%d body=%s", create2.Code, create2.Body.String())
	}
	var created2 struct {
		Team struct {
			ID string `json:"id"`
		} `json:"team"`
	}
	if err := json.Unmarshal(create2.Body.Bytes(), &created2); err != nil {
		t.Fatalf("decode second create: %v", err)
	}
	teamID2 := uuid.MustParse(created2.Team.ID)
	var hivy2 model.Agent
	if err := h.db.Where("org_id = ? AND team_id = ? AND is_default = true", org.ID, teamID2).First(&hivy2).Error; err != nil {
		t.Fatalf("load second team Hivy: %v", err)
	}
	if hivy2.Name != "Hivy" {
		t.Fatalf("second team Hivy name = %q, want %q (no suffix)", hivy2.Name, "Hivy")
	}
	var hivyCount int64
	if err := h.db.Model(&model.Agent{}).Where("org_id = ? AND name = ? AND is_default = true", org.ID, "Hivy").Count(&hivyCount).Error; err != nil {
		t.Fatalf("count Hivy agents: %v", err)
	}
	if hivyCount != 2 {
		t.Fatalf(`agents named "Hivy" = %d, want 2`, hivyCount)
	}
	var generalCount int64
	if err := h.db.Model(&model.Channel{}).Where("org_id = ? AND name = ?", org.ID, "general").Count(&generalCount).Error; err != nil {
		t.Fatalf("count generals: %v", err)
	}
	if generalCount != 2 {
		t.Fatalf("#general count = %d, want 2", generalCount)
	}
}
