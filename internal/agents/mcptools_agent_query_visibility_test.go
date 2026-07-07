package agents

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// agentVisibilityScenario seeds an org with two users (an admin and a plain
// member), two teams (the member belongs to only one), and a spread of agents
// assigned to channels with different visibility so the actor-scoping matrix can
// be asserted against list_agents / get_agent.
type agentVisibilityScenario struct {
	org         model.Org
	admin       model.User
	member      model.User
	base        model.Agent // channel default; unassigned, so hidden from the member
	visibleTeam model.Agent // assigned to a channel scoped to the member's team
	otherTeam   model.Agent // assigned to a channel scoped to a team the member is not in
	teamNull    model.Agent // assigned to a channel with no team scope
	external    model.Agent // assigned to an externally-sourced channel
	unassigned  model.Agent // assigned to no channel at all
}

func seedAgentVisibility(t *testing.T, db *gorm.DB) agentVisibilityScenario {
	t.Helper()
	org := model.Org{ID: uuid.New(), Name: "vis-" + uuid.NewString()[:8], RateLimit: 1000, Active: true}
	admin := model.User{ID: uuid.New(), Email: "admin-" + uuid.NewString()[:8] + "@test.com", Name: "admin"}
	member := model.User{ID: uuid.New(), Email: "member-" + uuid.NewString()[:8] + "@test.com", Name: "member"}

	mkAgent := func(name string) model.Agent {
		return model.Agent{ID: uuid.New(), OrgID: &org.ID, Name: name, Model: "test", Status: "active"}
	}
	base := mkAgent("Base")
	visibleTeam := mkAgent("VisibleTeam")
	otherTeam := mkAgent("OtherTeam")
	teamNull := mkAgent("TeamNull")
	external := mkAgent("External")
	unassigned := mkAgent("Unassigned")

	teamA := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "team-a-" + uuid.NewString()[:8]}
	teamB := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "team-b-" + uuid.NewString()[:8]}

	chTeam := model.Channel{ID: uuid.New(), OrgID: org.ID, Name: "team-ch-" + uuid.NewString()[:8], Kind: "standard", TeamID: &teamA.ID, DefaultAgentID: base.ID}
	chOther := model.Channel{ID: uuid.New(), OrgID: org.ID, Name: "other-ch-" + uuid.NewString()[:8], Kind: "standard", TeamID: &teamB.ID, DefaultAgentID: base.ID}
	chNull := model.Channel{ID: uuid.New(), OrgID: org.ID, Name: "null-ch-" + uuid.NewString()[:8], Kind: "standard", DefaultAgentID: base.ID}
	chExt := model.Channel{ID: uuid.New(), OrgID: org.ID, Name: "ext-ch-" + uuid.NewString()[:8], Kind: "standard", Origin: "external", TeamID: &teamB.ID, DefaultAgentID: base.ID}

	rows := []any{
		&org, &admin, &member,
		&model.OrgMembership{UserID: admin.ID, OrgID: org.ID, Role: "admin"},
		&model.OrgMembership{UserID: member.ID, OrgID: org.ID, Role: "member"},
		&base, &visibleTeam, &otherTeam, &teamNull, &external, &unassigned,
		&teamA, &teamB,
		&model.TeamMember{OrgID: org.ID, TeamID: teamA.ID, UserID: member.ID, Role: "member"},
		&chTeam, &chOther, &chNull, &chExt,
		&model.ChannelAgent{OrgID: org.ID, ChannelID: chTeam.ID, AgentID: visibleTeam.ID},
		&model.ChannelAgent{OrgID: org.ID, ChannelID: chOther.ID, AgentID: otherTeam.ID},
		&model.ChannelAgent{OrgID: org.ID, ChannelID: chNull.ID, AgentID: teamNull.ID},
		&model.ChannelAgent{OrgID: org.ID, ChannelID: chExt.ID, AgentID: external.ID},
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
		db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		db.Where("id IN ?", []uuid.UUID{admin.ID, member.ID}).Delete(&model.User{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})
	return agentVisibilityScenario{
		org: org, admin: admin, member: member,
		base: base, visibleTeam: visibleTeam, otherTeam: otherTeam,
		teamNull: teamNull, external: external, unassigned: unassigned,
	}
}

func TestListAgents_ActorScopedVisibility(t *testing.T) {
	db := testDB(t)
	sc := seedAgentVisibility(t, db)
	token := &model.Token{OrgID: sc.org.ID}
	ctx := context.Background()

	// A plain member sees only agents assigned to channels they can use:
	// their team's channel, the team-less channel, and the external channel.
	memberText := listAgentsText(t, ctx, db, token, sc.member.ID.String())
	for _, want := range []model.Agent{sc.visibleTeam, sc.teamNull, sc.external} {
		if !strings.Contains(memberText, want.ID.String()) {
			t.Fatalf("member list should include %s (%s), got: %s", want.Name, want.ID, memberText)
		}
	}
	for _, hidden := range []model.Agent{sc.otherTeam, sc.unassigned, sc.base} {
		if strings.Contains(memberText, hidden.ID.String()) {
			t.Fatalf("member list must NOT include %s (%s), got: %s", hidden.Name, hidden.ID, memberText)
		}
	}

	// An org manager sees every agent in the org.
	adminText := listAgentsText(t, ctx, db, token, sc.admin.ID.String())
	// No human actor (automated run) also keeps the org-wide view.
	noActorText := listAgentsText(t, ctx, db, token, "")
	for _, all := range []model.Agent{sc.base, sc.visibleTeam, sc.otherTeam, sc.teamNull, sc.external, sc.unassigned} {
		if !strings.Contains(adminText, all.ID.String()) {
			t.Fatalf("admin list should include %s (%s)", all.Name, all.ID)
		}
		if !strings.Contains(noActorText, all.ID.String()) {
			t.Fatalf("no-actor list should include %s (%s)", all.Name, all.ID)
		}
	}
}

func TestGetAgent_ActorScopedVisibility(t *testing.T) {
	db := testDB(t)
	sc := seedAgentVisibility(t, db)
	token := &model.Token{OrgID: sc.org.ID}
	ctx := context.Background()

	// The member can inspect a visible agent.
	if res, _ := handleGetAgent(ctx, db, token, "", sc.member.ID.String(), getAgentArgs{AgentID: sc.visibleTeam.ID.String()}); res.IsError {
		t.Fatalf("member get of visible agent errored: %s", errResultText(res))
	}
	// A hidden agent is reported as not found (never as forbidden) so its
	// existence does not leak.
	for _, hidden := range []model.Agent{sc.otherTeam, sc.unassigned} {
		res, _ := handleGetAgent(ctx, db, token, "", sc.member.ID.String(), getAgentArgs{AgentID: hidden.ID.String()})
		if !res.IsError || !strings.Contains(errResultText(res), "not found") {
			t.Fatalf("member get of hidden %s should be not found, got: %s", hidden.Name, errResultText(res))
		}
	}
	// A manager can inspect any agent, including team-scoped ones.
	if res, _ := handleGetAgent(ctx, db, token, "", sc.admin.ID.String(), getAgentArgs{AgentID: sc.otherTeam.ID.String()}); res.IsError {
		t.Fatalf("admin get of any agent errored: %s", errResultText(res))
	}
	// No actor keeps org-wide access.
	if res, _ := handleGetAgent(ctx, db, token, "", "", getAgentArgs{AgentID: sc.unassigned.ID.String()}); res.IsError {
		t.Fatalf("no-actor get of unassigned agent errored: %s", errResultText(res))
	}
}

func listAgentsText(t *testing.T, ctx context.Context, db *gorm.DB, token *model.Token, actorRaw string) string {
	t.Helper()
	res, _ := handleListAgents(ctx, db, token, actorRaw)
	if res.IsError {
		t.Fatalf("list_agents error: %s", errResultText(res))
	}
	return errResultText(res)
}
