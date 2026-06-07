package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	slacksdk "github.com/slack-go/slack"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/slackgateway"
)

func TestServiceHandleRuntimeFinalSendsSlackConnectionReply(t *testing.T) {
	db := connectGatewayTestDB(t)
	session := seedSlackConnectionSession(t, db)
	nango := &fakeSlackNangoClient{token: "xoxb-test-token"}
	slack := &recordingSlackThreadClient{messageTS: "1780836000.123456"}
	sender := NewSlackNangoResponseSender(nango)
	sender.SetSlackClientFactory(func(token string) slackgateway.ThreadReplyClient {
		slack.token = token
		return slack
	})
	service := NewService(db, nil, nil, NewSlackAdapter(WithSlackResponseSender(sender)))

	delivery, err := service.HandleRuntimeFinal(t.Context(), AgentResponse{
		EmployeeSession:  session,
		RuntimeSessionID: session.RuntimeConversationID,
		TurnID:           "turn-after-wake",
		ChannelID:        "C123",
		ThreadID:         "1780835661.752449",
		Text:             "Specialist finished the report.",
	})
	if err != nil {
		t.Fatalf("handle slack runtime final: %v", err)
	}
	if delivery == nil || delivery.Status != "sent" || delivery.Error != "" {
		t.Fatalf("delivery = %#v, want sent without error", delivery)
	}
	if nango.connectionID != "nango-slack-conn" || nango.providerKey != "slack" {
		t.Fatalf("nango lookup = %q/%q, want nango-slack-conn/slack", nango.connectionID, nango.providerKey)
	}
	if slack.token != "xoxb-test-token" {
		t.Fatalf("slack token = %q, want bot token", slack.token)
	}
	if slack.channelID != "C123" || slack.options != 3 {
		t.Fatalf("slack post = channel %q options %d, want channel C123 with text, block, and thread options", slack.channelID, slack.options)
	}
}

type fakeSlackNangoClient struct {
	token        string
	connectionID string
	providerKey  string
}

func (f *fakeSlackNangoClient) GetConnection(_ context.Context, connectionID, providerKey string) (map[string]any, error) {
	f.connectionID = connectionID
	f.providerKey = providerKey
	return map[string]any{"credentials": map[string]any{"bot_token": f.token}}, nil
}

type recordingSlackThreadClient struct {
	token     string
	channelID string
	options   int
	messageTS string
}

func (c *recordingSlackThreadClient) PostMessageContext(_ context.Context, channelID string, options ...slacksdk.MsgOption) (string, string, error) {
	c.channelID = channelID
	c.options = len(options)
	return channelID, c.messageTS, nil
}

func seedSlackConnectionSession(t *testing.T, db *gorm.DB) model.EmployeeSession {
	t.Helper()
	org := model.Org{Name: "Slack Gateway Test " + uuid.NewString()}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	user := model.User{Email: "slack-gateway-" + uuid.NewString() + "@test.com"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	employee := model.Employee{OrgID: &org.ID, Model: "test-model", Status: "active"}
	if err := db.Create(&employee).Error; err != nil {
		t.Fatalf("create employee: %v", err)
	}
	sandbox := model.Sandbox{
		OrgID:                  &org.ID,
		EmployeeID:             &employee.ID,
		ExternalID:             "slack-gateway-test-" + uuid.NewString(),
		RuntimeURL:             "http://localhost:1",
		EncryptedRuntimeSecret: []byte("test-key"),
		Status:                 "running",
	}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	integration := model.Integration{UniqueKey: "slack", Provider: SlackProvider, DisplayName: "Slack"}
	if err := db.Create(&integration).Error; err != nil {
		t.Fatalf("create integration: %v", err)
	}
	connection := model.Connection{
		OrgID:             org.ID,
		UserID:            user.ID,
		IntegrationID:     integration.ID,
		NangoConnectionID: "nango-slack-conn",
	}
	if err := db.Create(&connection).Error; err != nil {
		t.Fatalf("create connection: %v", err)
	}
	sourceID := connection.ID
	session := model.EmployeeSession{
		OrgID:                 org.ID,
		EmployeeID:            employee.ID,
		SandboxID:             sandbox.ID,
		RuntimeConversationID: "http-gateway-" + uuid.NewString(),
		Source:                Source,
		SourceID:              &sourceID,
		SourceResourceKey:     "C123:1780835661.752449",
		Status:                "active",
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create employee session: %v", err)
	}
	event := model.EmployeeGatewayEvent{
		OrgID:                 org.ID,
		EmployeeID:            employee.ID,
		EmployeeSessionID:     &session.ID,
		Provider:              SlackProvider,
		ExternalMessageID:     "1780835661.752449",
		DedupeKey:             "Ev-slack-test",
		ThreadKey:             "C123:1780835661.752449",
		ChannelID:             "C123",
		ThreadID:              "1780835661.752449",
		Status:                "delivered",
		RuntimeConversationID: session.RuntimeConversationID,
		ReceivedAt:            time.Now().UTC(),
	}
	if err := db.Create(&event).Error; err != nil {
		t.Fatalf("create gateway event: %v", err)
	}
	return session
}
