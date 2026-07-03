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
)

func seedNangoGitHubConnection(t *testing.T, db *gorm.DB) (model.Org, model.Connection) {
	t.Helper()
	org := model.Org{Name: "nango-github-" + uuid.NewString()[:8], Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	user := model.User{Email: "nango-github-" + uuid.NewString() + "@example.com"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	integ := model.Integration{UniqueKey: "github-app-" + uuid.NewString(), Provider: githubAppProvider, DisplayName: "GitHub"}
	if err := db.Create(&integ).Error; err != nil {
		t.Fatalf("create integration: %v", err)
	}
	conn := model.Connection{
		OrgID:             org.ID,
		UserID:            user.ID,
		IntegrationID:     integ.ID,
		NangoConnectionID: "nango-github-" + uuid.NewString(),
		Meta:              model.JSON{},
	}
	if err := db.Create(&conn).Error; err != nil {
		t.Fatalf("create connection: %v", err)
	}
	return org, conn
}

func TestTriggerHandlerCreateGitHubMentionTrigger(t *testing.T) {
	db := connectNangoSlackTestDB(t)
	org, conn := seedNangoGitHubConnection(t, db)
	agent := seedSlackReactionAgent(t, db, org.ID)
	body, _ := json.Marshal(createTriggerRequest{
		Provider:             githubAppProvider,
		ConnectionID:         conn.ID.String(),
		ExternalResourceKey:  "UseHivy/Hivy",
		ExternalResourceName: "hivy",
		AgentID:              agent.ID.String(),
		TriggerKey:           model.TriggerKeyGitHubMention,
		Instructions:         "Reply with one concise comment.",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/triggers", bytes.NewReader(body))
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()

	NewTriggerHandler(db, WithTriggerExternalProvisioner(&fakeTriggerProvisioner{})).Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]agentTriggerResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	trigger := resp["trigger"]
	if trigger.TriggerKey != model.TriggerKeyGitHubMention || trigger.TriggerValue != "usehivy/hivy" {
		t.Fatalf("trigger key/value=%q/%q", trigger.TriggerKey, trigger.TriggerValue)
	}
	var row model.AgentTrigger
	if err := db.First(&row, "id = ?", trigger.ID).Error; err != nil {
		t.Fatalf("load trigger: %v", err)
	}
	if row.TriggerType != "webhook" || len(row.TriggerKeys) != len(model.GitHubMentionEventKeys) {
		t.Fatalf("stored trigger=%+v", row)
	}
	for i, key := range model.GitHubMentionEventKeys {
		if row.TriggerKeys[i] != key {
			t.Fatalf("trigger_keys=%v want %v", row.TriggerKeys, model.GitHubMentionEventKeys)
		}
	}
	var channel model.Channel
	if err := db.First(&channel, "id = ?", *row.ChannelID).Error; err != nil {
		t.Fatalf("load created channel: %v", err)
	}
	if channel.ExternalResourceType != "github_repo" || channel.ExternalResourceKey != "UseHivy/Hivy" ||
		channel.ExternalProvider != githubAppProvider || channel.DefaultAgentID != agent.ID {
		t.Fatalf("created channel=%+v", channel)
	}

	// The same repo + agent installs once: the partial unique index on
	// (agent_id, source_slug) turns a duplicate into a 409.
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/triggers", bytes.NewReader(body))
	req = middleware.WithOrg(req, &org)
	NewTriggerHandler(db, WithTriggerExternalProvisioner(&fakeTriggerProvisioner{})).Create(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("duplicate status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestTriggerHandlerCreateGitHubMentionRejectsUnknownKey(t *testing.T) {
	db := connectNangoSlackTestDB(t)
	org, conn := seedNangoGitHubConnection(t, db)
	agent := seedSlackReactionAgent(t, db, org.ID)
	body, _ := json.Marshal(createTriggerRequest{
		Provider:            githubAppProvider,
		ConnectionID:        conn.ID.String(),
		ExternalResourceKey: "usehivy/hivy",
		AgentID:             agent.ID.String(),
		TriggerKey:          "reaction_added",
		Instructions:        "x",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/triggers", bytes.NewReader(body))
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()

	NewTriggerHandler(db).Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}
