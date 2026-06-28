package handler

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/slackapp"
	"github.com/usehivy/hivy/internal/tasks"
	"github.com/usehivy/hivy/internal/testdb"
)

func TestNangoWebhookSlackForwardSetsStatusAndEnqueuesMention(t *testing.T) {
	db := connectNangoSlackTestDB(t)
	org, conn := seedNangoSlackConnection(t, db)
	enq := &enqueue.MockClient{}
	handler := NewNangoWebhookHandler(db, "nango-secret", nil, nil, enq)
	handler.slackLoadBotToken = func(context.Context, model.Connection) (string, error) {
		return "xoxb-test", nil
	}
	var statusChannel, statusThread string
	handler.slackSetStatus = func(_ context.Context, _ string, channelID, threadTS string) error {
		statusChannel = channelID
		statusThread = threadTS
		return nil
	}

	body := nangoSlackWebhookBody(t, conn.NangoConnectionID, map[string]any{
		"type":     "event_callback",
		"team_id":  "T123",
		"event_id": "Ev123",
		"event": map[string]any{
			"type":    "app_mention",
			"channel": "C123",
			"user":    "U123",
			"text":    "<@B123> ship it",
			"ts":      "1710000000.123456",
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/internal/webhooks/nango", bytes.NewReader(body))
	req.Header.Set("X-Nango-Hmac-Sha256", signNangoTestBody(body, "nango-secret"))
	rr := httptest.NewRecorder()

	handler.Handle(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if statusChannel != "C123" || statusThread != "1710000000.123456" {
		t.Fatalf("status channel/thread=%q/%q", statusChannel, statusThread)
	}
	enqueued := enq.Tasks()
	if len(enqueued) != 1 || enqueued[0].TypeName != tasks.TypeSlackAppMention {
		t.Fatalf("enqueued tasks=%+v", enqueued)
	}
	var row model.SlackThreadEvent
	if err := db.First(&row, "org_id = ? AND event_id = ?", org.ID, "Ev123").Error; err != nil {
		t.Fatalf("load slack event: %v", err)
	}
	if row.Status != model.SlackThreadEventStatusEnqueued || row.StatusSetAt == nil || row.EnqueuedAt == nil {
		t.Fatalf("row status=%s status_set=%v enqueued=%v", row.Status, row.StatusSetAt, row.EnqueuedAt)
	}
}

func TestNangoWebhookSlackReactionEnqueuesTrigger(t *testing.T) {
	db := connectNangoSlackTestDB(t)
	org, conn := seedNangoSlackConnection(t, db)
	agent := seedSlackReactionAgent(t, db, org.ID)
	channel := seedSlackReactionChannel(t, db, org.ID, conn, agent)
	trigger := model.AgentTrigger{
		OrgID:        org.ID,
		AgentID:      agent.ID,
		TriggerType:  "webhook",
		ChannelID:    &channel.ID,
		ConnectionID: &conn.ID,
		TriggerKeys:  pq.StringArray{slackapp.EventReactionAdded},
		TriggerKey:   slackapp.EventReactionAdded,
		TriggerValue: "eyes",
		Enabled:      true,
		Instructions: "Summarize the reacted message.",
	}
	if err := db.Create(&trigger).Error; err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	enq := &enqueue.MockClient{}
	handler := NewNangoWebhookHandler(db, "nango-secret", nil, nil, enq)

	body := nangoSlackWebhookBody(t, conn.NangoConnectionID, map[string]any{
		"type":       "event_callback",
		"team_id":    "T111",
		"event_id":   "EvReaction111",
		"event_time": 1599616881,
		"event": map[string]any{
			"type":      "reaction_added",
			"user":      "W111",
			"reaction":  "eyes",
			"item":      map[string]any{"type": "message", "channel": "C111", "ts": "1599529504.000400"},
			"item_user": "W222",
			"event_ts":  "1599616881.000800",
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/internal/webhooks/nango", bytes.NewReader(body))
	req.Header.Set("X-Nango-Hmac-Sha256", signNangoTestBody(body, "nango-secret"))
	rr := httptest.NewRecorder()

	handler.Handle(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	enqueued := enq.Tasks()
	if len(enqueued) != 1 || enqueued[0].TypeName != tasks.TypeSlackReactionTrigger {
		t.Fatalf("enqueued tasks=%+v", enqueued)
	}
	var row model.SlackThreadEvent
	if err := db.First(&row, "org_id = ? AND event_id = ?", org.ID, "EvReaction111").Error; err != nil {
		t.Fatalf("load slack event: %v", err)
	}
	if row.TriggerID == nil || *row.TriggerID != trigger.ID {
		t.Fatalf("trigger_id=%v want %s", row.TriggerID, trigger.ID)
	}
	if row.Status != model.SlackThreadEventStatusEnqueued || row.EventType != slackapp.EventReactionAdded {
		t.Fatalf("row status/event=%s/%s", row.Status, row.EventType)
	}
	if row.ThreadTS != "1599529504.000400" || row.MessageTS != "1599529504.000400" {
		t.Fatalf("row thread/message ts=%q/%q", row.ThreadTS, row.MessageTS)
	}
}

func nangoSlackWebhookBody(t *testing.T, connectionID string, payload map[string]any) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"from":         "slack",
		"type":         "forward",
		"connectionId": connectionID,
		"provider":     slackapp.Provider,
		"payload":      payload,
	})
	if err != nil {
		t.Fatalf("marshal webhook: %v", err)
	}
	return body
}

func signNangoTestBody(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func connectNangoSlackTestDB(t *testing.T) *gorm.DB {
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

func seedNangoSlackConnection(t *testing.T, db *gorm.DB) (model.Org, model.Connection) {
	t.Helper()
	org := model.Org{Name: "nango-slack-" + uuid.NewString()[:8], Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	user := model.User{Email: "nango-slack-" + uuid.NewString() + "@example.com"}
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
		NangoConnectionID: "nango-slack-" + uuid.NewString(),
		Meta:              model.JSON{},
	}
	if err := db.Create(&conn).Error; err != nil {
		t.Fatalf("create connection: %v", err)
	}
	return org, conn
}
