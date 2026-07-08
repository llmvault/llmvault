package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestIntegration_TeamsListIncludesScopedChannels(t *testing.T) {
	h := newTeamHarness(t)
	org := createTestOrg(t, h.db)
	owner := seedSessionUser(t, h.db, org.ID, "owner")
	member := seedSessionUser(t, h.db, org.ID, "member")
	agent := seedSessionAgent(t, h.db, org.ID)
	t.Cleanup(func() {
		h.db.Where("channel_id IN (?)", h.db.Model(&model.Channel{}).Select("id").Where("org_id = ?", org.ID)).Delete(&model.ChannelMember{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.Channel{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.TeamMember{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.Team{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		h.db.Where("id IN ?", []uuid.UUID{owner.ID, member.ID}).Delete(&model.User{})
	})

	teamA := model.Team{OrgID: org.ID, Name: "Team A", CreatedBy: &owner.ID}
	teamB := model.Team{OrgID: org.ID, Name: "Team B", CreatedBy: &owner.ID}
	for _, tm := range []*model.Team{&teamA, &teamB} {
		if err := h.db.Create(tm).Error; err != nil {
			t.Fatalf("create team: %v", err)
		}
	}
	if err := h.db.Create(&model.TeamMember{OrgID: org.ID, TeamID: teamA.ID, UserID: member.ID, Role: "member"}).Error; err != nil {
		t.Fatalf("add member to teamA: %v", err)
	}

	newChannel := func(name, visibility string, teamID uuid.UUID) model.Channel {
		c := model.Channel{
			OrgID:          org.ID,
			Name:           name,
			Kind:           "standard",
			Visibility:     visibility,
			TeamID:         &teamID,
			DefaultAgentID: agent.ID,
			Origin:         "native",
			CreatedBy:      &owner.ID,
		}
		if err := h.db.Create(&c).Error; err != nil {
			t.Fatalf("create channel %s: %v", name, err)
		}
		return c
	}
	pubA := newChannel("public-a", "public", teamA.ID)
	privA := newChannel("private-a", "private", teamA.ID)
	pubB := newChannel("public-b", "public", teamB.ID)

	channelsByTeam := func(rr *httptest.ResponseRecorder) map[string]map[string]bool {
		t.Helper()
		if rr.Code != http.StatusOK {
			t.Fatalf("list status=%d body=%s", rr.Code, rr.Body.String())
		}
		var out struct {
			Data []struct {
				ID       string `json:"id"`
				Channels []struct {
					ID string `json:"id"`
				} `json:"channels"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode list: %v\n%s", err, rr.Body.String())
		}
		result := map[string]map[string]bool{}
		for _, team := range out.Data {
			ids := map[string]bool{}
			for _, c := range team.Channels {
				ids[c.ID] = true
			}
			result[team.ID] = ids
		}
		return result
	}

	ownerView := channelsByTeam(h.doJSON(t, http.MethodGet, "/v1/orgs/current/teams", org, owner, nil))
	if !ownerView[teamA.ID.String()][pubA.ID.String()] || !ownerView[teamA.ID.String()][privA.ID.String()] {
		t.Fatalf("owner should see both teamA channels, got %v", ownerView[teamA.ID.String()])
	}
	if !ownerView[teamB.ID.String()][pubB.ID.String()] {
		t.Fatalf("owner should see teamB channel, got %v", ownerView[teamB.ID.String()])
	}

	memberView := channelsByTeam(h.doJSON(t, http.MethodGet, "/v1/orgs/current/teams", org, member, nil))
	if _, ok := memberView[teamB.ID.String()]; ok {
		t.Fatalf("member should not see teamB, got teams %v", memberView)
	}
	teamAChannels := memberView[teamA.ID.String()]
	if !teamAChannels[pubA.ID.String()] {
		t.Fatalf("member should see teamA public channel, got %v", teamAChannels)
	}
	if teamAChannels[privA.ID.String()] {
		t.Fatalf("member must not see teamA private channel they are not a member of, got %v", teamAChannels)
	}
}
