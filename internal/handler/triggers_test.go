package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/slackapp"
)

type fakeTriggerProvisioner struct {
	calls []ChannelExternalProvisionRequest
	err   error
}

func (f *fakeTriggerProvisioner) EnsureExternalChannel(_ context.Context, _ uuid.UUID, req ChannelExternalProvisionRequest) error {
	f.calls = append(f.calls, req)
	return f.err
}

func TestTriggerHandlerCreateSlackReactionTrigger(t *testing.T) {
	db := connectNangoSlackTestDB(t)
	org, conn := seedNangoSlackConnection(t, db)
	agent := seedSlackReactionAgent(t, db, org.ID)
	provisioner := &fakeTriggerProvisioner{}
	body, _ := json.Marshal(createTriggerRequest{
		Provider:             slackapp.Provider,
		ConnectionID:         conn.ID.String(),
		ExternalResourceKey:  "C111",
		ExternalResourceName: "general",
		AgentID:              agent.ID.String(),
		TriggerKey:           slackapp.EventReactionAdded,
		TriggerValue:         ":eyes:",
		Instructions:         "Summarize the reacted message.",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/triggers", bytes.NewReader(body))
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()

	NewTriggerHandler(db, WithTriggerExternalProvisioner(provisioner)).Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]agentTriggerResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	trigger := resp["trigger"]
	if trigger.TriggerKey != slackapp.EventReactionAdded || trigger.TriggerValue != "eyes" {
		t.Fatalf("trigger key/value=%q/%q", trigger.TriggerKey, trigger.TriggerValue)
	}
	var row model.AgentTrigger
	if err := db.First(&row, "id = ?", trigger.ID).Error; err != nil {
		t.Fatalf("load trigger: %v", err)
	}
	if row.AgentID != agent.ID || row.TriggerValue != "eyes" || row.ChannelID == nil {
		t.Fatalf("stored trigger=%+v", row)
	}
	var channel model.Channel
	if err := db.First(&channel, "id = ?", *row.ChannelID).Error; err != nil {
		t.Fatalf("load created channel: %v", err)
	}
	if channel.Name != "slack-general" || channel.DefaultAgentID != agent.ID ||
		channel.ExternalResourceKey != "C111" || channel.ExternalResourceName != "general" ||
		channel.ExternalConnectionID == nil || *channel.ExternalConnectionID != conn.ID {
		t.Fatalf("created channel=%+v", channel)
	}
	if len(provisioner.calls) != 1 {
		t.Fatalf("provisioner calls=%d, want 1", len(provisioner.calls))
	}
	call := provisioner.calls[0]
	if call.Provider != slackapp.Provider || call.ConnectionID != conn.ID ||
		call.ResourceType != "slack_channel" || call.ResourceKey != "C111" ||
		call.ResourceName != "general" || call.WorkspaceKey != conn.NangoConnectionID {
		t.Fatalf("provisioner call=%+v", call)
	}
}

func TestTriggerHandlerCreateSlackReactionTriggerReusesExternalChannel(t *testing.T) {
	db := connectNangoSlackTestDB(t)
	org, conn := seedNangoSlackConnection(t, db)
	agent := seedSlackReactionAgent(t, db, org.ID)
	channel := seedSlackReactionChannel(t, db, org.ID, conn, agent)
	provisioner := &fakeTriggerProvisioner{}
	body, _ := json.Marshal(createTriggerRequest{
		Provider:             slackapp.Provider,
		ConnectionID:         conn.ID.String(),
		ExternalResourceKey:  channel.ExternalResourceKey,
		ExternalResourceName: channel.ExternalResourceName,
		AgentID:              agent.ID.String(),
		TriggerKey:           slackapp.EventReactionAdded,
		TriggerValue:         "eyes",
		Instructions:         "Summarize the reacted message.",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/triggers", bytes.NewReader(body))
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()

	NewTriggerHandler(db, WithTriggerExternalProvisioner(provisioner)).Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var row model.AgentTrigger
	if err := db.Where("agent_id = ?", agent.ID).First(&row).Error; err != nil {
		t.Fatalf("load trigger: %v", err)
	}
	if row.ChannelID == nil || *row.ChannelID != channel.ID {
		t.Fatalf("trigger channel=%v want %s", row.ChannelID, channel.ID)
	}
	var count int64
	if err := db.Model(&model.Channel{}).
		Where("org_id = ? AND external_resource_key = ?", org.ID, channel.ExternalResourceKey).
		Count(&count).Error; err != nil {
		t.Fatalf("count channels: %v", err)
	}
	if count != 1 {
		t.Fatalf("external channel count=%d want 1", count)
	}
	if len(provisioner.calls) != 0 {
		t.Fatalf("provisioner calls=%d, want 0", len(provisioner.calls))
	}
}

func TestTriggerHandlerCreateSlackReactionTriggerProvisionFailure(t *testing.T) {
	db := connectNangoSlackTestDB(t)
	org, conn := seedNangoSlackConnection(t, db)
	agent := seedSlackReactionAgent(t, db, org.ID)
	provisioner := &fakeTriggerProvisioner{
		err: &ChannelExternalProvisionError{StatusCode: http.StatusBadRequest, Message: "Slack channel is not available"},
	}
	body, _ := json.Marshal(createTriggerRequest{
		Provider:             slackapp.Provider,
		ConnectionID:         conn.ID.String(),
		ExternalResourceKey:  "CFAIL",
		ExternalResourceName: "private-room",
		AgentID:              agent.ID.String(),
		TriggerKey:           slackapp.EventReactionAdded,
		TriggerValue:         "eyes",
		Instructions:         "Summarize the reacted message.",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/triggers", bytes.NewReader(body))
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()

	NewTriggerHandler(db, WithTriggerExternalProvisioner(provisioner)).Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(provisioner.calls) != 1 {
		t.Fatalf("provisioner calls=%d, want 1", len(provisioner.calls))
	}
	var channelCount int64
	if err := db.Model(&model.Channel{}).
		Where("org_id = ? AND external_resource_key = ?", org.ID, "CFAIL").
		Count(&channelCount).Error; err != nil {
		t.Fatalf("count channels: %v", err)
	}
	if channelCount != 0 {
		t.Fatalf("created channels=%d, want 0", channelCount)
	}
	var triggerCount int64
	if err := db.Model(&model.AgentTrigger{}).
		Where("org_id = ? AND trigger_value = ?", org.ID, "eyes").
		Count(&triggerCount).Error; err != nil {
		t.Fatalf("count triggers: %v", err)
	}
	if triggerCount != 0 {
		t.Fatalf("created triggers=%d, want 0", triggerCount)
	}
}

func seedSlackReactionAgent(t *testing.T, db *gorm.DB, orgID uuid.UUID) model.Agent {
	t.Helper()
	agent := model.Agent{
		OrgID:  &orgID,
		Name:   "slack-reaction-agent-" + uuid.NewString(),
		Model:  "gpt-5",
		Status: "active",
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Where("id = ?", agent.ID).Delete(&model.Agent{}).Error
	})
	return agent
}

func seedSlackReactionChannel(t *testing.T, db *gorm.DB, orgID uuid.UUID, conn model.Connection, agent model.Agent) model.Channel {
	t.Helper()
	channel := model.Channel{
		OrgID:                orgID,
		Name:                 "slack-general-" + uuid.NewString(),
		DefaultAgentID:       agent.ID,
		Origin:               "external",
		ExternalProvider:     slackapp.Provider,
		ExternalConnectionID: &conn.ID,
		ExternalWorkspaceKey: conn.NangoConnectionID,
		ExternalResourceType: "slack_channel",
		ExternalResourceKey:  "C111",
		ExternalResourceName: "general",
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Where("id = ?", channel.ID).Delete(&model.Channel{}).Error
	})
	return channel
}
