package tasks

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func TestFindOrCreateTriggerSessionRejectsMissingChannel(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	trigger := seedTriggerForSession(t, db, org.ID, agent.ID, nil)
	handler := &AgentTriggerDispatchHandler{db: db}

	// A trigger with no channel is a configuration error — there is no #system
	// fallback anymore, and no system channel must be created.
	if _, err := handler.findOrCreateTriggerSession(context.Background(), &agent, trigger, "github/acme/repo/issues/1"); err == nil {
		t.Fatalf("expected error for trigger with no channel")
	}

	var systemCount int64
	if err := db.Model(&model.Channel{}).Where("org_id = ? AND kind = ?", org.ID, "system").Count(&systemCount).Error; err != nil {
		t.Fatalf("count system channels: %v", err)
	}
	if systemCount != 0 {
		t.Fatalf("system channel count = %d, want 0", systemCount)
	}
}

func TestFindOrCreateTriggerSessionUsesConfiguredChannel(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	firstChannel := seedTriggerChannel(t, db, org.ID, agent.ID, "triage")
	secondChannel := seedTriggerChannel(t, db, org.ID, agent.ID, "incidents")
	trigger := seedTriggerForSession(t, db, org.ID, agent.ID, &firstChannel.ID)
	handler := &AgentTriggerDispatchHandler{db: db}
	resourceKey := "github/acme/repo/pulls/9"

	first, err := handler.findOrCreateTriggerSession(context.Background(), &agent, trigger, resourceKey)
	if err != nil {
		t.Fatalf("find first session: %v", err)
	}
	if first.ChannelID != firstChannel.ID {
		t.Fatalf("channel = %s, want %s", first.ChannelID, firstChannel.ID)
	}

	reused, err := handler.findOrCreateTriggerSession(context.Background(), &agent, trigger, resourceKey)
	if err != nil {
		t.Fatalf("reuse first session: %v", err)
	}
	if reused.ID != first.ID {
		t.Fatalf("reused session = %s, want %s", reused.ID, first.ID)
	}

	trigger.ChannelID = &secondChannel.ID
	second, err := handler.findOrCreateTriggerSession(context.Background(), &agent, trigger, resourceKey)
	if err != nil {
		t.Fatalf("find second session: %v", err)
	}
	if second.ChannelID != secondChannel.ID {
		t.Fatalf("channel = %s, want %s", second.ChannelID, secondChannel.ID)
	}
	if second.ID == first.ID {
		t.Fatalf("same resource reused session across channels: %s", second.ID)
	}
}

func seedTriggerSessionFixture(t *testing.T, db *gorm.DB) (model.Org, model.Agent, model.Sandbox) {
	t.Helper()
	org := model.Org{Name: "trigger-session-" + uuid.NewString()[:8], Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Agent{
		OrgID:         &org.ID,
		Name:          "trigger-agent-" + uuid.NewString()[:8],
		Model:         "test-model",
		Tools:         model.JSON{},
		McpServers:    model.RawJSON("[]"),
		Skills:        model.JSON{},
		RuntimeConfig: model.JSON{},
		Permissions:   model.JSON{},
		Resources:     model.JSON{},
		Status:        "active",
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sandbox := model.Sandbox{
		OrgID:                  &org.ID,
		AgentID:                &agent.ID,
		ProviderID:             "test",
		ExternalID:             "trigger-sandbox-" + uuid.NewString()[:8],
		RuntimeURL:             "http://runtime.test",
		EncryptedRuntimeSecret: []byte("encrypted"),
		Status:                 "running",
	}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Where("org_id = ?", org.ID).Delete(&model.Session{}).Error
		_ = db.Where("org_id = ?", org.ID).Delete(&model.AgentTrigger{}).Error
		_ = db.Where("org_id = ?", org.ID).Delete(&model.Sandbox{}).Error
		_ = db.Where("org_id = ?", org.ID).Delete(&model.AgentChannel{}).Error
		_ = db.Where("org_id = ?", org.ID).Delete(&model.Channel{}).Error
		_ = db.Where("id = ?", agent.ID).Delete(&model.Agent{}).Error
		_ = db.Where("id = ?", org.ID).Delete(&model.Org{}).Error
	})
	return org, agent, sandbox
}

func seedTriggerChannel(t *testing.T, db *gorm.DB, orgID, agentID uuid.UUID, name string) model.Channel {
	t.Helper()
	channel := model.Channel{
		OrgID:          orgID,
		Name:           name + "-" + uuid.NewString()[:8],
		Kind:           "standard",
		Visibility:     "public",
		DefaultAgentID: agentID,
		Origin:         "native",
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	// Team-less channel: any org agent may act in it under the team-primary model.
	return channel
}

func seedTriggerForSession(t *testing.T, db *gorm.DB, orgID, agentID uuid.UUID, channelID *uuid.UUID) model.AgentTrigger {
	t.Helper()
	trigger := model.AgentTrigger{
		ID:          uuid.New(),
		OrgID:       orgID,
		AgentID:     agentID,
		TriggerType: "webhook",
		ChannelID:   channelID,
		Enabled:     true,
	}
	if err := db.Create(&trigger).Error; err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	return trigger
}
