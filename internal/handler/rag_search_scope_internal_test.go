package handler

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	ragmodel "github.com/usehivy/hivy/internal/rag/model"
	"github.com/usehivy/hivy/internal/testdb"
)

func connectScopeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := testdb.DatabaseURL("DATABASE_URL", "HIVY_DATABASE_URL", "TEST_DATABASE_URL")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
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

func TestIntersectSourceIDs(t *testing.T) {
	usable := []string{"a", "b", "c"}
	// Empty request means "all usable".
	if got := intersectSourceIDs(usable, nil); len(got) != 3 {
		t.Fatalf("empty request = %v, want all usable", got)
	}
	// Requested ids are clamped to the usable set.
	got := intersectSourceIDs(usable, []string{"b", "z"})
	if len(got) != 1 || got[0] != "b" {
		t.Fatalf("intersect = %v, want [b]", got)
	}
	// No overlap yields empty (deny).
	if got := intersectSourceIDs(usable, []string{"z"}); len(got) != 0 {
		t.Fatalf("no-overlap = %v, want empty", got)
	}
}

// TestUsableRagSourceIDs proves a non-manager member resolves only the sources
// granted to channels they can use (team-based predicate), the core of the
// /v1/rag/search scope fix.
func TestUsableRagSourceIDs(t *testing.T) {
	db := connectScopeTestDB(t)
	ctx := context.Background()

	org := model.Org{ID: uuid.New(), Name: "rsrc-" + uuid.NewString()[:8], RateLimit: 1000, Active: true}
	member := model.User{ID: uuid.New(), Email: "rsrc-" + uuid.NewString()[:8] + "@test.com", Name: "m"}
	agent := model.Agent{ID: uuid.New(), OrgID: &org.ID, Name: "a", Model: "test", Status: "active"}
	teamA := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "ta-" + uuid.NewString()[:8]}
	teamB := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "tb-" + uuid.NewString()[:8]}
	visibleCh := model.Channel{ID: uuid.New(), OrgID: org.ID, Name: "vc-" + uuid.NewString()[:8], Kind: "standard", TeamID: &teamA.ID, DefaultAgentID: agent.ID}
	hiddenCh := model.Channel{ID: uuid.New(), OrgID: org.ID, Name: "hc-" + uuid.NewString()[:8], Kind: "standard", TeamID: &teamB.ID, DefaultAgentID: agent.ID}

	// Base rows first (sources + grants carry FKs to org/channels).
	base := []any{
		&org, &member, &agent, &teamA, &teamB, &visibleCh, &hiddenCh,
		&model.OrgMembership{UserID: member.ID, OrgID: org.ID, Role: "member"},
		&model.TeamMember{OrgID: org.ID, TeamID: teamA.ID, UserID: member.ID, Role: "member"},
	}
	for _, r := range base {
		if err := db.Create(r).Error; err != nil {
			t.Fatalf("seed base: %v", err)
		}
	}
	mkSrc := func(name string) ragmodel.RAGSource {
		return ragmodel.RAGSource{
			OrgIDValue:  org.ID,
			KindValue:   ragmodel.RAGSourceKindWebsite,
			Name:        name,
			Status:      ragmodel.RAGSourceStatusActive,
			Enabled:     true,
			ConfigValue: model.JSON{"url": "https://" + name + ".example"},
		}
	}
	visSrc := mkSrc("vis-" + uuid.NewString()[:8])
	hidSrc := mkSrc("hid-" + uuid.NewString()[:8])
	if err := db.Create(&visSrc).Error; err != nil {
		t.Fatalf("seed vis source: %v", err)
	}
	if err := db.Create(&hidSrc).Error; err != nil {
		t.Fatalf("seed hid source: %v", err)
	}
	visibleSrc := visSrc.ID
	hiddenSrc := hidSrc.ID
	grants := []any{
		&model.ChannelRagSource{OrgID: org.ID, ChannelID: visibleCh.ID, RagSourceID: visibleSrc},
		&model.ChannelRagSource{OrgID: org.ID, ChannelID: hiddenCh.ID, RagSourceID: hiddenSrc},
	}
	for _, r := range grants {
		if err := db.Create(r).Error; err != nil {
			t.Fatalf("seed grant: %v", err)
		}
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.ChannelRagSource{})
		db.Where("org_id = ?", org.ID).Delete(&ragmodel.RAGSource{})
		db.Where("org_id = ?", org.ID).Delete(&model.Channel{})
		db.Where("org_id = ?", org.ID).Delete(&model.TeamMember{})
		db.Where("org_id = ?", org.ID).Delete(&model.Team{})
		db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		db.Where("id = ?", member.ID).Delete(&model.User{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})

	ids, err := usableRagSourceIDs(ctx, db, org.ID, &member.ID)
	if err != nil {
		t.Fatalf("usableRagSourceIDs: %v", err)
	}
	got := map[string]bool{}
	for _, id := range ids {
		got[id] = true
	}
	if !got[visibleSrc.String()] {
		t.Fatalf("member missing visible source: %v", got)
	}
	if got[hiddenSrc.String()] {
		t.Fatalf("member must not resolve hidden-channel source: %v", got)
	}
}
