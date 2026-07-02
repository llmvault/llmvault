package handler

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func TestCreateUserDefaultOrg_CreatesGeneralAndSystemChannels(t *testing.T) {
	db := connectInternalTestDB(t)
	user := seedSignupUser(t, db)

	var org model.Org
	err := db.Transaction(func(tx *gorm.DB) error {
		var e error
		org, e = createUserDefaultOrg(context.Background(), tx, nil, user)
		return e
	})
	if err != nil {
		t.Fatalf("createUserDefaultOrg: %v", err)
	}
	cleanupOrgAndLedger(t, db, org.ID)

	var general model.Channel
	if err := db.Where("org_id = ? AND name = ?", org.ID, defaultChannelName).First(&general).Error; err != nil {
		t.Fatalf("load #general channel: %v", err)
	}
	if !general.IsDefault || general.Kind != "standard" {
		t.Fatalf("#general = %#v", general)
	}

	var system model.Channel
	if err := db.Where("org_id = ? AND name = ?", org.ID, systemChannelName).First(&system).Error; err != nil {
		t.Fatalf("load system channel: %v", err)
	}
	if system.Kind != "system" || system.Visibility != "private" || system.Origin != "native" {
		t.Fatalf("system channel = %#v", system)
	}
	if source, _ := system.ExternalMetadata["source"].(string); source != "system" {
		t.Fatalf("system channel metadata = %v", system.ExternalMetadata)
	}
	// Anchored to the same default agent as #general, not an arbitrary agent.
	if system.DefaultAgentID != general.DefaultAgentID {
		t.Fatalf("system channel default agent = %s, want %s", system.DefaultAgentID, general.DefaultAgentID)
	}
	var defaultAgent model.Agent
	if err := db.Where("org_id = ? AND is_default = true", org.ID).First(&defaultAgent).Error; err != nil {
		t.Fatalf("load default agent: %v", err)
	}
	if system.DefaultAgentID != defaultAgent.ID {
		t.Fatalf("system channel default agent = %s, want default agent %s", system.DefaultAgentID, defaultAgent.ID)
	}
}

func TestCreateSystemChannelTxIsIdempotent(t *testing.T) {
	db := connectInternalTestDB(t)
	user := seedSignupUser(t, db)

	var org model.Org
	err := db.Transaction(func(tx *gorm.DB) error {
		var e error
		org, e = createUserDefaultOrg(context.Background(), tx, nil, user)
		return e
	})
	if err != nil {
		t.Fatalf("createUserDefaultOrg: %v", err)
	}
	cleanupOrgAndLedger(t, db, org.ID)

	var existing model.Channel
	if err := db.Where("org_id = ? AND name = ?", org.ID, systemChannelName).First(&existing).Error; err != nil {
		t.Fatalf("load system channel: %v", err)
	}

	other := model.Agent{ID: uuid.New(), OrgID: &org.ID, Name: "Other " + uuid.NewString(), Model: "test", Status: "active"}
	if err := db.Create(&other).Error; err != nil {
		t.Fatalf("create second agent: %v", err)
	}

	again, err := createSystemChannelTx(context.Background(), db, org.ID, other.ID)
	if err != nil {
		t.Fatalf("createSystemChannelTx second run: %v", err)
	}
	if again.ID != existing.ID {
		t.Fatalf("second run created a new channel %s, want existing %s", again.ID, existing.ID)
	}
	var count int64
	if err := db.Model(&model.Channel{}).Where("org_id = ? AND name = ?", org.ID, systemChannelName).Count(&count).Error; err != nil {
		t.Fatalf("count system channels: %v", err)
	}
	if count != 1 {
		t.Fatalf("system channel count = %d, want 1", count)
	}
	// The original default agent anchor is preserved on conflict.
	var reloaded model.Channel
	if err := db.First(&reloaded, "id = ?", existing.ID).Error; err != nil {
		t.Fatalf("reload system channel: %v", err)
	}
	if reloaded.DefaultAgentID != existing.DefaultAgentID {
		t.Fatalf("default agent changed to %s, want %s", reloaded.DefaultAgentID, existing.DefaultAgentID)
	}
}
