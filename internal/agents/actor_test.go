package agents

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/model"
)

// TestRequireTeamManager verifies the agent-builder tools gate on the acting
// human's ability to manage the agent's owning team: org managers always pass,
// active team members pass for their own team, non-members are blocked, an
// automated run with no actor fails closed, and a nil/zero team is manager-only.
func TestRequireTeamManager(t *testing.T) {
	db := testDB(t)
	org := testOrg(t, db)
	team := testTeam(t, db, org.ID)
	otherTeam := testTeam(t, db, org.ID)

	owner := model.User{ID: uuid.New(), Email: "owner-" + uuid.NewString() + "@example.test", Name: "Owner"}
	teamMember := model.User{ID: uuid.New(), Email: "team-" + uuid.NewString() + "@example.test", Name: "TeamMember"}
	outsider := model.User{ID: uuid.New(), Email: "outsider-" + uuid.NewString() + "@example.test", Name: "Outsider"}
	for _, row := range []any{
		&owner, &teamMember, &outsider,
		&model.OrgMembership{UserID: owner.ID, OrgID: org.ID, Role: "owner"},
		&model.OrgMembership{UserID: teamMember.ID, OrgID: org.ID, Role: "member"},
		&model.OrgMembership{UserID: outsider.ID, OrgID: org.ID, Role: "member"},
		&model.TeamMember{OrgID: org.ID, TeamID: team.ID, UserID: teamMember.ID, Role: "member"},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed row %T: %v", row, err)
		}
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.TeamMember{})
		db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		db.Delete(&model.User{}, "id IN ?", []uuid.UUID{owner.ID, teamMember.ID, outsider.ID})
	})

	reqFor := func(actorID string) *mcp.CallToolRequest {
		args, _ := json.Marshal(map[string]any{"_hivy_actor_user_id": actorID})
		return &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: args}}
	}

	// An org manager (owner) is allowed for any team.
	if res := requireTeamManager(t.Context(), db, org.ID, team.ID, reqFor(owner.ID.String()), "creating an agent"); res != nil {
		t.Fatalf("owner should be allowed: %v", res.Content)
	}

	// An active member of the target team is allowed.
	if res := requireTeamManager(t.Context(), db, org.ID, team.ID, reqFor(teamMember.ID.String()), "creating an agent"); res != nil {
		t.Fatalf("team member should be allowed for own team: %v", res.Content)
	}

	// A member of the org but not the target team is blocked with a clear message.
	blocked := requireTeamManager(t.Context(), db, org.ID, otherTeam.ID, reqFor(teamMember.ID.String()), "creating an agent")
	if blocked == nil {
		t.Fatal("non-member of the target team should be blocked")
	}
	msg := blocked.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(msg, "member of the agent's team") {
		t.Fatalf("error should tell the actor to be a member of the team: %q", msg)
	}

	// A plain org member with no team membership at all is blocked.
	if res := requireTeamManager(t.Context(), db, org.ID, team.ID, reqFor(outsider.ID.String()), "creating an agent"); res == nil {
		t.Fatal("org member outside the team should be blocked")
	}

	// No actor (automated run) fails closed: these tools mutate agents and must
	// not be reachable from an externally-triggerable run.
	if res := requireTeamManager(t.Context(), db, org.ID, team.ID, reqFor(""), "creating an agent"); res == nil {
		t.Fatal("automated run (no actor) must be blocked")
	}

	// A nil/zero team is manager-only: a plain member is blocked even though they
	// belong to a team, and an owner still passes.
	if res := requireTeamManager(t.Context(), db, org.ID, uuid.Nil, reqFor(teamMember.ID.String()), "creating an agent"); res == nil {
		t.Fatal("nil team must be manager-only (member blocked)")
	}
	if res := requireTeamManager(t.Context(), db, org.ID, uuid.Nil, reqFor(owner.ID.String()), "creating an agent"); res != nil {
		t.Fatalf("nil team should still allow a manager: %v", res.Content)
	}

	// A present-but-non-member id is a hard, explicit error (never silently allowed).
	if res := requireTeamManager(t.Context(), db, org.ID, team.ID, reqFor(uuid.NewString()), "creating an agent"); res == nil {
		t.Fatal("a non-member actor id must be rejected")
	}
}

// TestAuthorizeUpdateTarget verifies the update_agent gate loads the target
// org-scoped (so a cross-org id is reported as not found and never leaks) and
// then applies the team-management rule against the target agent's own team.
func TestAuthorizeUpdateTarget(t *testing.T) {
	db := testDB(t)
	org := testOrg(t, db)
	other := testOrg(t, db)
	team := testTeam(t, db, org.ID)
	otherTeam := testTeam(t, db, org.ID)
	deps := noopDeps(db)

	owner := model.User{ID: uuid.New(), Email: "owner-" + uuid.NewString() + "@example.test", Name: "Owner"}
	teamMember := model.User{ID: uuid.New(), Email: "team-" + uuid.NewString() + "@example.test", Name: "TeamMember"}
	outsider := model.User{ID: uuid.New(), Email: "outsider-" + uuid.NewString() + "@example.test", Name: "Outsider"}
	agent := model.Agent{ID: uuid.New(), OrgID: &org.ID, Name: "Target", Model: "test", Status: "active", TeamID: team.ID}
	otherTeamAgent := model.Agent{ID: uuid.New(), OrgID: &org.ID, Name: "Other team target", Model: "test", Status: "active", TeamID: otherTeam.ID}
	for _, row := range []any{
		&owner, &teamMember, &outsider, &agent, &otherTeamAgent,
		&model.OrgMembership{UserID: owner.ID, OrgID: org.ID, Role: "owner"},
		&model.OrgMembership{UserID: teamMember.ID, OrgID: org.ID, Role: "member"},
		&model.OrgMembership{UserID: outsider.ID, OrgID: org.ID, Role: "member"},
		&model.TeamMember{OrgID: org.ID, TeamID: team.ID, UserID: teamMember.ID, Role: "member"},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed row %T: %v", row, err)
		}
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.TeamMember{})
		db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		db.Delete(&model.User{}, "id IN ?", []uuid.UUID{owner.ID, teamMember.ID, outsider.ID})
	})

	reqFor := func(actorID string) *mcp.CallToolRequest {
		args, _ := json.Marshal(map[string]any{"_hivy_actor_user_id": actorID})
		return &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: args}}
	}

	// A cross-org id reports as not found, without leaking that it exists elsewhere.
	crossOrg := authorizeUpdateTarget(t.Context(), deps, other.ID, team.ID, reqFor(owner.ID.String()), agent.ID)
	if crossOrg == nil {
		t.Fatal("cross-org update target must be rejected")
	}
	if msg := crossOrg.Content[0].(*mcp.TextContent).Text; !strings.Contains(msg, ErrAgentNotFound.Error()) {
		t.Fatalf("cross-org rejection should read as not found: %q", msg)
	}

	// Even an org owner cannot widen Hivy beyond Hivy's own team.
	crossTeam := authorizeUpdateTarget(t.Context(), deps, org.ID, team.ID, reqFor(owner.ID.String()), otherTeamAgent.ID)
	if crossTeam == nil {
		t.Fatal("cross-team update target must be rejected even for an org owner")
	}
	if msg := crossTeam.Content[0].(*mcp.TextContent).Text; !strings.Contains(msg, ErrAgentNotFound.Error()) {
		t.Fatalf("cross-team rejection should read as not found: %q", msg)
	}

	// A member of the target agent's team may update it.
	if res := authorizeUpdateTarget(t.Context(), deps, org.ID, team.ID, reqFor(teamMember.ID.String()), agent.ID); res != nil {
		t.Fatalf("team member should be allowed to update own-team agent: %v", res.Content)
	}

	// An org member outside the agent's team is denied.
	if res := authorizeUpdateTarget(t.Context(), deps, org.ID, team.ID, reqFor(outsider.ID.String()), agent.ID); res == nil {
		t.Fatal("non-member of the agent's team must be denied")
	}

	// No actor (automated run) fails closed.
	if res := authorizeUpdateTarget(t.Context(), deps, org.ID, team.ID, reqFor(""), agent.ID); res == nil {
		t.Fatal("automated run (no actor) must be blocked from updating")
	}
}
