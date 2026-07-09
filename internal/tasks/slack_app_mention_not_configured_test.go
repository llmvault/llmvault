package tasks

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	slacksdk "github.com/slack-go/slack"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/slackapp"
)

type fakeSlackPost struct {
	channel  string
	threadTS string
	text     string
}

type fakeSlackPoster struct {
	posts    []fakeSlackPost
	statuses []string
}

func (f *fakeSlackPoster) PostMessageContext(_ context.Context, channelID string, opts ...slacksdk.MsgOption) (string, string, error) {
	_, values, err := slacksdk.UnsafeApplyMsgOptions("test-token", channelID, "https://slack.com/api/", opts...)
	if err != nil {
		return "", "", err
	}
	f.posts = append(f.posts, fakeSlackPost{
		channel:  channelID,
		threadTS: values.Get("thread_ts"),
		text:     values.Get("text"),
	})
	return channelID, "1782644011.000000", nil
}

func (f *fakeSlackPoster) SetAssistantThreadsStatusContext(_ context.Context, p slacksdk.AssistantThreadsSetStatusParameters) error {
	f.statuses = append(f.statuses, p.Status)
	return nil
}

func (f *fakeSlackPoster) GetConversationInfoContext(context.Context, *slacksdk.GetConversationInfoInput) (*slacksdk.Channel, error) {
	return nil, nil
}

func (f *fakeSlackPoster) GetConversationHistoryContext(context.Context, *slacksdk.GetConversationHistoryParameters) (*slacksdk.GetConversationHistoryResponse, error) {
	return nil, nil
}

func (f *fakeSlackPoster) GetConversationRepliesContext(context.Context, *slacksdk.GetConversationRepliesParameters) ([]slacksdk.Message, bool, string, error) {
	return nil, false, "", nil
}

func (f *fakeSlackPoster) GetReactionsContext(context.Context, slacksdk.ItemRef, slacksdk.GetReactionsParameters) (slacksdk.ReactedItem, error) {
	return slacksdk.ReactedItem{}, nil
}

func seedSlackUnlinkedFixture(t *testing.T, db *gorm.DB) (model.Org, model.Connection) {
	t.Helper()
	org := model.Org{Name: "slack-unlinked-" + uuid.NewString()[:8], Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	user := model.User{Email: "slack-unlinked-" + uuid.NewString() + "@example.com"}
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
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.Session{})
		db.Where("org_id = ?", org.ID).Delete(&model.SlackThreadEvent{})
		db.Where("org_id = ?", org.ID).Delete(&model.Channel{})
		db.Where("id = ?", connection.ID).Delete(&model.Connection{})
		db.Where("id = ?", integration.ID).Delete(&model.Integration{})
		db.Where("id = ?", user.ID).Delete(&model.User{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})
	return org, connection
}

func seedUnlinkedInbound(t *testing.T, db *gorm.DB, org model.Org, connection model.Connection, text string) model.SlackThreadEvent {
	t.Helper()
	messageAt, _ := slackapp.ParseTimestamp("1782644010.000000")
	row := model.SlackThreadEvent{
		OrgID: org.ID, ConnectionID: connection.ID, TeamID: "T999", SlackChannelID: "CUNLINKED",
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

func TestProcessWithSlackUnlinkedChannelRepliesAndStartsNoSession(t *testing.T) {
	db := connectTestDB(t)
	org, connection := seedSlackUnlinkedFixture(t, db)
	row := seedUnlinkedInbound(t, db, org, connection, "hey can you help")

	handler := &SlackAppMentionHandler{db: db}
	fake := &fakeSlackPoster{}

	if err := handler.processWithSlack(context.Background(), &row, "token", fake); err != nil {
		t.Fatalf("processWithSlack: %v", err)
	}

	var channelCount int64
	db.Model(&model.Channel{}).Where("org_id = ?", org.ID).Count(&channelCount)
	if channelCount != 0 {
		t.Fatalf("channel count=%d, want 0 (no team-less channel auto-created)", channelCount)
	}

	var sessionCount int64
	db.Model(&model.Session{}).Where("org_id = ?", org.ID).Count(&sessionCount)
	if sessionCount != 0 {
		t.Fatalf("session count=%d, want 0 (no session started)", sessionCount)
	}

	if len(fake.posts) != 1 {
		t.Fatalf("posts=%d, want 1 not-configured notice", len(fake.posts))
	}
	post := fake.posts[0]
	if post.text != slackChannelNotConfiguredMessage {
		t.Fatalf("notice text=%q, want %q", post.text, slackChannelNotConfiguredMessage)
	}
	if post.channel != row.SlackChannelID || post.threadTS != row.ThreadTS {
		t.Fatalf("notice posted to channel=%q thread=%q, want channel=%q thread=%q",
			post.channel, post.threadTS, row.SlackChannelID, row.ThreadTS)
	}

	var stored model.SlackThreadEvent
	if err := db.First(&stored, "id = ?", row.ID).Error; err != nil {
		t.Fatalf("load inbound: %v", err)
	}
	if stored.Status != model.SlackThreadEventStatusCompleted {
		t.Fatalf("event status=%q, want completed (idempotent, no re-notify)", stored.Status)
	}
	if stored.ChannelID != nil {
		t.Fatalf("event channel_id=%v, want nil", stored.ChannelID)
	}
}

func TestResolveChannelAndAgentUnlinkedReturnsSentinel(t *testing.T) {
	db := connectTestDB(t)
	org, connection := seedSlackUnlinkedFixture(t, db)
	row := seedUnlinkedInbound(t, db, org, connection, "anyone around")

	handler := &SlackAppMentionHandler{db: db}
	fake := &fakeSlackPoster{}

	_, _, err := handler.resolveChannelAndAgent(context.Background(), &row, fake, "token")
	if !errors.Is(err, errSlackChannelNotConfigured) {
		t.Fatalf("err=%v, want errSlackChannelNotConfigured", err)
	}
}

func TestResolveChannelAndAgentLinkedChannelStillResolvesWithoutNotice(t *testing.T) {
	db := connectTestDB(t)
	f := seedSlackRoutingFixture(t, db)
	inbound := seedRoutingInbound(t, db, f, "hey Grace can you pull the numbers")

	handler := &SlackAppMentionHandler{db: db}
	fake := &fakeSlackPoster{}

	channel, agent, err := handler.resolveChannelAndAgent(context.Background(), &inbound, fake, "token")
	if err != nil {
		t.Fatalf("resolveChannelAndAgent: %v", err)
	}
	if channel.ID != f.channel.ID {
		t.Fatalf("channel=%s, want linked channel %s", channel.ID, f.channel.ID)
	}
	if agent.ID != f.grace.ID {
		t.Fatalf("agent=%s, want Grace %s", agent.ID, f.grace.ID)
	}
	if len(fake.posts) != 0 {
		t.Fatalf("posts=%d, want 0 (linked channel must not get the not-configured notice)", len(fake.posts))
	}
}
