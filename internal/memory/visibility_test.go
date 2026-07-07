package memory

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// visibilityFixture builds on the base memory fixture an org whose member
// (fx.user) belongs to teamA only, plus channels exercising every arm of the
// team-based visibility predicate: team-A-scoped (usable), team-B-scoped (not
// usable), team-less native (usable via team_id IS NULL), and externally
// sourced under team B (usable via origin='external').
type visibilityFixture struct {
	fx           memoryToolFixture
	chTeamA      uuid.UUID
	chTeamB      uuid.UUID
	chNativeNull uuid.UUID
	chExternal   uuid.UUID
	// memory ids by the channel they live in (mGlobal has no channel).
	mGlobal uuid.UUID
	mTeamA  uuid.UUID
	mTeamB  uuid.UUID
	mNull   uuid.UUID
	mExt    uuid.UUID
}

func seedVisibilityFixture(t *testing.T, db *gorm.DB, service *Service) visibilityFixture {
	t.Helper()
	fx := seedMemoryToolFixture(t, db)

	teamA := model.Team{ID: uuid.New(), OrgID: fx.org.ID, Name: "team-a-" + uuid.NewString()}
	teamB := model.Team{ID: uuid.New(), OrgID: fx.org.ID, Name: "team-b-" + uuid.NewString()}
	if err := db.Create(&teamA).Error; err != nil {
		t.Fatalf("create teamA: %v", err)
	}
	if err := db.Create(&teamB).Error; err != nil {
		t.Fatalf("create teamB: %v", err)
	}
	// fx.user is a member of teamA only.
	if err := db.Create(&model.TeamMember{OrgID: fx.org.ID, TeamID: teamA.ID, UserID: fx.user.ID, Role: "member"}).Error; err != nil {
		t.Fatalf("add user to teamA: %v", err)
	}

	newChannel := func(name string, teamID *uuid.UUID, origin string) uuid.UUID {
		ch := model.Channel{
			ID:             uuid.New(),
			OrgID:          fx.org.ID,
			Name:           name + "-" + uuid.NewString(),
			DefaultAgentID: fx.agent.ID,
			TeamID:         teamID,
			Origin:         origin,
		}
		if err := db.Create(&ch).Error; err != nil {
			t.Fatalf("create channel %s: %v", name, err)
		}
		return ch.ID
	}
	vf := visibilityFixture{fx: fx}
	vf.chTeamA = newChannel("team-a-ch", &teamA.ID, "native")
	vf.chTeamB = newChannel("team-b-ch", &teamB.ID, "native")
	vf.chNativeNull = newChannel("native-null-ch", nil, "native")
	vf.chExternal = newChannel("external-ch", &teamB.ID, "external")

	vf.mGlobal = seedReadyMemory(t, service, fx.org.ID, nil, "global memory")
	vf.mTeamA = seedReadyMemory(t, service, fx.org.ID, &vf.chTeamA, "team A memory")
	vf.mTeamB = seedReadyMemory(t, service, fx.org.ID, &vf.chTeamB, "team B memory (secret)")
	vf.mNull = seedReadyMemory(t, service, fx.org.ID, &vf.chNativeNull, "native null channel memory")
	vf.mExt = seedReadyMemory(t, service, fx.org.ID, &vf.chExternal, "external channel memory")

	t.Cleanup(func() {
		db.Where("org_id = ?", fx.org.ID).Delete(&model.TeamMember{})
		db.Where("org_id = ?", fx.org.ID).Delete(&model.Team{})
	})
	return vf
}

func idSet(ids ...uuid.UUID) map[uuid.UUID]bool {
	out := map[uuid.UUID]bool{}
	for _, id := range ids {
		out[id] = true
	}
	return out
}

func collectMemoryIDs(rows []model.AgentMemory) map[uuid.UUID]bool {
	out := map[uuid.UUID]bool{}
	for _, r := range rows {
		out[r.ID] = true
	}
	return out
}

// TestListVisibilityRestrictsToUsableChannels covers L3: a non-manager member
// must not receive memories referencing channels they cannot use, while global
// (channel-less) memories stay visible.
func TestListVisibilityRestrictsToUsableChannels(t *testing.T) {
	db := connectMemoryToolTestDB(t)
	ctx := context.Background()
	service := NewService(Config{DB: db, Embedder: staticMemoryToolEmbedder{vector: testMemoryVector()}})
	vf := seedVisibilityFixture(t, db, service)

	// Member: restricted visibility.
	rows, err := service.List(ctx, ListRequest{
		OrgID:      vf.fx.org.ID,
		Scope:      ChannelScope{AllChannels: true},
		Visibility: Visibility{Restrict: true, UserID: &vf.fx.user.ID},
		NoLimit:    true,
	})
	if err != nil {
		t.Fatalf("member list: %v", err)
	}
	got := collectMemoryIDs(rows)
	want := idSet(vf.mGlobal, vf.mTeamA, vf.mNull, vf.mExt)
	if len(got) != len(want) {
		t.Fatalf("member sees %d memories, want %d: %v", len(got), len(want), got)
	}
	for id := range want {
		if !got[id] {
			t.Fatalf("member should see memory %s but did not", id)
		}
	}
	if got[vf.mTeamB] {
		t.Fatalf("LEAK: member saw team-B channel memory %s", vf.mTeamB)
	}

	// Manager / API-key (unrestricted): sees everything including team B.
	all, err := service.List(ctx, ListRequest{
		OrgID:   vf.fx.org.ID,
		Scope:   ChannelScope{AllChannels: true},
		NoLimit: true,
	})
	if err != nil {
		t.Fatalf("admin list: %v", err)
	}
	adminGot := collectMemoryIDs(all)
	for _, id := range []uuid.UUID{vf.mGlobal, vf.mTeamA, vf.mTeamB, vf.mNull, vf.mExt} {
		if !adminGot[id] {
			t.Fatalf("admin/unrestricted should see memory %s but did not", id)
		}
	}
}

// TestListVisibilityNilUserSeesOnlyOpenChannels mirrors an empty team
// membership: only external and team-less channels (plus global) survive.
func TestListVisibilityNilUserSeesOnlyOpenChannels(t *testing.T) {
	db := connectMemoryToolTestDB(t)
	ctx := context.Background()
	service := NewService(Config{DB: db, Embedder: staticMemoryToolEmbedder{vector: testMemoryVector()}})
	vf := seedVisibilityFixture(t, db, service)

	rows, err := service.List(ctx, ListRequest{
		OrgID:      vf.fx.org.ID,
		Scope:      ChannelScope{AllChannels: true},
		Visibility: Visibility{Restrict: true, UserID: nil},
		NoLimit:    true,
	})
	if err != nil {
		t.Fatalf("nil-user list: %v", err)
	}
	got := collectMemoryIDs(rows)
	want := idSet(vf.mGlobal, vf.mNull, vf.mExt)
	if len(got) != len(want) {
		t.Fatalf("nil user sees %d memories, want %d: %v", len(got), len(want), got)
	}
	if got[vf.mTeamA] || got[vf.mTeamB] {
		t.Fatalf("LEAK: nil user saw team-scoped memory")
	}
}

// TestGroupedVisibilityOmitsUnusableChannels covers L4: grouped view must not
// surface a channel (or its name) the member cannot use.
func TestGroupedVisibilityOmitsUnusableChannels(t *testing.T) {
	db := connectMemoryToolTestDB(t)
	ctx := context.Background()
	service := NewService(Config{DB: db, Embedder: staticMemoryToolEmbedder{vector: testMemoryVector()}})
	vf := seedVisibilityFixture(t, db, service)

	groups, err := service.GroupedByChannel(ctx, vf.fx.org.ID, 10, Visibility{Restrict: true, UserID: &vf.fx.user.ID})
	if err != nil {
		t.Fatalf("member grouped: %v", err)
	}
	seenChannels := map[uuid.UUID]bool{}
	globalSeen := false
	for _, g := range groups {
		if g.ChannelID == nil {
			globalSeen = true
			continue
		}
		seenChannels[*g.ChannelID] = true
	}
	if !globalSeen {
		t.Fatalf("grouped omitted global group")
	}
	if seenChannels[vf.chTeamB] {
		t.Fatalf("LEAK: grouped surfaced team-B channel %s to member", vf.chTeamB)
	}
	for _, want := range []uuid.UUID{vf.chTeamA, vf.chNativeNull, vf.chExternal} {
		if !seenChannels[want] {
			t.Fatalf("grouped omitted usable channel %s", want)
		}
	}

	// Unrestricted sees team B too.
	allGroups, err := service.GroupedByChannel(ctx, vf.fx.org.ID, 10, Visibility{})
	if err != nil {
		t.Fatalf("admin grouped: %v", err)
	}
	teamBSeen := false
	for _, g := range allGroups {
		if g.ChannelID != nil && *g.ChannelID == vf.chTeamB {
			teamBSeen = true
		}
	}
	if !teamBSeen {
		t.Fatalf("admin grouped should include team-B channel")
	}
}

// TestSearchVisibilityRestrictsToUsableChannels guards the semantic-search arm
// of GET /v1/memories against the same leak.
func TestSearchVisibilityRestrictsToUsableChannels(t *testing.T) {
	db := connectMemoryToolTestDB(t)
	ctx := context.Background()
	service := NewService(Config{DB: db, Embedder: staticMemoryToolEmbedder{vector: testMemoryVector()}})
	vf := seedVisibilityFixture(t, db, service)

	hits, err := service.Search(ctx, SearchRequest{
		OrgID:       vf.fx.org.ID,
		Scope:       ChannelScope{AllChannels: true},
		Visibility:  Visibility{Restrict: true, UserID: &vf.fx.user.ID},
		QueryVector: testMemoryVector(),
		Limit:       50,
	})
	if err != nil {
		t.Fatalf("member search: %v", err)
	}
	for _, h := range hits {
		if h.Memory.ID == vf.mTeamB {
			t.Fatalf("LEAK: search returned team-B channel memory %s", vf.mTeamB)
		}
	}
	// The usable memories are all present.
	found := map[uuid.UUID]bool{}
	for _, h := range hits {
		found[h.Memory.ID] = true
	}
	for _, want := range []uuid.UUID{vf.mGlobal, vf.mTeamA, vf.mNull, vf.mExt} {
		if !found[want] {
			t.Fatalf("search omitted usable memory %s", want)
		}
	}
}
