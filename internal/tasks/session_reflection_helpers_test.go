package tasks

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/runtimeevents"
	"github.com/usehivy/hivy/internal/trigger/hivy"
)

func reflectionTestHandler(db *gorm.DB, enq *enqueue.MockClient, mock *hivy.MockCompletionClient, now time.Time) *SessionReflectionHandler {
	handler := &SessionReflectionHandler{
		db:           db,
		enqueuer:     enq,
		reg:          registry.Global(),
		cacheManager: &cache.Manager{},
		loadCredential: func(context.Context, *gorm.DB, *cache.Manager, *registry.Registry) (*reflectionCredential, error) {
			return &reflectionCredential{
				credential:  &model.Credential{ProviderID: reflectionDefaultProviderID},
				apiKey:      "sk-test",
				modelID:     "openai/gpt-5-mini",
				temperature: reflectionDefaultTemperature,
			}, nil
		},
		newCompletionClient: func(*model.Credential, string) hivy.CompletionClient { return mock },
		now:                 func() time.Time { return now },
	}
	return handler
}

func seedReflectionFixture(t *testing.T, db *gorm.DB, eventAt time.Time) reflectionFixture {
	t.Helper()
	org := model.Org{Name: "reflection-org-" + uuid.NewString()[:8], Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	user := model.User{Email: "reflection-" + uuid.NewString()[:8] + "@example.com", Name: "Dana"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	agent := model.Agent{
		OrgID:         &org.ID,
		Name:          "reflection-agent-" + uuid.NewString()[:8],
		Model:         "deepseek-v4-flash",
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
	channel := model.Channel{
		OrgID:          org.ID,
		Name:           "reflection-" + uuid.NewString()[:8],
		Kind:           "standard",
		Visibility:     "public",
		DefaultAgentID: agent.ID,
		Origin:         "native",
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	session := model.Session{
		OrgID:             org.ID,
		ChannelID:         channel.ID,
		AgentID:           agent.ID,
		CreatedBy:         &user.ID,
		Model:             agent.Model,
		ReasoningEffort:   "high",
		Source:            model.SessionSourceWeb,
		SourceResourceKey: uuid.NewString(),
		Name:              "reflection",
		Status:            "active",
		AgentTurnStatus:   model.SessionAgentTurnIdle,
		IntegrationScopes: model.JSON{},
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	fx := reflectionFixture{user: user, agent: agent, channel: channel, session: session}
	fx.event = insertReflectionEvent(t, db, fx, eventAt, "My name is Dana. Please keep implementation plans concise.")
	return fx
}

func insertReflectionEvent(t *testing.T, db *gorm.DB, fx reflectionFixture, eventAt time.Time, text string) model.SessionEvent {
	t.Helper()
	event := model.SessionEvent{
		OrgID:            fx.session.OrgID,
		SessionID:        fx.session.ID,
		AgentID:          fx.agent.ID,
		RuntimeSessionID: fx.session.ID.String(),
		EventID:          "reflection-" + uuid.NewString(),
		EventType:        runtimeevents.EventUserMessageReceived,
		ActorUserID:      &fx.user.ID,
		Source:           model.SessionSourceWeb,
		SequenceNumber:   eventAt.UnixNano(),
		Durability:       "durable",
		Payload:          model.JSON{"text": text},
		EventAt:          eventAt,
	}
	if err := db.Create(&event).Error; err != nil {
		t.Fatalf("create event: %v", err)
	}
	return event
}
