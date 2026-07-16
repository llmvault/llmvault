package skillaccess

import (
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/teamprovision"
	"github.com/usehivy/hivy/internal/testdb"
)

// TestSkillGrantsAreIndependentFromConnectionGrants exercises direct org-skill
// grants, team ownership, cross-team isolation, and a concrete connection grant
// in the same persisted authorization graph. Connection lifecycle changes must
// not mutate the team's independently managed skills.
func TestSkillGrantsAreIndependentFromConnectionGrants(t *testing.T) {
	db, err := gorm.Open(postgres.Open(testdb.DatabaseURL()), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	testdb.ApplyMigrations(t, db)
	org := model.Org{ID: uuid.New(), Name: "skill-flow-" + uuid.NewString()[:8], Active: true}
	user := model.User{ID: uuid.New(), Email: "skill-flow-" + uuid.NewString() + "@example.test"}
	teamA := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "A"}
	teamB := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "B"}
	for _, row := range []any{&org, &user, &teamA, &teamB} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed %T: %v", row, err)
		}
	}

	orgSkill := model.Skill{ID: uuid.New(), OrgID: &org.ID, Slug: "org-" + uuid.NewString()[:8], Name: "Org", SourceType: model.SkillSourceInline, Bundle: model.RawJSON(`{}`), Status: model.SkillStatusPublished}
	ownedA := model.Skill{ID: uuid.New(), OrgID: &org.ID, TeamID: &teamA.ID, Slug: "team-a-" + uuid.NewString()[:8], Name: "Team A", SourceType: model.SkillSourceInline, Bundle: model.RawJSON(`{}`), Status: model.SkillStatusPublished}
	ownedB := model.Skill{ID: uuid.New(), OrgID: &org.ID, TeamID: &teamB.ID, Slug: "team-b-" + uuid.NewString()[:8], Name: "Team B", SourceType: model.SkillSourceInline, Bundle: model.RawJSON(`{}`), Status: model.SkillStatusPublished}
	for _, row := range []any{&orgSkill, &ownedA, &ownedB} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed skill: %v", err)
		}
	}

	integration := model.Integration{ID: uuid.New(), UniqueKey: "linear-" + uuid.NewString(), Provider: "linear", DisplayName: "Linear"}
	connectionA := model.Connection{ID: uuid.New(), OrgID: org.ID, UserID: user.ID, IntegrationID: integration.ID, NangoConnectionID: "one"}
	connectionB := model.Connection{ID: uuid.New(), OrgID: org.ID, UserID: user.ID, IntegrationID: integration.ID, NangoConnectionID: "two", Name: "second", Slug: "second"}
	for _, row := range []any{&integration, &connectionA, &connectionB} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed connection: %v", err)
		}
	}
	if err := teamprovision.GrantSkill(t.Context(), db, org.ID, teamA.ID, orgSkill.ID, &user.ID); err != nil {
		t.Fatalf("grant org skill: %v", err)
	}
	if err := teamprovision.GrantConnection(t.Context(), db, org.ID, teamA.ID, connectionB.ID, &user.ID); err != nil {
		t.Fatalf("grant second connection instance: %v", err)
	}

	agentA := model.Agent{OrgID: &org.ID, TeamID: teamA.ID}
	got, err := ResolveAgent(t.Context(), db, agentA)
	if err != nil {
		t.Fatalf("resolve team A: %v", err)
	}
	assertEffectiveSources(t, got, map[string]string{
		ownedA.Slug:   SourceTeamOwned,
		orgSkill.Slug: SourceTeamGrant,
	})
	if containsSlug(got, ownedB.Slug) {
		t.Fatal("team A received team B skill")
	}

	gotB, err := ResolveAgent(t.Context(), db, model.Agent{OrgID: &org.ID, TeamID: teamB.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(gotB) != 1 || gotB[0].Skill.ID != ownedB.ID {
		t.Fatalf("team B skills=%v, want only owned skill", slugs(gotB))
	}

	if err := teamprovision.RevokeConnection(t.Context(), db, org.ID, teamA.ID, connectionB.ID); err != nil {
		t.Fatalf("revoke connection: %v", err)
	}
	after, err := ResolveAgent(t.Context(), db, agentA)
	if err != nil {
		t.Fatal(err)
	}
	if !containsSlug(after, orgSkill.Slug) || !containsSlug(after, ownedA.Slug) {
		t.Fatalf("independent skills disappeared after connection revoke: %v", slugs(after))
	}
}

func assertEffectiveSources(t *testing.T, got []EffectiveSkill, want map[string]string) {
	t.Helper()
	for slug, source := range want {
		found := false
		for _, item := range got {
			if item.Skill.Slug == slug {
				found = contains(item.Sources, source)
			}
		}
		if !found {
			t.Fatalf("skill %s missing source %s; got=%v", slug, source, slugs(got))
		}
	}
}

func containsSlug(items []EffectiveSkill, slug string) bool {
	for _, item := range items {
		if item.Skill.Slug == slug {
			return true
		}
	}
	return false
}

func slugs(items []EffectiveSkill) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Skill.Slug)
	}
	return out
}
