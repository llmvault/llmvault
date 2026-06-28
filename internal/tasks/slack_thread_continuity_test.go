package tasks

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/slackapp"
)

func TestSlackThreadContinuationReusesReactionSession(t *testing.T) {
	for _, eventType := range []string{slackapp.EventAppMention, slackapp.EventMessage} {
		t.Run(eventType, func(t *testing.T) {
			db := connectTestDB(t)
			fixture := seedSlackContinuationFixture(t, db)
			inbound := seedSlackContinuationInbound(t, db, fixture, eventType)
			handler := &SlackAppMentionHandler{db: db}

			if err := handler.resolveSlackThreadContinuation(context.Background(), &inbound); err != nil {
				t.Fatalf("resolveSlackThreadContinuation: %v", err)
			}
			if inbound.SessionID == nil || *inbound.SessionID != fixture.session.ID {
				t.Fatalf("session id=%v, want %s", inbound.SessionID, fixture.session.ID)
			}
			if inbound.TriggerID == nil || *inbound.TriggerID != fixture.trigger.ID {
				t.Fatalf("trigger id=%v, want %s", inbound.TriggerID, fixture.trigger.ID)
			}

			var stored model.SlackThreadEvent
			if err := db.First(&stored, "id = ?", inbound.ID).Error; err != nil {
				t.Fatalf("load inbound: %v", err)
			}
			if stored.SessionID == nil || *stored.SessionID != fixture.session.ID {
				t.Fatalf("stored session id=%v, want %s", stored.SessionID, fixture.session.ID)
			}

			channel, agent, err := handler.resolveChannelAndAgent(context.Background(), &inbound, nil, "")
			if err != nil {
				t.Fatalf("resolveChannelAndAgent: %v", err)
			}
			if channel.ID != fixture.channel.ID {
				t.Fatalf("channel=%s, want %s", channel.ID, fixture.channel.ID)
			}
			if agent.ID != fixture.reactionAgent.ID {
				t.Fatalf("agent=%s, want reaction session agent %s", agent.ID, fixture.reactionAgent.ID)
			}

			session, err := handler.findOrCreateSlackSession(context.Background(), &inbound, channel, agent)
			if err != nil {
				t.Fatalf("findOrCreateSlackSession: %v", err)
			}
			if session.ID != fixture.session.ID {
				t.Fatalf("session=%s, want %s", session.ID, fixture.session.ID)
			}
		})
	}
}

type slackContinuationFixture struct {
	org           model.Org
	connection    model.Connection
	channel       model.Channel
	trigger       model.AgentTrigger
	session       model.Session
	reactionAgent model.Agent
}

func seedSlackContinuationFixture(t *testing.T, db *gorm.DB) slackContinuationFixture {
	t.Helper()
	org := model.Org{Name: "slack-continuity-" + uuid.NewString()[:8], Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	defaultAgent := seedSlackContinuationAgent(t, db, org.ID, "default")
	reactionAgent := seedSlackContinuationAgent(t, db, org.ID, "reaction")
	user := model.User{Email: "slack-continuity-" + uuid.NewString() + "@example.com"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	integration := model.Integration{UniqueKey: "slack-" + uuid.NewString(), Provider: slackapp.Provider, DisplayName: "Slack"}
	if err := db.Create(&integration).Error; err != nil {
		t.Fatalf("create integration: %v", err)
	}
	connection := model.Connection{
		OrgID:             org.ID,
		UserID:            user.ID,
		IntegrationID:     integration.ID,
		NangoConnectionID: "slack-conn-" + uuid.NewString(),
		Meta:              model.JSON{},
	}
	if err := db.Create(&connection).Error; err != nil {
		t.Fatalf("create connection: %v", err)
	}
	connID := connection.ID
	channel := model.Channel{
		OrgID:                org.ID,
		Name:                 "slack-incidents-" + uuid.NewString()[:8],
		Kind:                 "standard",
		Visibility:           "public",
		DefaultAgentID:       defaultAgent.ID,
		Origin:               "external",
		ExternalProvider:     slackapp.Provider,
		ExternalConnectionID: &connID,
		ExternalWorkspaceKey: connection.NangoConnectionID,
		ExternalResourceType: "slack_channel",
		ExternalResourceKey:  "C123",
		ExternalResourceName: "incidents",
		ExternalMetadata:     model.JSON{},
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	trigger := model.AgentTrigger{
		ID:           uuid.New(),
		OrgID:        org.ID,
		AgentID:      reactionAgent.ID,
		TriggerType:  "webhook",
		ChannelID:    &channel.ID,
		ConnectionID: &connection.ID,
		TriggerKey:   slackapp.EventReactionAdded,
		TriggerValue: "mag",
		Enabled:      true,
		SourceSlug:   slackapp.Provider,
	}
	if err := db.Create(&trigger).Error; err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	sessionSeed := model.SlackThreadEvent{
		OrgID:          org.ID,
		ConnectionID:   connection.ID,
		TeamID:         "T123",
		SlackChannelID: "C123",
		ThreadTS:       "1782643393.499329",
		TriggerID:      &trigger.ID,
	}
	session := model.Session{
		ID:                stableSlackSessionID(sessionSeed),
		OrgID:             org.ID,
		ChannelID:         channel.ID,
		AgentID:           reactionAgent.ID,
		Model:             reactionAgent.Model,
		ReasoningEffort:   "high",
		Source:            model.SessionSourceExternal,
		SourceID:          &connID,
		SourceResourceKey: slackSessionResourceKey(sessionSeed),
		Name:              "Slack: incidents",
		Status:            "active",
		AgentTurnStatus:   model.SessionAgentTurnIdle,
		IntegrationScopes: model.JSON{},
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	replyAt, _ := slackapp.ParseTimestamp("1782644000.000000")
	outbound := model.SlackThreadEvent{
		OrgID:          org.ID,
		ConnectionID:   connection.ID,
		ChannelID:      &channel.ID,
		TriggerID:      &trigger.ID,
		SessionID:      &session.ID,
		TeamID:         "T123",
		SlackChannelID: "C123",
		ThreadTS:       "1782643393.499329",
		MessageTS:      "1782644000.000000",
		MessageAt:      replyAt,
		EventType:      "reaction_added.response",
		Direction:      model.SlackThreadEventDirectionOutbound,
		SenderID:       "hivy",
		Text:           "investigated",
		Status:         model.SlackThreadEventStatusSent,
		Raw:            model.JSON{},
		ReceivedAt:     time.Now().UTC(),
	}
	if err := db.Create(&outbound).Error; err != nil {
		t.Fatalf("create outbound: %v", err)
	}
	return slackContinuationFixture{
		org:           org,
		connection:    connection,
		channel:       channel,
		trigger:       trigger,
		session:       session,
		reactionAgent: reactionAgent,
	}
}

func seedSlackContinuationAgent(t *testing.T, db *gorm.DB, orgID uuid.UUID, prefix string) model.Agent {
	t.Helper()
	agent := model.Agent{
		OrgID:         &orgID,
		Name:          "slack-" + prefix + "-" + uuid.NewString()[:8],
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
	return agent
}

func seedSlackContinuationInbound(t *testing.T, db *gorm.DB, fixture slackContinuationFixture, eventType string) model.SlackThreadEvent {
	t.Helper()
	messageAt, _ := slackapp.ParseTimestamp("1782644010.000000")
	row := model.SlackThreadEvent{
		OrgID:          fixture.org.ID,
		ConnectionID:   fixture.connection.ID,
		TeamID:         "T123",
		SlackChannelID: "C123",
		ThreadTS:       "1782643393.499329",
		MessageTS:      "1782644010.000000",
		MessageAt:      messageAt,
		EventID:        "Ev" + uuid.NewString(),
		EventType:      eventType,
		Direction:      model.SlackThreadEventDirectionInbound,
		SenderID:       "U123",
		Text:           "following up",
		Status:         model.SlackThreadEventStatusReceived,
		Raw:            model.JSON{},
		ReceivedAt:     time.Now().UTC(),
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	return row
}
