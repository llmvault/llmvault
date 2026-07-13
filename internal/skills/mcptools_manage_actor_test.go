package skills

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// seedActor creates a user with an org membership of the given role and returns
// the user id as a string (the runtime-injected `_hivy_actor_user_id` form).
func seedActor(t *testing.T, db *gorm.DB, orgID uuid.UUID, role string) string {
	t.Helper()
	user := model.User{ID: uuid.New(), Email: role + "-" + uuid.NewString()[:8] + "@skills.test", Name: role, PasswordHash: "x"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", user.ID).Delete(&model.User{}) })
	if err := db.Create(&model.OrgMembership{UserID: user.ID, OrgID: orgID, Role: role}).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}
	t.Cleanup(func() { db.Where("user_id = ? AND org_id = ?", user.ID, orgID).Delete(&model.OrgMembership{}) })
	return user.ID.String()
}

// Skill-manager mutations are available to every active member of the calling
// agent's team. Members of another team and automated runs are denied; org
// managers retain cross-team access.
func TestSkillManagerActorScoping(t *testing.T) {
	db := connectManageTestDB(t)
	ctx := context.Background()

	org := model.Org{ID: uuid.New(), Name: "skill-actor-" + uuid.NewString()[:8], RateLimit: 1000}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })
	team := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "skill-actor-team-" + uuid.NewString()[:8]}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	agent := manageTestAgent(org.ID, team.ID)
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	token := manageTestToken(org.ID, agent.ID)

	memberID := seedActor(t, db, org.ID, "member")
	memberUUID := uuid.MustParse(memberID)
	if err := db.Create(&model.TeamMember{ID: uuid.New(), OrgID: org.ID, TeamID: team.ID, UserID: memberUUID, Role: "member"}).Error; err != nil {
		t.Fatalf("create team membership: %v", err)
	}
	outsiderID := seedActor(t, db, org.ID, "member")
	ownerID := seedActor(t, db, org.ID, "owner")
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.TeamPlugin{})
		db.Where("org_id = ?", org.ID).Delete(&model.OrgPluginInstall{})
		db.Where("org_id = ?", org.ID).Delete(&model.Plugin{})
		db.Where("org_id = ?", org.ID).Delete(&model.TeamMember{})
		db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		db.Where("id = ?", team.ID).Delete(&model.Team{})
	})

	// A member of the calling agent's team may create a team plugin.
	res, _ := handleCreateTeamPlugin(ctx, db, token, createTeamPluginArgs{Name: "Sales", HivyActorUserID: memberID})
	if res == nil || res.IsError {
		t.Fatalf("team member must be allowed create_team_plugin, got: %s", toolResultText(res))
	}

	// An org member outside the calling agent's team is denied.
	res, _ = handleCreateSkill(ctx, db, token, "http://localhost:3000", createSkillArgs{
		PluginSlug: "sales", Name: "X", Description: "Use when testing.", Content: "# X", HivyActorUserID: outsiderID,
	})
	if res == nil || !res.IsError || !strings.Contains(toolResultText(res), "membership") {
		t.Fatalf("member outside team must be denied create_skill, got: %s", toolResultText(res))
	}

	// An org manager (owner) is allowed and the plugin is created.
	res, err := handleCreateTeamPlugin(ctx, db, token, createTeamPluginArgs{Name: "Marketing", HivyActorUserID: ownerID})
	if err != nil {
		t.Fatalf("owner create_team_plugin: %v", err)
	}
	if res == nil || res.IsError {
		t.Fatalf("owner must be allowed create_team_plugin, got: %s", toolResultText(res))
	}
	// No actor (empty id, e.g. an automated/cron/trigger run) is rejected.
	res, err = handleCreateTeamPlugin(ctx, db, token, createTeamPluginArgs{Name: "Operations", HivyActorUserID: ""})
	if err != nil {
		t.Fatalf("no-actor create_team_plugin: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("no-actor run must be rejected, got: %s", toolResultText(res))
	}
}
