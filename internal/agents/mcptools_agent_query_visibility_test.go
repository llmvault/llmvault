package agents

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// agentVisibilityScenario seeds an org with multiple teams and a spread of
// agents so list_agents/get_agent can be pinned to the calling Hivy agent's
// team, independently of the human actor's org role.
type agentVisibilityScenario struct {
	org      model.Org
	admin    model.User
	member   model.User
	teamA    model.Team
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
		db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		db.Where("org_id = ?", org.ID).Delete(&model.Team{})
		db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		db.Where("id IN ?", []uuid.UUID{admin.ID, member.ID}).Delete(&model.User{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})
	return agentVisibilityScenario{
		org: org, admin: admin, member: member,
		teamA:   teamA,
		inTeamA: inTeamA, inTeamB: inTeamB, teamNull: teamNull,
	}
}

func TestListAgents_CallingAgentTeamScopedVisibility(t *testing.T) {
	db := testDB(t)
	sc := seedAgentVisibility(t, db)
	token := &model.Token{OrgID: sc.org.ID}
	ctx := context.Background()

	text := listAgentsText(t, ctx, db, token, sc.teamA.ID)
	if !strings.Contains(text, sc.inTeamA.ID.String()) {
		t.Fatalf("team list should include %s (%s), got: %s", sc.inTeamA.Name, sc.inTeamA.ID, text)
	}
	for _, hidden := range []model.Agent{sc.inTeamB, sc.teamNull} {
		if strings.Contains(text, hidden.ID.String()) {
			t.Fatalf("team list must NOT include %s (%s), got: %s", hidden.Name, hidden.ID, text)
		}
	}
}

func TestGetAgent_CallingAgentTeamScopedVisibility(t *testing.T) {
	db := testDB(t)
	sc := seedAgentVisibility(t, db)
	token := &model.Token{OrgID: sc.org.ID}
	ctx := context.Background()

	if res, _ := handleGetAgent(ctx, db, token, sc.teamA.ID, "", getAgentArgs{AgentID: sc.inTeamA.ID.String()}); res.IsError {
		t.Fatalf("get of same-team agent errored: %s", errResultText(res))
	}
	for _, hidden := range []model.Agent{sc.inTeamB, sc.teamNull} {
		res, _ := handleGetAgent(ctx, db, token, sc.teamA.ID, "", getAgentArgs{AgentID: hidden.ID.String()})
		if !res.IsError || !strings.Contains(errResultText(res), "not found") {
			t.Fatalf("cross-team get of %s should be not found, got: %s", hidden.Name, errResultText(res))
		}
	}
}

func listAgentsText(t *testing.T, ctx context.Context, db *gorm.DB, token *model.Token, teamID uuid.UUID) string {
	t.Helper()
	res, _ := handleListAgents(ctx, db, token, teamID)
	if res.IsError {
		t.Fatalf("list_agents error: %s", errResultText(res))
	}
	return errResultText(res)
}
