package handler

import (
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

func connectTeamAuthzTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.Open(testdb.DatabaseURL("DATABASE_URL", "HIVY_DATABASE_URL", "TEST_DATABASE_URL")), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	testdb.ApplyMigrations(t, db)
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

func TestIsOrgOwnerHTTP(t *testing.T) {
	if !isOrgOwner("owner") {
		t.Errorf("owner should be org owner")
	}
	for _, role := range []string{"admin", "member", "unknown", ""} {
		if isOrgOwner(role) {
			t.Errorf("role %q must not be org owner", role)
		}
		// isOrgManager still admits admin — owner/admin must be distinguishable.
		if role == "admin" && !isOrgManager(role) {
			t.Errorf("admin must remain a manager")
		}
	}
}

// TestTeamAuthzMigrationSchema asserts migration 000081 applied cleanly:
// agents.team_id exists, the new tables exist, and the goose version is 81.
func TestTeamAuthzMigrationSchema(t *testing.T) {
	db := connectTeamAuthzTestDB(t)

	var colCount int64
	if err := db.Raw(`SELECT count(*) FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'agents' AND column_name = 'team_id'`).
		Scan(&colCount).Error; err != nil {
		t.Fatalf("query agents.team_id: %v", err)
	}
	if colCount != 1 {
		t.Errorf("agents.team_id present=%d, want 1", colCount)
	}

	for _, table := range []string{"team_plugins", "team_rag_sources"} {
		var n int64
		if err := db.Raw(`SELECT count(*) FROM information_schema.tables
			WHERE table_schema = current_schema() AND table_name = ?`, table).Scan(&n).Error; err != nil {
			t.Fatalf("query table %s: %v", table, err)
		}
		if n != 1 {
			t.Errorf("table %s present=%d, want 1", table, n)
		}
	}

	var version int64
	if err := db.Raw(`SELECT COALESCE(MAX(version_id) FILTER (WHERE is_applied), 0) FROM goose_db_version`).
		Scan(&version).Error; err != nil {
		t.Fatalf("query goose version: %v", err)
	}
	if version < 81 {
		t.Errorf("goose version=%d, want >= 81", version)
	}
}

type manageFixture struct {
	org     model.Org
	owner   model.User
	member  model.User
	teamIn  model.Team
	teamOut model.Team
	chTeam  model.Channel // standard, teamIn
	chOther model.Channel // standard, teamOut
	chNull  model.Channel // standard, no team
	chExt   model.Channel // external origin, teamIn
	chSys   model.Channel // system kind, no team
}

func seedManageFixture(t *testing.T, db *gorm.DB) manageFixture {
	t.Helper()
	org := model.Org{ID: uuid.New(), Name: "mng-" + uuid.NewString()[:8], Active: true, RateLimit: 1000}
	owner := model.User{ID: uuid.New(), Email: "o-" + uuid.NewString()[:8] + "@t.com", Name: "owner"}
	member := model.User{ID: uuid.New(), Email: "m-" + uuid.NewString()[:8] + "@t.com", Name: "member"}
	agent := model.Agent{ID: uuid.New(), OrgID: &org.ID, Name: "A-" + uuid.NewString()[:8], Model: "test", Status: "active"}
	teamIn := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "in-" + uuid.NewString()[:8]}
	teamOut := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "out-" + uuid.NewString()[:8]}

	ch := func(name, kind, origin string, team *uuid.UUID) model.Channel {
		return model.Channel{ID: uuid.New(), OrgID: org.ID, Name: name + "-" + uuid.NewString()[:8], Kind: kind, Origin: origin, TeamID: team, DefaultAgentID: agent.ID}
	}
	chTeam := ch("team", "standard", "native", &teamIn.ID)
	chOther := ch("other", "standard", "native", &teamOut.ID)
	chNull := ch("null", "standard", "native", nil)
	chExt := ch("ext", "standard", "external", &teamIn.ID)
	chSys := ch("sys", "system", "native", nil)

	rows := []any{
		&org, &owner, &member, &agent,
		&model.OrgMembership{UserID: owner.ID, OrgID: org.ID, Role: "owner"},
		&model.OrgMembership{UserID: member.ID, OrgID: org.ID, Role: "member"},
		&teamIn, &teamOut,
		&model.TeamMember{OrgID: org.ID, TeamID: teamIn.ID, UserID: member.ID, Role: "member"},
		&chTeam, &chOther, &chNull, &chExt, &chSys,
	}
	for _, r := range rows {
		if err := db.Create(r).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.Channel{})
		db.Where("org_id = ?", org.ID).Delete(&model.TeamMember{})
		db.Where("org_id = ?", org.ID).Delete(&model.Team{})
		db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		db.Where("id IN ?", []uuid.UUID{owner.ID, member.ID}).Delete(&model.User{})
		db.Delete(&model.Org{}, "id = ?", org.ID)
	})
	return manageFixture{org: org, owner: owner, member: member, teamIn: teamIn, teamOut: teamOut, chTeam: chTeam, chOther: chOther, chNull: chNull, chExt: chExt, chSys: chSys}
}

func TestCanManageTeamResourceHTTP(t *testing.T) {
	db := connectTeamAuthzTestDB(t)
	fx := seedManageFixture(t, db)
	ctx := t.Context()

	if !canManageTeamResource(ctx, db, fx.org.ID, &fx.owner.ID, "owner", fx.teamOut.ID) {
		t.Errorf("manager should manage any team")
	}
	if !canManageTeamResource(ctx, db, fx.org.ID, &fx.member.ID, "member", fx.teamIn.ID) {
		t.Errorf("member should manage own team")
	}
	if canManageTeamResource(ctx, db, fx.org.ID, &fx.member.ID, "member", fx.teamOut.ID) {
		t.Errorf("member should not manage other team")
	}
	if canManageTeamResource(ctx, db, fx.org.ID, nil, "member", fx.teamIn.ID) {
		t.Errorf("nil user, non-manager should not manage")
	}
}

func TestCanManageChannelMatrix(t *testing.T) {
	db := connectTeamAuthzTestDB(t)
	fx := seedManageFixture(t, db)
	ctx := t.Context()

	type tc struct {
		name    string
		channel model.Channel
		userID  *uuid.UUID
		role    string
		apiKey  bool
		want    bool
	}
	owner := fx.owner.ID
	member := fx.member.ID
	cases := []tc{
		// API key: always true, regardless of team.
		{"apikey-null-team", fx.chNull, nil, "", true, true},
		{"apikey-other-team", fx.chOther, nil, "", true, true},
		// Manager (owner): always true.
		{"owner-team", fx.chTeam, &owner, "owner", false, true},
		{"owner-null", fx.chNull, &owner, "owner", false, true},
		{"owner-system", fx.chSys, &owner, "owner", false, true},
		// Member: gated by the channel's team.
		{"member-own-team", fx.chTeam, &member, "member", false, true},
		{"member-other-team", fx.chOther, &member, "member", false, false},
		{"member-null-team", fx.chNull, &member, "member", false, false},
		{"member-external-own-team", fx.chExt, &member, "member", false, true},
		{"member-system", fx.chSys, &member, "member", false, false},
		// nil user, non-manager: false.
		{"nil-user-team", fx.chTeam, nil, "member", false, false},
	}
	for _, c := range cases {
		if got := canManageChannel(ctx, db, c.channel, c.userID, c.role, c.apiKey); got != c.want {
			t.Errorf("%s: canManageChannel=%v, want %v", c.name, got, c.want)
		}
	}
}
