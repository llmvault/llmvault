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

// slackRoutingFixture is a Slack-external channel with two named agents (Ada is
// the default) so routing decisions can be observed end-to-end through
// resolveChannelAndAgent.
type slackRoutingFixture struct {
	org        model.Org
	connection model.Connection
	channel    model.Channel
	ada        model.Agent // default agent
	grace      model.Agent
}

func seedSlackRoutingFixture(t *testing.T, db *gorm.DB) slackRoutingFixture {
	t.Helper()
	org := model.Org{Name: "slack-routing-" + uuid.NewString()[:8], Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	ada := seedNamedAgent(t, db, org.ID, "Ada")
	grace := seedNamedAgent(t, db, org.ID, "Grace")

	user := model.User{Email: "slack-routing-" + uuid.NewString() + "@example.com"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	integration := model.Integration{UniqueKey: "slack-" + uuid.NewString(), Provider: slackapp.Provider, DisplayName: "Slack"}
	if err := db.Create(&integration).Error; err != nil {
		t.Fatalf("create integration: %v", err)
	}
	connection := model.Connection{
		OrgID: org.ID, UserID: user.ID, IntegrationID: integration.ID,
		NangoConnectionID: "slack-conn-" + uuid.NewString(), Meta: model.JSON{},
	}
	if err := db.Create(&connection).Error; err != nil {
		t.Fatalf("create connection: %v", err)
	}
	connID := connection.ID
	channel := model.Channel{
		OrgID: org.ID, Name: "slack-eng-" + uuid.NewString()[:8], Kind: "standard",
		Visibility: "public", DefaultAgentID: ada.ID, Origin: "external",
		ExternalProvider: slackapp.Provider, ExternalConnectionID: &connID,
		ExternalWorkspaceKey: connection.NangoConnectionID, ExternalResourceType: "slack_channel",
		ExternalResourceKey: "C123", ExternalResourceName: "eng", ExternalMetadata: model.JSON{},
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}

	t.Cleanup(func() {
		db.Where("channel_id = ?", channel.ID).Delete(&model.ChannelAgent{})
		db.Where("org_id = ?", org.ID).Delete(&model.Session{})
		db.Where("org_id = ?", org.ID).Delete(&model.SlackThreadEvent{})
		db.Where("id = ?", channel.ID).Delete(&model.Channel{})
		db.Where("id IN ?", []uuid.UUID{ada.ID, grace.ID}).Delete(&model.Agent{})
		db.Where("id = ?", connection.ID).Delete(&model.Connection{})
		db.Where("id = ?", integration.ID).Delete(&model.Integration{})
		db.Where("id = ?", user.ID).Delete(&model.User{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})

	return slackRoutingFixture{org: org, connection: connection, channel: channel, ada: ada, grace: grace}
}

func seedNamedAgent(t *testing.T, db *gorm.DB, orgID uuid.UUID, name string) model.Agent {
	t.Helper()
	agent := model.Agent{
		OrgID: &orgID, Name: name, Model: "test-model",
		Tools: model.JSON{}, McpServers: model.RawJSON("[]"), Skills: model.JSON{},
		RuntimeConfig: model.JSON{}, Permissions: model.JSON{}, Resources: model.JSON{}, Status: "active",
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent %s: %v", name, err)
	}
	return agent
}

func assignAgent(t *testing.T, db *gorm.DB, f slackRoutingFixture, agentID uuid.UUID, createdAt time.Time) {
	t.Helper()
	row := model.ChannelAgent{
		OrgID: f.org.ID, ChannelID: f.channel.ID, AgentID: agentID, CreatedAt: createdAt,
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("assign agent %s: %v", agentID, err)
	}
}

func seedRoutingInbound(t *testing.T, db *gorm.DB, f slackRoutingFixture, text string) model.SlackThreadEvent {
	t.Helper()
	messageAt, _ := slackapp.ParseTimestamp("1782644010.000000")
	row := model.SlackThreadEvent{
		OrgID: f.org.ID, ConnectionID: f.connection.ID, TeamID: "T123", SlackChannelID: "C123",
		ThreadTS: "1782644010.000000", MessageTS: "1782644010.000000", MessageAt: messageAt,
		EventID: "Ev" + uuid.NewString(), EventType: slackapp.EventAppMention,
		Direction: model.SlackThreadEventDirectionInbound, SenderID: "U123", Text: text,
		Status: model.SlackThreadEventStatusReceived, Raw: model.JSON{}, ReceivedAt: time.Now().UTC(),
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	return row
}

func TestResolveChannelAndAgentNameMatchRoutesToNamedAgent(t *testing.T) {
	db := connectTestDB(t)
	f := seedSlackRoutingFixture(t, db)
	base := time.Now().UTC()
	assignAgent(t, db, f, f.ada.ID, base.Add(-2*time.Hour))
	assignAgent(t, db, f, f.grace.ID, base.Add(-1*time.Hour))

	// nil cacheManager: no LLM available. Name match must decide deterministically.
	handler := &SlackAppMentionHandler{db: db}
	inbound := seedRoutingInbound(t, db, f, "hey Grace can you pull the numbers")

	_, agent, err := handler.resolveChannelAndAgent(context.Background(), &inbound, nil, "")
	if err != nil {
		t.Fatalf("resolveChannelAndAgent: %v", err)
	}
	if agent.ID != f.grace.ID {
		t.Fatalf("agent=%s, want Grace %s", agent.ID, f.grace.ID)
	}
}

func TestResolveChannelAndAgentSingleAssignedShortCircuitsToDefault(t *testing.T) {
	db := connectTestDB(t)
	f := seedSlackRoutingFixture(t, db)
	assignAgent(t, db, f, f.ada.ID, time.Now().UTC())

	handler := &SlackAppMentionHandler{db: db}
	// Message even names the OTHER (unassigned) agent — must not matter.
	inbound := seedRoutingInbound(t, db, f, "Grace please help")

	_, agent, err := handler.resolveChannelAndAgent(context.Background(), &inbound, nil, "")
	if err != nil {
		t.Fatalf("resolveChannelAndAgent: %v", err)
	}
	if agent.ID != f.ada.ID {
		t.Fatalf("agent=%s, want default Ada %s", agent.ID, f.ada.ID)
	}
}

func TestResolveChannelAndAgentMultiAgentNoSignalFallsBackToDefault(t *testing.T) {
	db := connectTestDB(t)
	f := seedSlackRoutingFixture(t, db)
	base := time.Now().UTC()
	assignAgent(t, db, f, f.ada.ID, base.Add(-2*time.Hour))
	assignAgent(t, db, f, f.grace.ID, base.Add(-1*time.Hour))

	// No agent named, and no cache manager → LLM unavailable → default agent.
	handler := &SlackAppMentionHandler{db: db}
	inbound := seedRoutingInbound(t, db, f, "can someone look into the deploy failure")

	_, agent, err := handler.resolveChannelAndAgent(context.Background(), &inbound, nil, "")
	if err != nil {
		t.Fatalf("resolveChannelAndAgent: %v", err)
	}
	if agent.ID != f.ada.ID {
		t.Fatalf("agent=%s, want default Ada %s", agent.ID, f.ada.ID)
	}
}

func TestResolveChannelAndAgentThreadAffinityWinsOverNameMatch(t *testing.T) {
	db := connectTestDB(t)
	f := seedSlackRoutingFixture(t, db)
	base := time.Now().UTC()
	assignAgent(t, db, f, f.ada.ID, base.Add(-2*time.Hour))
	assignAgent(t, db, f, f.grace.ID, base.Add(-1*time.Hour))

	// An existing active external session bound to Grace.
	connID := f.connection.ID
	session := model.Session{
		OrgID: f.org.ID, ChannelID: f.channel.ID, AgentID: f.grace.ID, Model: f.grace.Model,
		ReasoningEffort: "high", Source: model.SessionSourceExternal, SourceID: &connID,
		SourceResourceKey: uuid.NewString(), Name: "existing", Status: "active",
		AgentTurnStatus: model.SessionAgentTurnIdle, IntegrationScopes: model.JSON{},
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}

	handler := &SlackAppMentionHandler{db: db}
	// Thread already maps to Grace's session; message names Ada. Affinity wins.
	inbound := seedRoutingInbound(t, db, f, "Ada take a look")
	inbound.SessionID = &session.ID

	_, agent, err := handler.resolveChannelAndAgent(context.Background(), &inbound, nil, "")
	if err != nil {
		t.Fatalf("resolveChannelAndAgent: %v", err)
	}
	if agent.ID != f.grace.ID {
		t.Fatalf("agent=%s, want thread-session agent Grace %s", agent.ID, f.grace.ID)
	}
}
