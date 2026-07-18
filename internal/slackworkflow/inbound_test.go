package slackworkflow

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/slackapp"
	"github.com/usehivy/hivy/internal/testdb"
)

func TestClaimInboundAllowsContinuationOnlyAfterHivyReply(t *testing.T) {
	db := connectWorkflowTestDB(t)
	org, conn := seedWorkflowConnection(t, db)
	appMention := slackEvent("Ev1", slackapp.EventAppMention, "1710000000.000000", "")
	claim, err := ClaimInbound(t.Context(), db, org.ID, conn.ID, appMention)
	if err != nil {
		t.Fatalf("claim app mention: %v", err)
	}
	if !claim.Accepted || claim.Duplicate {
		t.Fatalf("app mention claim accepted=%v duplicate=%v", claim.Accepted, claim.Duplicate)
	}
	duplicate, err := ClaimInbound(t.Context(), db, org.ID, conn.ID, appMention)
	if err != nil {
		t.Fatalf("claim duplicate: %v", err)
	}
	if !duplicate.Duplicate {
		t.Fatal("duplicate Slack event was not deduped")
	}

	replyAt, _ := slackapp.ParseTimestamp("1710000002.000000")
	if err := db.Create(&model.SlackThreadEvent{
		OrgID:          org.ID,
		ConnectionID:   conn.ID,
		SlackTeamID:    "T123",
		SlackChannelID: "C123",
		ThreadTS:       appMention.ThreadTS,
		MessageTS:      "1710000002.000000",
		MessageAt:      replyAt,
		EventType:      "app_mention.response",
		Direction:      model.SlackThreadEventDirectionOutbound,
		Status:         model.SlackThreadEventStatusSent,
		ReceivedAt:     time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("seed outbound: %v", err)
	}

	firstReply := slackEvent("Ev2", slackapp.EventMessage, "1710000003.000000", appMention.ThreadTS)
	claim, err = ClaimInbound(t.Context(), db, org.ID, conn.ID, firstReply)
	if err != nil {
		t.Fatalf("claim first continuation: %v", err)
	}
	if !claim.Accepted {
		t.Fatalf("first continuation ignored: %s", claim.Reason)
	}

	secondReply := slackEvent("Ev3", slackapp.EventMessage, "1710000004.000000", appMention.ThreadTS)
	claim, err = ClaimInbound(t.Context(), db, org.ID, conn.ID, secondReply)
	if err != nil {
		t.Fatalf("claim second continuation: %v", err)
	}
	if claim.Accepted || claim.Reason != "prior_human_message_after_hivy_reply" {
		t.Fatalf("second continuation accepted=%v reason=%q", claim.Accepted, claim.Reason)
	}
}

func TestClaimInboundAcceptsEmptyAppMentionForWorkerEnrichment(t *testing.T) {
	db := connectWorkflowTestDB(t)
	org, conn := seedWorkflowConnection(t, db)
	event := slackEvent("EvEmpty", slackapp.EventAppMention, "1710000005.000000", "")
	event.CleanText = ""

	claim, err := ClaimInbound(t.Context(), db, org.ID, conn.ID, event)
	if err != nil {
		t.Fatalf("claim app mention: %v", err)
	}
	if !claim.Accepted || claim.Reason != "" {
		t.Fatalf("empty app mention accepted=%v reason=%q", claim.Accepted, claim.Reason)
	}
}

func TestClaimInboundAcceptsAppMentionWithoutChannelType(t *testing.T) {
	db := connectWorkflowTestDB(t)
	org, conn := seedWorkflowConnection(t, db)
	event := slackEvent("EvNoType", slackapp.EventAppMention, "1710000006.000000", "")
	event.ChannelType = ""

	claim, err := ClaimInbound(t.Context(), db, org.ID, conn.ID, event)
	if err != nil {
		t.Fatalf("claim app mention: %v", err)
	}
	if !claim.Accepted || claim.Reason != "" {
		t.Fatalf("app mention without channel_type accepted=%v reason=%q", claim.Accepted, claim.Reason)
	}
}

func TestClaimInboundRejectsAppMentionFromDMSurface(t *testing.T) {
	db := connectWorkflowTestDB(t)
	org, conn := seedWorkflowConnection(t, db)
	event := slackEvent("EvDM", slackapp.EventAppMention, "1710000007.000000", "")
	event.ChannelType = ""
	event.ChannelID = "D123DMCHAN"

	claim, err := ClaimInbound(t.Context(), db, org.ID, conn.ID, event)
	if err != nil {
		t.Fatalf("claim app mention: %v", err)
	}
	if claim.Accepted || claim.Reason != "unsupported_channel_type" {
		t.Fatalf("dm app mention accepted=%v reason=%q", claim.Accepted, claim.Reason)
	}
}

func slackEvent(eventID, eventType, ts, threadTS string) slackapp.InboundEvent {
	if threadTS == "" {
		threadTS = ts
	}
	messageAt, _ := slackapp.ParseTimestamp(ts)
	return slackapp.InboundEvent{
		TeamID:        "T123",
		EventID:       eventID,
		EventType:     eventType,
		ChannelID:     "C123",
		ChannelType:   "channel",
		UserID:        "U123",
		CleanText:     "hello",
		MessageTS:     ts,
		ThreadTS:      threadTS,
		MessageAt:     messageAt,
		IsThreadReply: threadTS != ts,
		Raw:           map[string]any{"event_id": eventID},
	}
}

func connectWorkflowTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.Open(testdb.DatabaseURL("DATABASE_URL", "HIVY_DATABASE_URL", "TEST_DATABASE_URL")), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	testdb.ApplyMigrations(t, db)
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

func seedWorkflowConnection(t *testing.T, db *gorm.DB) (model.Org, model.Connection) {
	t.Helper()
	org := model.Org{Name: "slack-workflow-" + uuid.NewString()[:8], Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	user := model.User{Email: "slack-workflow-" + uuid.NewString() + "@example.com"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	integ := model.Integration{UniqueKey: "slack-" + uuid.NewString(), Provider: slackapp.Provider, DisplayName: "Slack"}
	if err := db.Create(&integ).Error; err != nil {
		t.Fatalf("create integration: %v", err)
	}
	conn := model.Connection{
		OrgID:             org.ID,
		UserID:            user.ID,
		IntegrationID:     integ.ID,
		NangoConnectionID: "slack-conn-" + uuid.NewString(),
		Meta:              model.JSON{},
	}
	if err := db.Create(&conn).Error; err != nil {
		t.Fatalf("create connection: %v", err)
	}
	return org, conn
}
