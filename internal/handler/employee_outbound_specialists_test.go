package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestEmployeeOutboundWebhook_SpecialistFinalMarksIdleAndNotifiesParentRuntime(t *testing.T) {
	db := connectEmployeeSkillSyncTestDB(t)
	encKey := outboundWebhookTestSymmetricKey(t)
	var delivered employeeruntimeMessage
	var runtime *httptest.Server
	runtime = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/gateway/http/messages" {
			t.Fatalf("unexpected runtime path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer runtime-secret" {
			t.Fatalf("authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&delivered); err != nil {
			t.Fatalf("decode runtime message: %v", err)
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"session_id":           "runtime-parent-session",
			"stream_id":            "stream-1",
			"response_stream_id":   "response-stream-1",
			"response_stream_url":  runtime.URL + "/stream/response-stream-1",
			"trace_id":             "trace-1",
			"turn_id":              "turn-1",
			"runtime_conversation": "runtime-parent-session",
		})
	}))
	defer runtime.Close()

	org := model.Org{Name: "specialist-webhook-" + uuid.NewString()}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	employee := model.Employee{OrgID: &org.ID, Name: "Hivy", Model: "deepseek-v4-flash", Status: "active"}
	if err := db.Create(&employee).Error; err != nil {
		t.Fatalf("create employee: %v", err)
	}
	encryptedSecret, err := encKey.EncryptString("runtime-secret")
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}
	parentSandbox := model.Sandbox{
		ID:                     uuid.New(),
		OrgID:                  &org.ID,
		EmployeeID:             &employee.ID,
		ExternalID:             "parent-runtime",
		RuntimeURL:             runtime.URL,
		EncryptedRuntimeSecret: encryptedSecret,
		Status:                 "running",
	}
	specialistSandbox := model.Sandbox{
		ID:                     uuid.New(),
		OrgID:                  &org.ID,
		EmployeeID:             &employee.ID,
		ExternalID:             "specialist-runtime",
		RuntimeURL:             "http://specialist-runtime",
		EncryptedRuntimeSecret: encryptedSecret,
		Status:                 "running",
	}
	if err := db.Create(&parentSandbox).Error; err != nil {
		t.Fatalf("create parent sandbox: %v", err)
	}
	if err := db.Create(&specialistSandbox).Error; err != nil {
		t.Fatalf("create specialist sandbox: %v", err)
	}
	parentSession := model.EmployeeSession{
		ID:                    uuid.New(),
		OrgID:                 org.ID,
		EmployeeID:            employee.ID,
		SandboxID:             parentSandbox.ID,
		RuntimeConversationID: "http-runtime-parent-session",
		Source:                "gateway",
		Status:                "active",
	}
	if err := db.Create(&parentSession).Error; err != nil {
		t.Fatalf("create parent session: %v", err)
	}
	task := model.SpecialistTask{
		ID:                     uuid.New(),
		OrgID:                  org.ID,
		EmployeeID:             employee.ID,
		SpecialistSlug:         "software-engineering-specialist",
		EmployeeSessionID:      parentSession.RuntimeConversationID,
		SandboxID:              specialistSandbox.ID,
		ConversationID:         &parentSession.ID,
		ParentConversationType: "employee_session",
		ParentConversationID:   parentSession.RuntimeConversationID,
		Brief:                  "Investigate",
		Status:                 "running",
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("create specialist task: %v", err)
	}
	payload := map[string]any{
		"session_id": "specialist-runtime-session",
		"source":     "specialist",
		"text":       "The specialist finished the investigation.",
		"trace_id":   "specialist-trace",
		"turn_id":    "specialist-turn",
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	h := NewEmployeeOutboundWebhookHandler(db, encKey, nil)
	h.storeAndMaybeEnqueue(t.Context(), &specialistSandbox, &employeeOutboundEvent{
		EventType: "agent.message.sent",
		Payload:   raw,
		At:        time.Now().UTC(),
	})

	if delivered.ConversationID != "runtime-parent-session" {
		t.Fatalf("delivered conversation = %q", delivered.ConversationID)
	}
	if !strings.Contains(delivered.Text, "The specialist finished the investigation.") {
		t.Fatalf("delivered text missing specialist message: %q", delivered.Text)
	}
	if !strings.Contains(delivered.Text, "Files written there are not available in this employee sandbox") {
		t.Fatalf("delivered text missing specialist filesystem reminder: %q", delivered.Text)
	}
	var reloaded model.SpecialistTask
	if err := db.First(&reloaded, "id = ?", task.ID).Error; err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if reloaded.Status != "idle" {
		t.Fatalf("task status = %q", reloaded.Status)
	}
	var stored model.EmployeeSessionEvent
	if err := db.Where("specialist_task_id = ?", task.ID).First(&stored).Error; err != nil {
		t.Fatalf("load stored specialist event: %v", err)
	}
	if stored.EmployeeSessionID != parentSession.ID || stored.Mode != "specialist" {
		t.Fatalf("stored specialist event not linked to parent session: %#v", stored)
	}
}

type employeeruntimeMessage struct {
	Text           string         `json:"text"`
	ConversationID string         `json:"conversation_id"`
	User           string         `json:"user"`
	Raw            map[string]any `json:"raw"`
}
