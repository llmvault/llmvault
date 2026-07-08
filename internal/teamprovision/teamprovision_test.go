package teamprovision_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	ragmodel "github.com/usehivy/hivy/internal/rag/model"
	"github.com/usehivy/hivy/internal/teamprovision"
	"github.com/usehivy/hivy/internal/testdb"
)

func connectDB(t *testing.T) *gorm.DB {
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

type fixture struct {
	org      model.Org
	team     model.Team
	otherOrg model.Org
}

func seedFixture(t *testing.T, db *gorm.DB) fixture {
	t.Helper()
	org := model.Org{ID: uuid.New(), Name: "tp-" + uuid.NewString()[:8], Active: true, RateLimit: 1000}
	otherOrg := model.Org{ID: uuid.New(), Name: "tpx-" + uuid.NewString()[:8], Active: true, RateLimit: 1000}
	team := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "team-" + uuid.NewString()[:8]}
	for _, rec := range []any{&org, &otherOrg, &team} {
		if err := db.Create(rec).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	t.Cleanup(func() {
		db.Where("id IN ?", []uuid.UUID{team.ID}).Delete(&model.Team{})
		db.Where("id IN ?", []uuid.UUID{org.ID, otherOrg.ID}).Delete(&model.Org{})
	})
	return fixture{org: org, team: team, otherOrg: otherOrg}
}

// seedPlugin creates a plugin (global when orgID==nil) and, when installed,
// an active org_plugin_installs row for installOrg.
func seedPlugin(t *testing.T, db *gorm.DB, orgID *uuid.UUID, installOrg *uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	plugin := model.Plugin{ID: id, OrgID: orgID, Slug: "pl-" + uuid.NewString()[:8], Name: "pl", Status: model.PluginStatusActive}
	if err := db.Create(&plugin).Error; err != nil {
		t.Fatalf("seed plugin: %v", err)
	}
	if installOrg != nil {
		if err := db.Create(&model.OrgPluginInstall{ID: uuid.New(), OrgID: *installOrg, PluginID: id}).Error; err != nil {
			t.Fatalf("seed install: %v", err)
		}
	}
	t.Cleanup(func() {
		db.Where("plugin_id = ?", id).Delete(&model.OrgPluginInstall{})
		db.Where("id = ?", id).Delete(&model.Plugin{})
	})
	return id
}

func seedSource(t *testing.T, db *gorm.DB, orgID uuid.UUID) uuid.UUID {
	t.Helper()
	src := ragmodel.RAGSource{
		ID: uuid.New(), OrgIDValue: orgID, KindValue: ragmodel.RAGSourceKindWebsite,
		Name: "src-" + uuid.NewString()[:8], Status: ragmodel.RAGSourceStatusActive, Enabled: true,
		ConfigValue: model.JSON{"url": "https://x.example"},
	}
	if err := db.Create(&src).Error; err != nil {
		t.Fatalf("seed source: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", src.ID).Delete(&ragmodel.RAGSource{}) })
	return src.ID
}

func TestEnablePlugin_IdempotentAndPredicate(t *testing.T) {
	ctx := context.Background()
	db := connectDB(t)
	fx := seedFixture(t, db)
	pluginID := seedPlugin(t, db, nil, &fx.org.ID) // global, installed

	if err := teamprovision.EnablePlugin(ctx, db, fx.org.ID, fx.team.ID, pluginID, nil); err != nil {
		t.Fatalf("enable: %v", err)
	}
	// Idempotent second enable.
	if err := teamprovision.EnablePlugin(ctx, db, fx.org.ID, fx.team.ID, pluginID, nil); err != nil {
		t.Fatalf("enable twice: %v", err)
	}
	var count int64
	db.Model(&model.TeamPlugin{}).Where("team_id = ? AND plugin_id = ?", fx.team.ID, pluginID).Count(&count)
	if count != 1 {
		t.Fatalf("team_plugins rows=%d, want 1", count)
	}
	ok, err := teamprovision.PluginEnabledForTeam(ctx, db, fx.team.ID, pluginID)
	if err != nil || !ok {
		t.Fatalf("PluginEnabledForTeam=%v err=%v, want true", ok, err)
	}
	ids, err := teamprovision.EnabledPluginIDs(ctx, db, fx.team.ID)
	if err != nil || len(ids) != 1 || ids[0] != pluginID {
		t.Fatalf("EnabledPluginIDs=%v err=%v", ids, err)
	}

	// Disable is idempotent and clears the predicate.
	if err := teamprovision.DisablePlugin(ctx, db, fx.org.ID, fx.team.ID, pluginID); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if err := teamprovision.DisablePlugin(ctx, db, fx.org.ID, fx.team.ID, pluginID); err != nil {
		t.Fatalf("disable twice: %v", err)
	}
	if ok, _ := teamprovision.PluginEnabledForTeam(ctx, db, fx.team.ID, pluginID); ok {
		t.Fatalf("PluginEnabledForTeam after disable = true, want false")
	}
}

func TestEnablePlugin_Errors(t *testing.T) {
	ctx := context.Background()
	db := connectDB(t)
	fx := seedFixture(t, db)

	installed := seedPlugin(t, db, nil, &fx.org.ID)
	notInstalled := seedPlugin(t, db, nil, nil)
	otherOrgPlugin := seedPlugin(t, db, &fx.otherOrg.ID, &fx.otherOrg.ID)

	// Team not in org (using otherOrg as the org for our team).
	if err := teamprovision.EnablePlugin(ctx, db, fx.otherOrg.ID, fx.team.ID, installed, nil); !errors.Is(err, teamprovision.ErrTeamNotFound) {
		t.Fatalf("cross-org team: err=%v, want ErrTeamNotFound", err)
	}
	// Plugin owned by another org is invisible.
	if err := teamprovision.EnablePlugin(ctx, db, fx.org.ID, fx.team.ID, otherOrgPlugin, nil); !errors.Is(err, teamprovision.ErrPluginNotFound) {
		t.Fatalf("cross-org plugin: err=%v, want ErrPluginNotFound", err)
	}
	// Unknown plugin id.
	if err := teamprovision.EnablePlugin(ctx, db, fx.org.ID, fx.team.ID, uuid.New(), nil); !errors.Is(err, teamprovision.ErrPluginNotFound) {
		t.Fatalf("unknown plugin: err=%v, want ErrPluginNotFound", err)
	}
	// Visible but not installed.
	if err := teamprovision.EnablePlugin(ctx, db, fx.org.ID, fx.team.ID, notInstalled, nil); !errors.Is(err, teamprovision.ErrPluginNotInstalled) {
		t.Fatalf("not installed: err=%v, want ErrPluginNotInstalled", err)
	}
}

func TestGrantSource_IdempotentAndErrors(t *testing.T) {
	ctx := context.Background()
	db := connectDB(t)
	fx := seedFixture(t, db)
	srcID := seedSource(t, db, fx.org.ID)
	foreign := seedSource(t, db, fx.otherOrg.ID)

	if err := teamprovision.GrantSource(ctx, db, fx.org.ID, fx.team.ID, srcID, nil); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := teamprovision.GrantSource(ctx, db, fx.org.ID, fx.team.ID, srcID, nil); err != nil {
		t.Fatalf("grant twice: %v", err)
	}
	var count int64
	db.Model(&model.TeamRagSource{}).Where("team_id = ? AND rag_source_id = ?", fx.team.ID, srcID).Count(&count)
	if count != 1 {
		t.Fatalf("team_rag_sources rows=%d, want 1", count)
	}
	if ok, _ := teamprovision.SourceGrantedToTeam(ctx, db, fx.team.ID, srcID); !ok {
		t.Fatalf("SourceGrantedToTeam=false, want true")
	}
	got, err := teamprovision.UsableSourceIDsForTeams(ctx, db, fx.org.ID, []uuid.UUID{fx.team.ID})
	if err != nil || len(got) != 1 || got[0] != srcID {
		t.Fatalf("UsableSourceIDsForTeams=%v err=%v", got, err)
	}

	// Cross-org source is rejected.
	if err := teamprovision.GrantSource(ctx, db, fx.org.ID, fx.team.ID, foreign, nil); !errors.Is(err, teamprovision.ErrSourceNotFound) {
		t.Fatalf("cross-org source: err=%v, want ErrSourceNotFound", err)
	}
	// Team not in org.
	if err := teamprovision.GrantSource(ctx, db, fx.otherOrg.ID, fx.team.ID, srcID, nil); !errors.Is(err, teamprovision.ErrTeamNotFound) {
		t.Fatalf("cross-org team: err=%v, want ErrTeamNotFound", err)
	}

	// Revoke is idempotent and clears the predicate.
	if err := teamprovision.RevokeSource(ctx, db, fx.org.ID, fx.team.ID, srcID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if err := teamprovision.RevokeSource(ctx, db, fx.org.ID, fx.team.ID, srcID); err != nil {
		t.Fatalf("revoke twice: %v", err)
	}
	if ok, _ := teamprovision.SourceGrantedToTeam(ctx, db, fx.team.ID, srcID); ok {
		t.Fatalf("SourceGrantedToTeam after revoke = true, want false")
	}
}
