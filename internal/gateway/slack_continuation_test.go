package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func TestSlackConnectionContinuationRequiresHivyAsPreviousMessage(t *testing.T) {
	db := connectGatewayTestDB(t)
	conn, orgID, employeeID := seedSlackConnectionGateway(t, db)
	runtime := &recordingRuntime{}
	service := NewService(db, runtime, nil, NewSlackAdapter())
	var createdSessions []model.EmployeeSession
	service.SetSessionCreatedHook(func(_ context.Context, session model.EmployeeSession, _, _ string) {
		createdSessions = append(createdSessions, session)
	})
	envelope := func(body []byte) WebhookEnvelope {
		return WebhookEnvelope{
			ConnectionID: conn.ID,
			OrgID:        orgID,
			EmployeeID:   employeeID,
			Provider:     SlackProvider,
			Body:         body,
		}
	}

	first, err := service.ReceiveWebhookFromConnection(t.Context(), envelope(slackEventBody("Ev1", "app_mention", "100.000", "", "U1", "<@BOT> what are today's metrics?")))
	if err != nil {
		t.Fatalf("receive mention: %v", err)
	}
	if first == nil || first.Ignored || first.Session.ID == uuid.Nil {
		t.Fatalf("mention should create session: %#v", first)
	}

	beforeDelivery, err := service.ReceiveWebhookFromConnection(t.Context(), envelope(slackEventBody("Ev2", "message", "101.000", "100.000", "U1", "hello?")))
	if err != nil {
		t.Fatalf("receive reply before delivery: %v", err)
	}
	if beforeDelivery == nil || !beforeDelivery.Ignored || beforeDelivery.IgnoreReason != "slack_thread_no_hivy_delivery" {
		t.Fatalf("reply before Hivy delivery should be ignored: %#v", beforeDelivery)
	}

	seedSlackDelivery(t, db, first.Session, "100.500")
	accepted, err := service.ReceiveWebhookFromConnection(t.Context(), envelope(slackEventBody("Ev3", "message", "101.000", "100.000", "U1", "can you expand?")))
	if err != nil {
		t.Fatalf("receive accepted reply: %v", err)
	}
	if accepted == nil || accepted.Ignored || accepted.Session.ID != first.Session.ID {
		t.Fatalf("reply after Hivy should continue session: %#v", accepted)
	}

	secondHuman, err := service.ReceiveWebhookFromConnection(t.Context(), envelope(slackEventBody("Ev4", "message", "102.000", "100.000", "U2", "what about revenue?")))
	if err != nil {
		t.Fatalf("receive second human reply: %v", err)
	}
	if secondHuman == nil || !secondHuman.Ignored || secondHuman.IgnoreReason != "slack_hivy_not_latest" {
		t.Fatalf("second human reply before Hivy responds should be ignored: %#v", secondHuman)
	}

	sent := runtime.Sent()
	if len(sent) != 2 {
		t.Fatalf("runtime sends = %d, want mention + one accepted continuation", len(sent))
	}
	if sent[0].ConversationID != sent[1].ConversationID {
		t.Fatalf("continuation should use existing runtime conversation")
	}
	if len(createdSessions) != 1 || createdSessions[0].ID != first.Session.ID {
		t.Fatalf("session-created hook calls = %#v, want first session only", createdSessions)
	}
}

func seedSlackConnectionGateway(t *testing.T, db *gorm.DB) (model.Connection, uuid.UUID, uuid.UUID) {
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
	integration := model.Integration{
		UniqueKey:   "slack-" + uuid.NewString(),
		Provider:    SlackProvider,
		DisplayName: "Slack",
		OrgID:       &org.ID,
	}
	if err := db.Create(&integration).Error; err != nil {
		t.Fatalf("create integration: %v", err)
	}
	conn := model.Connection{
		OrgID:             org.ID,
		UserID:            user.ID,
		IntegrationID:     integration.ID,
		Integration:       integration,
		NangoConnectionID: "nango-slack-" + uuid.NewString(),
	}
	if err := db.Create(&conn).Error; err != nil {
		t.Fatalf("create connection: %v", err)
	}
	conn.Integration = integration
	return conn, org.ID, employee.ID
}

func slackEventBody(eventID, eventType, ts, threadTS, user, text string) []byte {
	fields := []string{
		fmt.Sprintf(`"type":%q`, eventType),
		`"channel":"C123"`,
		`"channel_type":"channel"`,
		fmt.Sprintf(`"user":%q`, user),
		fmt.Sprintf(`"text":%q`, text),
		fmt.Sprintf(`"ts":%q`, ts),
	}
	if threadTS != "" {
		fields = append(fields, fmt.Sprintf(`"thread_ts":%q`, threadTS))
	}
	body := `{"type":"event_callback","team_id":"T123","event_id":` + fmt.Sprintf("%q", eventID) + `,"event":{` + strings.Join(fields, ",") + `}}`
	return []byte(body)
}

func seedSlackDelivery(t *testing.T, db *gorm.DB, session model.EmployeeSession, slackTS string) {
	t.Helper()
	handles, err := json.Marshal([]MessageHandle{{
		ProviderMessageID: slackTS,
		ChannelID:         "C123",
		ThreadID:          "100.000",
		Raw: map[string]any{
			"slack_ts": slackTS,
		},
	}})
	if err != nil {
		t.Fatalf("marshal handles: %v", err)
	}
	delivery := model.EmployeeGatewayDelivery{
		OrgID:             session.OrgID,
		EmployeeID:        session.EmployeeID,
		EmployeeSessionID: session.ID,
		Provider:          SlackProvider,
		DedupeKey:         "delivery-" + slackTS,
		ThreadKey:         "C123:100.000",
		ChannelID:         "C123",
		ThreadID:          "100.000",
		ResponseText:      "Here are the metrics.",
		ProviderHandles:   model.RawJSON(handles),
		Status:            "sent",
	}
	if err := db.Create(&delivery).Error; err != nil {
		t.Fatalf("create delivery: %v", err)
	}
}
