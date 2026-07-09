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
// owned by different teams so the actor-scoping matrix can be asserted against
// list_agents / get_agent. Under the team-primary model an agent is a team
// member, so a user sees exactly the agents belonging to their teams.
type agentVisibilityScenario struct {
	org      model.Org
	admin    model.User
	member   model.User
	inTeamA  model.Agent // owned by the member's team; visible to the member
	inTeamB  model.Agent // owned by a team the member is not in; hidden
	teamNull model.Agent // owned by a third team the member is not in; hidden
}

func seedAgentVisibility(t *testing.T, db *gorm.DB) agentVisibilityScenario {
	t.Helper()
	org := model.Org{ID: uuid.New(), Name: "vis-" + uuid.NewString()[:8], RateLimit: 1000, Active: true}
	admin := model.User{ID: uuid.New(), Email: "admin-" + uuid.NewString()[:8] + "@test.com", Name: "admin"}
	member := model.User{ID: uuid.New(), Email: "member-" + uuid.NewString()[:8] + "@test.com", Name: "member"}

	teamA := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "team-a-" + uuid.NewString()[:8]}
	teamB := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "team-b-" + uuid.NewString()[:8]}
	teamC := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "team-c-" + uuid.NewString()[:8]}

	mkAgent := func(name string, team uuid.UUID) model.Agent {
		return model.Agent{ID: uuid.New(), OrgID: &org.ID, Name: name, Model: "test", Status: "active", TeamID: team}
	}
	inTeamA := mkAgent("InTeamA", teamA.ID)
	inTeamB := mkAgent("InTeamB", teamB.ID)
	teamNull := mkAgent("TeamNull", teamC.ID)

	rows := []any{
		&org, &admin, &member,
		&model.OrgMembership{UserID: admin.ID, OrgID: org.ID, Role: "admin"},
		&model.OrgMembership{UserID: member.ID, OrgID: org.ID, Role: "member"},
		&teamA, &teamB, &teamC,
		&inTeamA, &inTeamB, &teamNull,
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
	return agentVisibilityScenario{
		org: org, admin: admin, member: member,
		inTeamA: inTeamA, inTeamB: inTeamB, teamNull: teamNull,
	}
}

func TestListAgents_ActorScopedVisibility(t *testing.T) {
	db := testDB(t)
	sc := seedAgentVisibility(t, db)
	token := &model.Token{OrgID: sc.org.ID}
	ctx := context.Background()

	// A plain member sees only agents owned by teams they belong to.
	memberText := listAgentsText(t, ctx, db, token, sc.member.ID.String())
	if !strings.Contains(memberText, sc.inTeamA.ID.String()) {
		t.Fatalf("member list should include %s (%s), got: %s", sc.inTeamA.Name, sc.inTeamA.ID, memberText)
	}
	for _, hidden := range []model.Agent{sc.inTeamB, sc.teamNull} {
		if strings.Contains(memberText, hidden.ID.String()) {
			t.Fatalf("member list must NOT include %s (%s), got: %s", hidden.Name, hidden.ID, memberText)
		}
	}

	// An org manager sees every agent in the org.
	adminText := listAgentsText(t, ctx, db, token, sc.admin.ID.String())
	// No human actor (automated run) also keeps the org-wide view.
	noActorText := listAgentsText(t, ctx, db, token, "")
	for _, all := range []model.Agent{sc.inTeamA, sc.inTeamB, sc.teamNull} {
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

	// The member can inspect an agent owned by their team.
	if res, _ := handleGetAgent(ctx, db, token, "", sc.member.ID.String(), getAgentArgs{AgentID: sc.inTeamA.ID.String()}); res.IsError {
		t.Fatalf("member get of visible agent errored: %s", errResultText(res))
	}
	// A hidden agent is reported as not found (never as forbidden) so its
	// existence does not leak.
	for _, hidden := range []model.Agent{sc.inTeamB, sc.teamNull} {
		res, _ := handleGetAgent(ctx, db, token, "", sc.member.ID.String(), getAgentArgs{AgentID: hidden.ID.String()})
		if !res.IsError || !strings.Contains(errResultText(res), "not found") {
			t.Fatalf("member get of hidden %s should be not found, got: %s", hidden.Name, errResultText(res))
		}
	}
	// A manager can inspect any agent, including other teams' agents.
	if res, _ := handleGetAgent(ctx, db, token, "", sc.admin.ID.String(), getAgentArgs{AgentID: sc.inTeamB.ID.String()}); res.IsError {
		t.Fatalf("admin get of any agent errored: %s", errResultText(res))
	}
	// No actor keeps org-wide access.
	if res, _ := handleGetAgent(ctx, db, token, "", "", getAgentArgs{AgentID: sc.teamNull.ID.String()}); res.IsError {
		t.Fatalf("no-actor get of third-team agent errored: %s", errResultText(res))
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
