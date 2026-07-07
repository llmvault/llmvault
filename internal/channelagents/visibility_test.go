package channelagents_test

import (
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/channelagents"
	"github.com/usehivy/hivy/internal/model"
)

// visibleIDs runs VisibleAgentIDsSubquery and materializes the agent ids it
// yields (the subquery is normally embedded in `agents.id IN (?)`).
func visibleIDs(t *testing.T, db *gorm.DB, orgID uuid.UUID, userID *uuid.UUID) map[uuid.UUID]bool {
	t.Helper()
	var ids []uuid.UUID
	if err := db.Table("(?) AS vis", channelagents.VisibleAgentIDsSubquery(db, orgID, userID)).
		Pluck("agent_id", &ids).Error; err != nil {
		t.Fatalf("run subquery: %v", err)
	}
	out := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

func TestVisibleAgentIDsSubquery(t *testing.T) {
	db := connect(t)

	org := model.Org{ID: uuid.New(), Name: "vis-" + uuid.NewString()[:8], Active: true, RateLimit: 1000}
	member := model.User{ID: uuid.New(), Email: "m-" + uuid.NewString()[:8] + "@t.com", Name: "m"}

	mkAgent := func(name string) model.Agent {
		return model.Agent{ID: uuid.New(), OrgID: &org.ID, Name: name, Model: "test", Status: "active"}
	}
	base := mkAgent("Base")
	visibleTeam := mkAgent("VisibleTeam")
	otherTeam := mkAgent("OtherTeam")
	teamNull := mkAgent("TeamNull")
	external := mkAgent("External")
	systemAssigned := mkAgent("SystemAssigned")

	teamA := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "a-" + uuid.NewString()[:8]}
	teamB := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "b-" + uuid.NewString()[:8]}

	chTeam := model.Channel{ID: uuid.New(), OrgID: org.ID, Name: "team-" + uuid.NewString()[:8], Kind: "standard", TeamID: &teamA.ID, DefaultAgentID: base.ID}
	chOther := model.Channel{ID: uuid.New(), OrgID: org.ID, Name: "other-" + uuid.NewString()[:8], Kind: "standard", TeamID: &teamB.ID, DefaultAgentID: base.ID}
	chNull := model.Channel{ID: uuid.New(), OrgID: org.ID, Name: "null-" + uuid.NewString()[:8], Kind: "standard", DefaultAgentID: base.ID}
	chExt := model.Channel{ID: uuid.New(), OrgID: org.ID, Name: "ext-" + uuid.NewString()[:8], Kind: "standard", Origin: "external", TeamID: &teamB.ID, DefaultAgentID: base.ID}
	// A system channel is exempt from assignment and must never grant extra
	// visibility even if a stray row exists (team_id NULL would otherwise open
	// it to everyone).
	chSystem := model.Channel{ID: uuid.New(), OrgID: org.ID, Name: "sys-" + uuid.NewString()[:8], Kind: "system", DefaultAgentID: base.ID}

	rows := []any{
		&org, &member,
		&base, &visibleTeam, &otherTeam, &teamNull, &external, &systemAssigned,
		&teamA, &teamB,
		&model.TeamMember{OrgID: org.ID, TeamID: teamA.ID, UserID: member.ID, Role: "member"},
		&chTeam, &chOther, &chNull, &chExt, &chSystem,
		&model.ChannelAgent{OrgID: org.ID, ChannelID: chTeam.ID, AgentID: visibleTeam.ID},
		&model.ChannelAgent{OrgID: org.ID, ChannelID: chOther.ID, AgentID: otherTeam.ID},
		&model.ChannelAgent{OrgID: org.ID, ChannelID: chNull.ID, AgentID: teamNull.ID},
		&model.ChannelAgent{OrgID: org.ID, ChannelID: chExt.ID, AgentID: external.ID},
		&model.ChannelAgent{OrgID: org.ID, ChannelID: chSystem.ID, AgentID: systemAssigned.ID},
	}
	for _, r := range rows {
		if err := db.Create(r).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.ChannelAgent{})
		db.Where("org_id = ?", org.ID).Delete(&model.Channel{})
		db.Where("org_id = ?", org.ID).Delete(&model.TeamMember{})
		db.Where("org_id = ?", org.ID).Delete(&model.Team{})
		db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		db.Where("id = ?", member.ID).Delete(&model.User{})
		db.Delete(&model.Org{}, "id = ?", org.ID)
	})

	// A member of teamA: sees agents in their team channel, the team-less
	// channel, and the external channel. Not the other team's channel, and not
	// the system-channel assignment (excluded despite team_id NULL).
	got := visibleIDs(t, db, org.ID, &member.ID)
	for _, want := range []model.Agent{visibleTeam, teamNull, external} {
		if !got[want.ID] {
			t.Fatalf("member should see %s (%s); got %v", want.Name, want.ID, got)
		}
	}
	for _, hidden := range []model.Agent{otherTeam, systemAssigned, base} {
		if got[hidden.ID] {
			t.Fatalf("member must NOT see %s (%s); got %v", hidden.Name, hidden.ID, got)
		}
	}

	// A nil user (no membership) only sees external and team-less assignments.
	gotNil := visibleIDs(t, db, org.ID, nil)
	for _, want := range []model.Agent{teamNull, external} {
		if !gotNil[want.ID] {
			t.Fatalf("nil user should see %s via external/team-null; got %v", want.Name, gotNil)
		}
	}
	for _, hidden := range []model.Agent{visibleTeam, otherTeam, systemAssigned} {
		if gotNil[hidden.ID] {
			t.Fatalf("nil user must NOT see team-scoped/system %s; got %v", hidden.Name, gotNil)
		}
	}
}
