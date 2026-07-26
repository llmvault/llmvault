package agentemail

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func TestEmailReplyContextForSessionDerivesThreadRecipientAndSubject(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	for _, statement := range []string{
		`CREATE TABLE agent_email_threads (id text PRIMARY KEY, org_id text NOT NULL, agent_id text NOT NULL, session_id text, root_message_id text NOT NULL, last_message_at datetime NOT NULL, created_at datetime, updated_at datetime)`,
		`CREATE TABLE agent_email_messages (id text PRIMARY KEY, org_id text NOT NULL, agent_id text NOT NULL, thread_id text NOT NULL, direction text NOT NULL, status text NOT NULL, resend_email_id text NOT NULL, message_id text NOT NULL, in_reply_to text NOT NULL, "references" text NOT NULL, from_address text NOT NULL, to_addresses text NOT NULL, cc_addresses text NOT NULL, subject text NOT NULL, text_body text NOT NULL, html_body text NOT NULL, headers text NOT NULL, provider_at datetime NOT NULL, created_at datetime, updated_at datetime)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("create inbox table: %v", err)
		}
	}
	orgID, agentID, sessionID, threadID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	thread := model.AgentEmailThread{ID: threadID, OrgID: orgID, AgentID: agentID, SessionID: &sessionID, LastMessageAt: time.Now().UTC()}
	if err := db.Create(&thread).Error; err != nil {
		t.Fatalf("create thread: %v", err)
	}
	message := model.AgentEmailMessage{ID: uuid.New(), OrgID: orgID, AgentID: agentID, ThreadID: threadID, Direction: model.AgentEmailDirectionInbound, Status: model.AgentEmailStatusReceived, FromAddress: "Sender <sender@example.test>", Subject: "Project update", Headers: model.RawJSON(`{"Reply-To":"Reply Person <reply@example.test>"}`), ProviderAt: time.Now().UTC()}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create inbound message: %v", err)
	}

	got, err := emailReplyContextForSession(context.Background(), db, orgID, agentID, sessionID)
	if err != nil {
		t.Fatalf("emailReplyContextForSession: %v", err)
	}
	if got == nil || got.thread.ID != threadID || got.recipient != "reply@example.test" || got.subject != "Re: Project update" {
		t.Fatalf("reply context = %#v", got)
	}
}

func TestNewOutboundThreadBindsOriginatingSession(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(`CREATE TABLE agent_email_threads (id text PRIMARY KEY, org_id text NOT NULL, agent_id text NOT NULL, session_id text, root_message_id text NOT NULL DEFAULT '', last_message_at datetime NOT NULL, created_at datetime, updated_at datetime)`).Error; err != nil {
		t.Fatalf("create inbox thread table: %v", err)
	}
	orgID, agentID, sessionID := uuid.New(), uuid.New(), uuid.New()
	thread, err := newOutboundThread(t.Context(), db, orgID, agentID, sessionID)
	if err != nil {
		t.Fatalf("newOutboundThread: %v", err)
	}
	if thread.SessionID == nil || *thread.SessionID != sessionID {
		t.Fatalf("thread session = %v, want %s", thread.SessionID, sessionID)
	}
}

func TestReplySubjectDoesNotDuplicatePrefix(t *testing.T) {
	if got := replySubject("RE: Project update"); got != "RE: Project update" {
		t.Fatalf("replySubject() = %q", got)
	}
}

func TestRequireTeamRecipientsAllowsOnlyOwningTeamMembers(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	for _, statement := range []string{
		`CREATE TABLE users (id text PRIMARY KEY, email text NOT NULL, banned_at datetime)`,
		`CREATE TABLE team_members (id text PRIMARY KEY, org_id text NOT NULL, team_id text NOT NULL, user_id text NOT NULL)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("create team recipient table: %v", err)
		}
	}
	orgID, teamID, otherTeamID, memberID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if err := db.Exec(`INSERT INTO users (id, email) VALUES (?, ?)`, memberID.String(), "member@example.test").Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Exec(`INSERT INTO team_members (id, org_id, team_id, user_id) VALUES (?, ?, ?, ?)`, uuid.NewString(), orgID.String(), teamID.String(), memberID.String()).Error; err != nil {
		t.Fatalf("create team member: %v", err)
	}
	ctx := context.Background()
	if err := requireTeamRecipients(ctx, db, orgID, teamID, []string{"Member <member@example.test>"}); err != nil {
		t.Fatalf("allow team recipient: %v", err)
	}
	if err := requireTeamRecipients(ctx, db, orgID, otherTeamID, []string{"member@example.test"}); err == nil {
		t.Fatal("cross-team recipient was allowed")
	}
}
