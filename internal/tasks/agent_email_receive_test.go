package tasks

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentemail"
	"github.com/usehivy/hivy/internal/model"
)

func TestAgentEmailReplyResolvesOriginatingSessionFromRFCMessageID(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	for _, statement := range []string{
		`CREATE TABLE agent_email_threads (id text PRIMARY KEY, org_id text NOT NULL, agent_id text NOT NULL, session_id text, root_message_id text NOT NULL, last_message_at datetime NOT NULL, created_at datetime, updated_at datetime)`,
		`CREATE TABLE agent_email_messages (id text PRIMARY KEY, org_id text NOT NULL, agent_id text NOT NULL, thread_id text NOT NULL, direction text NOT NULL, message_id text NOT NULL, to_addresses text NOT NULL, cc_addresses text NOT NULL, provider_at datetime NOT NULL)`,
		`CREATE TABLE sessions (id text PRIMARY KEY, org_id text NOT NULL, team_id text NOT NULL, agent_id text NOT NULL, status text NOT NULL)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("create email reply test table: %v", err)
		}
	}

	orgID, teamID, agentID, sessionID, threadID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	now := time.Now().UTC()
	if err := db.Exec(`INSERT INTO sessions (id, org_id, team_id, agent_id, status) VALUES (?, ?, ?, ?, 'active')`,
		sessionID, orgID, teamID, agentID).Error; err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if err := db.Exec(`INSERT INTO agent_email_threads (id, org_id, agent_id, session_id, root_message_id, last_message_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		threadID, orgID, agentID, sessionID, "<outbound-1@example.test>", now, now, now).Error; err != nil {
		t.Fatalf("insert thread: %v", err)
	}
	if err := db.Exec(`INSERT INTO agent_email_messages (id, org_id, agent_id, thread_id, direction, message_id, to_addresses, cc_addresses, provider_at) VALUES (?, ?, ?, ?, ?, ?, ?, '[]', ?)`,
		uuid.New(), orgID, agentID, threadID, model.AgentEmailDirectionOutbound, "<outbound-1@example.test>", `["Member <member@example.test>"]`, now).Error; err != nil {
		t.Fatalf("insert outbound message: %v", err)
	}

	handler := &AgentEmailReceiveHandler{db: db, domain: "agents.example.test"}
	thread, err := handler.resolveThread(t.Context(), orgID, agentID, agentemail.ReceivedEmail{
		From:    "Member <member@example.test>",
		To:      []string{"agent-test1234@agents.example.test"},
		Headers: map[string]string{"In-Reply-To": "<outbound-1@example.test>"},
	})
	if err != nil {
		t.Fatalf("resolve reply thread: %v", err)
	}
	if thread.ID != threadID {
		t.Fatalf("resolved thread = %s, want %s", thread.ID, threadID)
	}

	agent := model.Agent{ID: agentID, OrgID: &orgID, TeamID: teamID}
	session, err := handler.activeThreadSession(t.Context(), agent, thread)
	if err != nil {
		t.Fatalf("load originating session: %v", err)
	}
	if session == nil || session.ID != sessionID {
		t.Fatalf("originating session = %#v, want %s", session, sessionID)
	}
}

func TestMatchedEmailReplyQueuesOriginatingSessionWithoutAutomation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	for _, statement := range []string{
		`CREATE TABLE sessions (id text PRIMARY KEY, org_id text NOT NULL, team_id text NOT NULL, agent_id text NOT NULL, sandbox_id text, model text NOT NULL, reasoning_effort text NOT NULL, status text NOT NULL)`,
		`CREATE TABLE session_events (
			id text PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
			org_id text NOT NULL, session_id text NOT NULL, agent_id text NOT NULL, sandbox_id text,
			runtime_session_id text, event_id text, event_type text NOT NULL, actor_user_id text,
			source text NOT NULL, sequence_number integer, runtime_seq integer, runtime_event_id text,
			turn_id text, span_id text, durability text, payload text NOT NULL, event_at datetime NOT NULL,
			retained_at datetime, created_at datetime,
			UNIQUE (session_id, event_id)
		)`,
		`CREATE TABLE session_message_queue (
			id text PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
			org_id text NOT NULL, session_id text NOT NULL, session_event_id text, actor_user_id text,
			message_text text NOT NULL, message_payload text NOT NULL, model text NOT NULL,
			reasoning_effort text NOT NULL, sequence_number integer NOT NULL, status text NOT NULL,
			attempt_count integer NOT NULL DEFAULT 0, leased_by text, leased_until datetime,
			delivered_at datetime, last_error text NOT NULL DEFAULT '', runtime_stream_id text NOT NULL DEFAULT '',
			runtime_stream_url text NOT NULL DEFAULT '', runtime_trace_id text NOT NULL DEFAULT '',
			runtime_turn_id text NOT NULL DEFAULT '', created_at datetime, updated_at datetime,
			UNIQUE (session_id, sequence_number),
			UNIQUE (session_id, session_event_id)
		)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("create email dispatch test table: %v", err)
		}
	}

	orgID, teamID, agentID, sessionID, threadID, messageID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if err := db.Exec(`INSERT INTO sessions (id, org_id, team_id, agent_id, model, reasoning_effort, status) VALUES (?, ?, ?, ?, 'test-model', 'high', 'active')`,
		sessionID, orgID, teamID, agentID).Error; err != nil {
		t.Fatalf("insert session: %v", err)
	}
	agent := model.Agent{ID: agentID, OrgID: &orgID, TeamID: teamID}
	thread := model.AgentEmailThread{ID: threadID, OrgID: orgID, AgentID: agentID, SessionID: &sessionID}
	message := model.AgentEmailMessage{
		ID:            messageID,
		OrgID:         orgID,
		AgentID:       agentID,
		ThreadID:      threadID,
		ResendEmailID: "received-1",
		FromAddress:   "Member <member@example.test>",
		Subject:       "Re: Status",
		TextBody:      "Please investigate.",
	}
	enqueuer := &sandboxSleepRecordingEnqueuer{}
	handler := &AgentEmailReceiveHandler{db: db, enqueuer: enqueuer, domain: "agents.example.test"}
	if err := handler.dispatchInbound(t.Context(), agent, &thread, message); err != nil {
		t.Fatalf("dispatch inbound reply: %v", err)
	}

	var queue model.SessionMessageQueue
	if err := db.First(&queue, "session_id = ?", sessionID).Error; err != nil {
		t.Fatalf("load queued reply: %v", err)
	}
	if !strings.Contains(queue.MessageText, "Please investigate.") {
		t.Fatalf("queued reply text = %q", queue.MessageText)
	}
	if strings.Contains(queue.MessageText, "Automation instructions:") {
		t.Fatalf("matched reply unexpectedly used automation instructions: %q", queue.MessageText)
	}
	if len(enqueuer.tasks) != 1 || enqueuer.tasks[0].Type() != TypeSessionMessageDeliver {
		t.Fatalf("delivery tasks = %#v", enqueuer.tasks)
	}
}
