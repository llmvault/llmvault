package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

// seedGitHubPluginForAgent installs an active GitHub plugin (integrating the
// github-app provider) on the agent so connectionaccess.ResolveAgentProvider
// resolves — i.e. the agent can actually use GitHub tooling.
func seedGitHubPluginForAgent(t *testing.T, db *gorm.DB, orgID, agentID uuid.UUID) {
	t.Helper()
	plugin := model.Plugin{Slug: "github-" + uuid.NewString()[:8], Name: "GitHub", Status: model.PluginStatusActive}
	if err := db.Create(&plugin).Error; err != nil {
		t.Fatalf("create plugin: %v", err)
	}
	if err := db.Create(&model.PluginIntegration{
		PluginID: plugin.ID, Provider: githubAppProvider, Kind: model.PluginIntegrationKindIntegration, Required: true,
	}).Error; err != nil {
		t.Fatalf("create plugin integration: %v", err)
	}
	if err := db.Create(&model.OrgPluginInstall{OrgID: orgID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("create org plugin install: %v", err)
	}
	if err := db.Create(&model.AgentPluginInstall{OrgID: orgID, AgentID: agentID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("create agent plugin install: %v", err)
	}
}

// The split issue/PR mention keys resolve to their own templates, so install
// must store the entity-specific event-key subscription.
func TestTriggerHandlerCreateGitHubMentionSplitKeys(t *testing.T) {
	cases := []struct {
		key       string
		eventKeys []string
	}{
		{model.TriggerKeyGitHubIssueMention, model.GitHubIssueMentionEventKeys},
		{model.TriggerKeyGitHubPRMention, model.GitHubPRMentionEventKeys},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			db := connectNangoSlackTestDB(t)
			org, conn := seedNangoGitHubConnection(t, db)
			agent := seedSlackReactionAgent(t, db, org.ID)
			seedGitHubPluginForAgent(t, db, org.ID, agent.ID)
			body, _ := json.Marshal(createTriggerRequest{
				Name:                 "Test trigger",
				Provider:             githubAppProvider,
				ConnectionID:         conn.ID.String(),
				ExternalResourceKey:  "UseHivy/Hivy",
				ExternalResourceName: "hivy",
				AgentID:              agent.ID.String(),
				TriggerKey:           tc.key,
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
			var row model.AgentTrigger
			if err := db.First(&row, "id = ?", resp["trigger"].ID).Error; err != nil {
				t.Fatalf("load trigger: %v", err)
			}
			if row.TriggerKey != tc.key || len(row.TriggerKeys) != len(tc.eventKeys) {
				t.Fatalf("stored trigger=%+v want key %s keys %v", row, tc.key, tc.eventKeys)
			}
			for i, key := range tc.eventKeys {
				if row.TriggerKeys[i] != key {
					t.Fatalf("trigger_keys=%v want %v", row.TriggerKeys, tc.eventKeys)
				}
			}
		})
	}
}

func TestTriggerHandlerCreateGitHubMentionTrigger(t *testing.T) {
	db := connectNangoSlackTestDB(t)
	org, conn := seedNangoGitHubConnection(t, db)
	agent := seedSlackReactionAgent(t, db, org.ID)
	seedGitHubPluginForAgent(t, db, org.ID, agent.ID)
	body, _ := json.Marshal(createTriggerRequest{
		Name:                 "Test trigger",
		Provider:             githubAppProvider,
		ConnectionID:         conn.ID.String(),
		ExternalResourceKey:  "UseHivy/Hivy",
		ExternalResourceName: "hivy",
		AgentID:              agent.ID.String(),
		TriggerKey:           model.TriggerKeyGitHubIssueMention,
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
	if trigger.TriggerKey != model.TriggerKeyGitHubIssueMention || trigger.TriggerValue != "usehivy/hivy" {
		t.Fatalf("trigger key/value=%q/%q", trigger.TriggerKey, trigger.TriggerValue)
	}
	var row model.AgentTrigger
	if err := db.First(&row, "id = ?", trigger.ID).Error; err != nil {
		t.Fatalf("load trigger: %v", err)
	}
	if row.TriggerType != "webhook" || len(row.TriggerKeys) != len(model.GitHubIssueMentionEventKeys) {
		t.Fatalf("stored trigger=%+v", row)
	}
	for i, key := range model.GitHubIssueMentionEventKeys {
		if row.TriggerKeys[i] != key {
			t.Fatalf("trigger_keys=%v want %v", row.TriggerKeys, model.GitHubIssueMentionEventKeys)
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

func TestTriggerHandlerCreateGitHubMentionRequiresGitHubPlugin(t *testing.T) {
	db := connectNangoSlackTestDB(t)
	org, conn := seedNangoGitHubConnection(t, db)
	agent := seedSlackReactionAgent(t, db, org.ID)
	// No GitHub plugin installed on the agent → the agent could not post a
	// reply, so install must be rejected.
	body, _ := json.Marshal(createTriggerRequest{
		Name:                 "Test trigger",
		Provider:             githubAppProvider,
		ConnectionID:         conn.ID.String(),
		ExternalResourceKey:  "usehivy/hivy",
		ExternalResourceName: "hivy",
		AgentID:              agent.ID.String(),
		TriggerKey:           model.TriggerKeyGitHubIssueMention,
		Instructions:         "Reply with one concise comment.",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/triggers", bytes.NewReader(body))
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()

	NewTriggerHandler(db, WithTriggerExternalProvisioner(&fakeTriggerProvisioner{})).Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if body := rr.Body.String(); !strings.Contains(body, "GitHub plugin") {
		t.Fatalf("expected GitHub plugin message, got %s", body)
	}
	var count int64
	if err := db.Model(&model.AgentTrigger{}).Where("org_id = ?", org.ID).Count(&count).Error; err != nil {
		t.Fatalf("count triggers: %v", err)
	}
	if count != 0 {
		t.Fatalf("created triggers=%d, want 0", count)
	}
}

func TestTriggerHandlerCreateGitHubMentionRejectsUnknownKey(t *testing.T) {
	db := connectNangoSlackTestDB(t)
	org, conn := seedNangoGitHubConnection(t, db)
	agent := seedSlackReactionAgent(t, db, org.ID)
	body, _ := json.Marshal(createTriggerRequest{
		Name:                "Test trigger",
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
