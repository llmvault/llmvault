package access_test

import (
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

func connectAccessTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.Open(testdb.DatabaseURL("DATABASE_URL", "HIVY_DATABASE_URL", "TEST_DATABASE_URL")), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	testdb.ApplyMigrations(t, db)
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

// predicateFixture: one org with owner/admin/member users and two teams. The
// member belongs to teamIn but not teamOut.
type predicateFixture struct {
	org     model.Org
	owner   model.User
	admin   model.User
	member  model.User
	teamIn  model.Team
	teamOut model.Team
}

func seedPredicateFixture(t *testing.T, db *gorm.DB) predicateFixture {
	t.Helper()
	org := model.Org{ID: uuid.New(), Name: "acc-" + uuid.NewString()[:8], Active: true, RateLimit: 1000}
	owner := model.User{ID: uuid.New(), Email: "owner-" + uuid.NewString()[:8] + "@t.com", Name: "owner"}
	admin := model.User{ID: uuid.New(), Email: "admin-" + uuid.NewString()[:8] + "@t.com", Name: "admin"}
	member := model.User{ID: uuid.New(), Email: "member-" + uuid.NewString()[:8] + "@t.com", Name: "member"}
	teamIn := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "in-" + uuid.NewString()[:8]}
	teamOut := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "out-" + uuid.NewString()[:8]}

	rows := []any{
		&org, &owner, &admin, &member,
		&model.OrgMembership{UserID: owner.ID, OrgID: org.ID, Role: "owner"},
		&model.OrgMembership{UserID: admin.ID, OrgID: org.ID, Role: "admin"},
		&model.OrgMembership{UserID: member.ID, OrgID: org.ID, Role: "member"},
		&teamIn, &teamOut,
		// Only the member is placed in teamIn. Owner/admin are in no team, to
		// prove manager rights do NOT depend on team membership.
		&model.TeamMember{OrgID: org.ID, TeamID: teamIn.ID, UserID: member.ID, Role: "member"},
	}
	for _, r := range rows {
		if err := db.Create(r).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.TeamMember{})
		db.Where("org_id = ?", org.ID).Delete(&model.Team{})
		db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		db.Where("id IN ?", []uuid.UUID{owner.ID, admin.ID, member.ID}).Delete(&model.User{})
		db.Delete(&model.Org{}, "id = ?", org.ID)
	})
	return predicateFixture{org: org, owner: owner, admin: admin, member: member, teamIn: teamIn, teamOut: teamOut}
}

func actorFor(fx predicateFixture, u model.User, role string) *access.Actor {
	return &access.Actor{UserID: u.ID, OrgID: fx.org.ID, OrgRole: role}
}

func TestIsOrgOwnerDistinguishesOwnerFromAdmin(t *testing.T) {
	fx := predicateFixture{org: model.Org{ID: uuid.New()}}
	cases := []struct {
		role string
		want bool
	}{
		{"owner", true},
		{"admin", false},
		{"member", false},
		{"viewer", false},
	}
	for _, c := range cases {
		a := actorFor(fx, model.User{ID: uuid.New()}, c.role)
		if got := a.IsOrgOwner(); got != c.want {
			t.Errorf("IsOrgOwner(role=%q)=%v, want %v", c.role, got, c.want)
		}
		// IsOrgManager stays owner OR admin — the two must differ for admin.
		if c.role == "admin" && !a.IsOrgManager() {
			t.Errorf("admin should be a manager")
		}
	}
	// nil-safe.
	var nilActor *access.Actor
	if nilActor.IsOrgOwner() {
		t.Errorf("nil actor must not be owner")
	}
}

func TestIsTeamMemberMatrix(t *testing.T) {
	db := connectAccessTestDB(t)
	fx := seedPredicateFixture(t, db)
	ctx := t.Context()

	cases := []struct {
		name  string
		actor *access.Actor
		team  uuid.UUID
		want  bool
	}{
		{"member-in-team", actorFor(fx, fx.member, "member"), fx.teamIn.ID, true},
		{"member-not-in-team", actorFor(fx, fx.member, "member"), fx.teamOut.ID, false},
		{"owner-not-in-team", actorFor(fx, fx.owner, "owner"), fx.teamIn.ID, false},
		{"admin-not-in-team", actorFor(fx, fx.admin, "admin"), fx.teamIn.ID, false},
	}
	for _, c := range cases {
		got, err := c.actor.IsTeamMember(ctx, db, c.team)
		if err != nil {
			t.Fatalf("%s: IsTeamMember err: %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s: IsTeamMember=%v, want %v", c.name, got, c.want)
		}
	}

	// nil-safe: nil actor is never a member and returns no error.
	var nilActor *access.Actor
	got, err := nilActor.IsTeamMember(ctx, db, fx.teamIn.ID)
	if err != nil || got {
		t.Errorf("nil actor IsTeamMember=(%v,%v), want (false,nil)", got, err)
	}
}

// TestCanUseChannelPrivate asserts the agent-path mirror gates private channels
// to explicit channel members even when origin/team would otherwise open them:
// a private external channel is unusable by a non-member member (regardless of
// team) but usable by a channel member; managers keep org-wide access.
func TestCanUseChannelPrivate(t *testing.T) {
	db := connectAccessTestDB(t)
	fx := seedPredicateFixture(t, db)
	ctx := t.Context()

	agent := model.Agent{ID: uuid.New(), OrgID: &fx.org.ID, Name: "a-" + uuid.NewString()[:8], Model: "test", Status: "active"}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	// Private + external: normally open to any org member, gated by private.
	priv := model.Channel{
		ID: uuid.New(), OrgID: fx.org.ID, Name: "priv-" + uuid.NewString()[:8],
		Kind: "standard", Origin: "external", Visibility: "private", DefaultAgentID: agent.ID,
	}
	if err := db.Create(&priv).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	// A second user who is a member of the channel but not of teamIn.
	chMember := model.User{ID: uuid.New(), Email: "chm-" + uuid.NewString()[:8] + "@t.com", Name: "chm"}
	if err := db.Create(&chMember).Error; err != nil {
		t.Fatalf("create channel member user: %v", err)
	}
	if err := db.Create(&model.OrgMembership{UserID: chMember.ID, OrgID: fx.org.ID, Role: "member"}).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}
	if err := db.Create(&model.ChannelMember{ChannelID: priv.ID, UserID: chMember.ID, Role: "member"}).Error; err != nil {
		t.Fatalf("create channel member: %v", err)
	}
	t.Cleanup(func() {
		db.Where("channel_id = ?", priv.ID).Delete(&model.ChannelMember{})
		db.Where("id = ?", priv.ID).Delete(&model.Channel{})
		db.Where("id = ?", agent.ID).Delete(&model.Agent{})
		db.Where("user_id = ?", chMember.ID).Delete(&model.OrgMembership{})
		db.Where("id = ?", chMember.ID).Delete(&model.User{})
	})

	cases := []struct {
		name  string
		actor *access.Actor
		want  bool
	}{
		{"non-member", actorFor(fx, fx.member, "member"), false},
		{"channel-member", actorFor(fx, chMember, "member"), true},
		{"manager", actorFor(fx, fx.admin, "admin"), true},
	}
	for _, c := range cases {
		if got := c.actor.CanUseChannel(ctx, db, priv); got != c.want {
			t.Errorf("%s: CanUseChannel=%v, want %v", c.name, got, c.want)
		}
	}
}

func TestCanManageTeamResourceMatrix(t *testing.T) {
	db := connectAccessTestDB(t)
	fx := seedPredicateFixture(t, db)
	ctx := t.Context()

	// owner/admin × in/out-team → always true (manager). member → team-gated.
	cases := []struct {
		name  string
		actor *access.Actor
		team  uuid.UUID
		want  bool
	}{
		{"owner-any-team", actorFor(fx, fx.owner, "owner"), fx.teamOut.ID, true},
		{"admin-any-team", actorFor(fx, fx.admin, "admin"), fx.teamOut.ID, true},
		{"member-in-team", actorFor(fx, fx.member, "member"), fx.teamIn.ID, true},
		{"member-not-in-team", actorFor(fx, fx.member, "member"), fx.teamOut.ID, false},
	}
	for _, c := range cases {
		got, err := c.actor.CanManageTeamResource(ctx, db, c.team)
		if err != nil {
			t.Fatalf("%s: err: %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s: CanManageTeamResource=%v, want %v", c.name, got, c.want)
		}
	}

	var nilActor *access.Actor
	got, err := nilActor.CanManageTeamResource(ctx, db, fx.teamIn.ID)
	if err != nil || got {
		t.Errorf("nil actor CanManageTeamResource=(%v,%v), want (false,nil)", got, err)
	}
}
