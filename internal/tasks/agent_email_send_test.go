package tasks

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentemail"
	"github.com/usehivy/hivy/internal/model"
)

type recordingAgentEmailSender struct {
	request agentemail.SendRequest
}

func (s *recordingAgentEmailSender) Send(_ context.Context, request agentemail.SendRequest, _ string) (agentemail.SendResponse, error) {
	s.request = request
	return agentemail.SendResponse{ID: "resend-sent-1"}, nil
}

func (*recordingAgentEmailSender) GetSent(_ context.Context, emailID string) (agentemail.SentEmail, error) {
	return agentemail.SentEmail{
		ID:        emailID,
		MessageID: "<provider-message-1@example.test>",
	}, nil
}

func TestAgentEmailSendStoresProviderMessageIDForReplyReconciliation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	for _, statement := range []string{
		`CREATE TABLE agents (id text PRIMARY KEY, org_id text NOT NULL, status text NOT NULL, email_inbox_local_part text NOT NULL)`,
		`CREATE TABLE agent_email_threads (id text PRIMARY KEY, org_id text NOT NULL, agent_id text NOT NULL, session_id text, root_message_id text NOT NULL, last_message_at datetime NOT NULL, created_at datetime, updated_at datetime)`,
		`CREATE TABLE agent_email_messages (id text PRIMARY KEY, org_id text NOT NULL, agent_id text NOT NULL, thread_id text NOT NULL, direction text NOT NULL, status text NOT NULL, resend_email_id text NOT NULL, message_id text NOT NULL, to_addresses text NOT NULL, cc_addresses text NOT NULL, subject text NOT NULL, text_body text NOT NULL, html_body text NOT NULL, headers text NOT NULL, provider_at datetime NOT NULL, created_at datetime, updated_at datetime)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("create email test table: %v", err)
		}
	}

	orgID, agentID, threadID, messageID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	now := time.Now().UTC()
	if err := db.Exec(`INSERT INTO agents (id, org_id, status, email_inbox_local_part) VALUES (?, ?, ?, ?)`,
		agentID, orgID, "active", "agent-test1234").Error; err != nil {
		t.Fatalf("insert agent: %v", err)
	}
	if err := db.Exec(`INSERT INTO agent_email_threads (id, org_id, agent_id, root_message_id, last_message_at, created_at, updated_at) VALUES (?, ?, ?, '', ?, ?, ?)`,
		threadID, orgID, agentID, now, now, now).Error; err != nil {
		t.Fatalf("insert thread: %v", err)
	}
	if err := db.Exec(`INSERT INTO agent_email_messages (id, org_id, agent_id, thread_id, direction, status, resend_email_id, message_id, to_addresses, cc_addresses, subject, text_body, html_body, headers, provider_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, '', '', ?, '[]', ?, ?, '', '{}', ?, ?, ?)`,
		messageID, orgID, agentID, threadID, model.AgentEmailDirectionOutbound, model.AgentEmailStatusQueued, `["member@example.test"]`, "Status", "Hello", now, now, now).Error; err != nil {
		t.Fatalf("insert message: %v", err)
	}

	task, _, err := NewAgentEmailSendTask(AgentEmailSendPayload{MessageID: messageID})
	if err != nil {
		t.Fatalf("create send task: %v", err)
	}
	sender := &recordingAgentEmailSender{}
	handler := &AgentEmailSendHandler{db: db, client: sender, domain: "agents.example.test"}
	if err := handler.Handle(t.Context(), task); err != nil {
		t.Fatalf("send email: %v", err)
	}

	if sender.request.From != "agent-test1234@agents.example.test" {
		t.Fatalf("from = %q", sender.request.From)
	}
	var stored model.AgentEmailMessage
	if err := db.First(&stored, "id = ?", messageID).Error; err != nil {
		t.Fatalf("load sent message: %v", err)
	}
	if stored.Status != model.AgentEmailStatusSent || stored.ResendEmailID != "resend-sent-1" || stored.MessageID != "<provider-message-1@example.test>" {
		t.Fatalf("stored sent message = %#v", stored)
	}
	var thread model.AgentEmailThread
	if err := db.First(&thread, "id = ?", threadID).Error; err != nil {
		t.Fatalf("load sent thread: %v", err)
	}
	if thread.RootMessageID != stored.MessageID {
		t.Fatalf("thread root message id = %q, want %q", thread.RootMessageID, stored.MessageID)
	}
}
