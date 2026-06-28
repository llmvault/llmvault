package handler

import (
	"bytes"
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

func TestTriggerHandlerCreateSlackReactionTrigger(t *testing.T) {
	db := connectNangoSlackTestDB(t)
	org, conn := seedNangoSlackConnection(t, db)
	agent := seedSlackReactionAgent(t, db, org.ID)
	channel := seedSlackReactionChannel(t, db, org.ID, conn, agent)
	body, _ := json.Marshal(createTriggerRequest{
		Provider:     slackapp.Provider,
		ConnectionID: conn.ID.String(),
		ChannelID:    channel.ID.String(),
		AgentID:      agent.ID.String(),
		TriggerKey:   slackapp.EventReactionAdded,
		TriggerValue: ":eyes:",
		Instructions: "Summarize the reacted message.",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/triggers", bytes.NewReader(body))
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()

	NewTriggerHandler(db).Create(rr, req)

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
	if row.AgentID != agent.ID || row.TriggerValue != "eyes" || row.ChannelID == nil || *row.ChannelID != channel.ID {
		t.Fatalf("stored trigger=%+v", row)
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
