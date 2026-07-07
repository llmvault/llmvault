package channelagents_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/channelagents"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

type fixture struct {
	org     model.Org
	agent   model.Agent
	other   model.Agent
	channel model.Channel
	system  model.Channel
}

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

func seed(t *testing.T, db *gorm.DB) fixture {
	t.Helper()
	org := model.Org{ID: uuid.New(), Name: "ca-" + uuid.NewString()[:8], Active: true, RateLimit: 1000}
	agent := model.Agent{ID: uuid.New(), OrgID: &org.ID, Name: "Agent " + uuid.NewString()[:8], Model: "test", Status: "active"}
	other := model.Agent{ID: uuid.New(), OrgID: &org.ID, Name: "Other " + uuid.NewString()[:8], Model: "test", Status: "active"}
	channel := model.Channel{ID: uuid.New(), OrgID: org.ID, Name: "work-" + uuid.NewString()[:8], Kind: "standard", DefaultAgentID: agent.ID}
	system := model.Channel{ID: uuid.New(), OrgID: org.ID, Name: "system-" + uuid.NewString()[:8], Kind: "system", DefaultAgentID: agent.ID}
	for _, obj := range []any{&org, &agent, &other, &channel, &system} {
		if err := db.Create(obj).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.ChannelAgent{})
		db.Where("org_id = ?", org.ID).Delete(&model.Session{})
		db.Where("org_id = ?", org.ID).Delete(&model.Channel{})
		db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		db.Delete(&model.Org{}, "id = ?", org.ID)
	})
	return fixture{org: org, agent: agent, other: other, channel: channel, system: system}
}

func TestAssignedTrueFalseAndSystemExemption(t *testing.T) {
	db := connect(t)
	fx := seed(t, db)

	// No assignment yet: not assigned in a standard channel.
	if ok, err := channelagents.Assigned(t.Context(), db, fx.org.ID, fx.channel.ID, fx.agent.ID); err != nil || ok {
		t.Fatalf("unassigned Assigned = %v, %v; want false, nil", ok, err)
	}
	// System channel is exempt: any org agent may run there.
	if ok, err := channelagents.Assigned(t.Context(), db, fx.org.ID, fx.system.ID, fx.other.ID); err != nil || !ok {
		t.Fatalf("system Assigned = %v, %v; want true, nil", ok, err)
	}
	// After assigning, true.
	if err := channelagents.Assign(t.Context(), db, fx.org.ID, fx.channel.ID, fx.agent.ID, nil); err != nil {
		t.Fatalf("assign: %v", err)
	}
	if ok, err := channelagents.Assigned(t.Context(), db, fx.org.ID, fx.channel.ID, fx.agent.ID); err != nil || !ok {
		t.Fatalf("assigned Assigned = %v, %v; want true, nil", ok, err)
	}
	// A different agent stays unassigned in the standard channel.
	if ok, err := channelagents.Assigned(t.Context(), db, fx.org.ID, fx.channel.ID, fx.other.ID); err != nil || ok {
		t.Fatalf("other Assigned = %v, %v; want false, nil", ok, err)
	}
}

func TestAssignIsIdempotent(t *testing.T) {
	db := connect(t)
	fx := seed(t, db)
	for i := 0; i < 3; i++ {
		if err := channelagents.Assign(t.Context(), db, fx.org.ID, fx.channel.ID, fx.agent.ID, nil); err != nil {
			t.Fatalf("assign %d: %v", i, err)
		}
	}
	var count int64
	if err := db.Model(&model.ChannelAgent{}).
		Where("channel_id = ? AND agent_id = ?", fx.channel.ID, fx.agent.ID).
		Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("assignment rows = %d, want 1", count)
	}
}

func TestAssignRejectsArchivedAgent(t *testing.T) {
	db := connect(t)
	fx := seed(t, db)
	if err := db.Model(&model.Agent{}).Where("id = ?", fx.other.ID).Update("status", "archived").Error; err != nil {
		t.Fatalf("archive agent: %v", err)
	}
	if err := channelagents.Assign(t.Context(), db, fx.org.ID, fx.channel.ID, fx.other.ID, nil); !errors.Is(err, channelagents.ErrAgentArchived) {
		t.Fatalf("assign archived = %v, want ErrAgentArchived", err)
	}
}

func TestUnassignRejectsDefaultAgent(t *testing.T) {
	db := connect(t)
	fx := seed(t, db)
	if err := channelagents.Assign(t.Context(), db, fx.org.ID, fx.channel.ID, fx.agent.ID, nil); err != nil {
		t.Fatalf("assign default: %v", err)
	}
	if err := channelagents.Unassign(t.Context(), db, fx.org.ID, fx.channel.ID, fx.agent.ID); !errors.Is(err, channelagents.ErrDefaultAgent) {
		t.Fatalf("unassign default = %v, want ErrDefaultAgent", err)
	}
	// A non-default agent can be unassigned.
	if err := channelagents.Assign(t.Context(), db, fx.org.ID, fx.channel.ID, fx.other.ID, nil); err != nil {
		t.Fatalf("assign other: %v", err)
	}
	if err := channelagents.Unassign(t.Context(), db, fx.org.ID, fx.channel.ID, fx.other.ID); err != nil {
		t.Fatalf("unassign other: %v", err)
	}
	if ok, _ := channelagents.IsAssignedRow(t.Context(), db, fx.org.ID, fx.channel.ID, fx.other.ID); ok {
		t.Fatalf("other still assigned after unassign")
	}
}

func TestListAssignedStableOrderAndExcludesArchived(t *testing.T) {
	db := connect(t)
	fx := seed(t, db)
	if err := channelagents.Assign(t.Context(), db, fx.org.ID, fx.channel.ID, fx.agent.ID, nil); err != nil {
		t.Fatalf("assign agent: %v", err)
	}
	if err := channelagents.Assign(t.Context(), db, fx.org.ID, fx.channel.ID, fx.other.ID, nil); err != nil {
		t.Fatalf("assign other: %v", err)
	}
	agents, err := channelagents.ListAssigned(t.Context(), db, fx.org.ID, fx.channel.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(agents) != 2 || agents[0].ID != fx.agent.ID || agents[1].ID != fx.other.ID {
		t.Fatalf("list order = %v, want [agent, other] by created_at", agents)
	}
	// Archiving an assigned agent removes it from the list (but not the row).
	if err := db.Model(&model.Agent{}).Where("id = ?", fx.other.ID).Update("status", "archived").Error; err != nil {
		t.Fatalf("archive: %v", err)
	}
	agents, err = channelagents.ListAssigned(t.Context(), db, fx.org.ID, fx.channel.ID)
	if err != nil {
		t.Fatalf("list after archive: %v", err)
	}
	if len(agents) != 1 || agents[0].ID != fx.agent.ID {
		t.Fatalf("list after archive = %v, want [agent]", agents)
	}
}
