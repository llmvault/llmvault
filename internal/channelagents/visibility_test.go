package channelagents_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/channelagents"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

func connect(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.Open(testdb.DatabaseURL()), &gorm.Config{})
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

// visibleIDs runs VisibleAgentIDsSubquery and materializes the agent ids it
// yields (the subquery is normally embedded in `agents.id IN (?)`).
func visibleIDs(t *testing.T, db *gorm.DB, orgID uuid.UUID, userID *uuid.UUID) map[uuid.UUID]bool {
	t.Helper()
	var ids []uuid.UUID
	if err := db.Table("(?) AS vis", channelagents.VisibleAgentIDsSubquery(db, orgID, userID)).
		Pluck("id", &ids).Error; err != nil {
		t.Fatalf("run subquery: %v", err)
	}
	out := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

// TestVisibleAgentIDsSubquery verifies the team-primary visibility rule: a
// non-manager user sees exactly the agents belonging to teams they actively
// belong to; a nil user sees nothing.
func TestVisibleAgentIDsSubquery(t *testing.T) {
	db := connect(t)

	org := model.Org{ID: uuid.New(), Name: "vis-" + uuid.NewString()[:8], Active: true, RateLimit: 1000}
	member := model.User{ID: uuid.New(), Email: "m-" + uuid.NewString()[:8] + "@t.com", Name: "m"}

	teamA := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "a-" + uuid.NewString()[:8]}
	teamB := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "b-" + uuid.NewString()[:8]}

	mkAgent := func(name string, team uuid.UUID) model.Agent {
		return model.Agent{ID: uuid.New(), OrgID: &org.ID, Name: name, Model: "test", Status: "active", TeamID: team}
	}
	inTeamA := mkAgent("InTeamA", teamA.ID)
	inTeamB := mkAgent("InTeamB", teamB.ID)

	rows := []any{
		&org, &member, &teamA, &teamB,
		&inTeamA, &inTeamB,
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
		db.Where("id = ?", member.ID).Delete(&model.User{})
		db.Delete(&model.Org{}, "id = ?", org.ID)
	})

	// A member of teamA sees teamA's agent only — not teamB's.
	got := visibleIDs(t, db, org.ID, &member.ID)
	if !got[inTeamA.ID] {
		t.Fatalf("member should see teamA agent %s; got %v", inTeamA.ID, got)
	}
	if got[inTeamB.ID] {
		t.Fatalf("member must NOT see %s (%s); got %v", inTeamB.Name, inTeamB.ID, got)
	}

	// A nil user (no membership) sees no agents.
	gotNil := visibleIDs(t, db, org.ID, nil)
	if len(gotNil) != 0 {
		t.Fatalf("nil user should see no agents; got %v", gotNil)
	}
}

// TestActsInChannel verifies the team-match gate: an agent may act iff it
// belongs to the channel's owning team. External channels are treated exactly
// like native team channels.
func TestActsInChannel(t *testing.T) {
	db := connect(t)
	ctx := context.Background()

	org := model.Org{ID: uuid.New(), Name: "act-" + uuid.NewString()[:8], Active: true, RateLimit: 1000}
	teamA := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "a-" + uuid.NewString()[:8]}
	teamB := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "b-" + uuid.NewString()[:8]}

	agentA := model.Agent{ID: uuid.New(), OrgID: &org.ID, Name: "A", Model: "test", Status: "active", TeamID: teamA.ID}
	agentB := model.Agent{ID: uuid.New(), OrgID: &org.ID, Name: "B", Model: "test", Status: "active", TeamID: teamB.ID}

	chTeamA := model.Channel{ID: uuid.New(), OrgID: org.ID, Name: "ta-" + uuid.NewString()[:8], Kind: "standard", TeamID: teamA.ID, DefaultAgentID: agentA.ID}
	chExt := model.Channel{ID: uuid.New(), OrgID: org.ID, Name: "e-" + uuid.NewString()[:8], Kind: "standard", Origin: "external", TeamID: teamA.ID, DefaultAgentID: agentA.ID}

	for _, r := range []any{&org, &teamA, &teamB, &agentA, &agentB, &chTeamA, &chExt} {
		if err := db.Create(r).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.Channel{})
		db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		db.Where("org_id = ?", org.ID).Delete(&model.Team{})
		db.Delete(&model.Org{}, "id = ?", org.ID)
	})

	cases := []struct {
		name    string
		channel uuid.UUID
		agent   uuid.UUID
		want    bool
	}{
		{"same team", chTeamA.ID, agentA.ID, true},
		{"different team rejected", chTeamA.ID, agentB.ID, false},
		{"external channel is team-scoped: same team", chExt.ID, agentA.ID, true},
		{"external channel is team-scoped: different team rejected", chExt.ID, agentB.ID, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := channelagents.ActsInChannel(ctx, db, org.ID, tc.channel, tc.agent)
			if err != nil {
				t.Fatalf("ActsInChannel: %v", err)
			}
			if got != tc.want {
				t.Fatalf("ActsInChannel = %v, want %v", got, tc.want)
			}
		})
	}
}
