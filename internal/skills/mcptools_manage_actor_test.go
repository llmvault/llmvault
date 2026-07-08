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

// The skill-manager tools are authorized on the invoking human's org role, not
// merely on the calling agent's capability flag: a member is denied, an org
// manager is allowed, and an absent actor (automated run) is denied.
func TestSkillManagerActorScoping(t *testing.T) {
	db := connectManageTestDB(t)
	ctx := context.Background()

	org := model.Org{ID: uuid.New(), Name: "skill-actor-" + uuid.NewString()[:8], RateLimit: 1000}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })
	token := &model.Token{OrgID: org.ID}

	memberID := seedActor(t, db, org.ID, "member")
	ownerID := seedActor(t, db, org.ID, "owner")

	// A member is denied create_org_plugin.
	res, _ := handleCreateOrgPlugin(ctx, db, token, createOrgPluginArgs{Name: "Sales", HivyActorUserID: memberID})
	if res == nil || !res.IsError || !strings.Contains(toolResultText(res), "admin or owner") {
		t.Fatalf("member must be denied create_org_plugin, got: %s", toolResultText(res))
	}

	// A member is denied create_skill (the actor gate runs before plugin lookup).
	res, _ = handleCreateSkill(ctx, db, token, "http://localhost:3000", createSkillArgs{
		PluginSlug: "whatever", Name: "X", Description: "Use when testing.", Content: "# X", HivyActorUserID: memberID,
	})
	if res == nil || !res.IsError || !strings.Contains(toolResultText(res), "admin or owner") {
		t.Fatalf("member must be denied create_skill, got: %s", toolResultText(res))
	}

	// A member is denied archive_skill.
	res, _ = handleArchiveSkill(ctx, db, token, archiveSkillArgs{
		PluginSlug: "whatever", Skill: "x", UserApproved: true, HivyActorUserID: memberID,
	})
	if res == nil || !res.IsError || !strings.Contains(toolResultText(res), "admin or owner") {
		t.Fatalf("member must be denied archive_skill, got: %s", toolResultText(res))
	}

	// An org manager (owner) is allowed and the plugin is created.
	res, err := handleCreateOrgPlugin(ctx, db, token, createOrgPluginArgs{Name: "Sales", HivyActorUserID: ownerID})
	if err != nil {
		t.Fatalf("owner create_org_plugin: %v", err)
	}
	if res == nil || res.IsError {
		t.Fatalf("owner must be allowed create_org_plugin, got: %s", toolResultText(res))
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.OrgPluginInstall{})
		db.Where("org_id = ?", org.ID).Delete(&model.Plugin{})
	})

	// No actor (empty id, e.g. an automated/cron/trigger run) is REJECTED:
	// org-wide skill/plugin mutation requires a present human org-manager, so a
	// possibly prompt-injected faceless run cannot author org plugins.
	res, err = handleCreateOrgPlugin(ctx, db, token, createOrgPluginArgs{Name: "Marketing", HivyActorUserID: ""})
	if err != nil {
		t.Fatalf("no-actor create_org_plugin: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("no-actor run must be rejected, got: %s", toolResultText(res))
	}
}
