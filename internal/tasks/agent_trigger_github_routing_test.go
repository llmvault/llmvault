package tasks

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func seedMentionTrigger(t *testing.T, db *gorm.DB, orgID, agentID uuid.UUID, repo string) model.AgentTrigger {
	t.Helper()
	trigger := model.AgentTrigger{
		ID: uuid.New(), OrgID: orgID, AgentID: agentID, TriggerType: "webhook",
		TriggerKey: model.TriggerKeyGitHubPRMention, TriggerValue: repo, Enabled: true,
	}
	if err := db.Create(&trigger).Error; err != nil {
		t.Fatalf("create mention trigger: %v", err)
	}
	return trigger
}

func seedActiveSession(t *testing.T, db *gorm.DB, orgID, agentID uuid.UUID) model.Session {
	t.Helper()
	channel := model.Channel{OrgID: orgID, Name: "orig-" + uuid.NewString()[:8], DefaultAgentID: agentID, Origin: "native"}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	session := model.Session{
		OrgID: orgID, ChannelID: channel.ID, AgentID: agentID, Model: "test-model",
		Source: "external", Status: "active", AgentTurnStatus: model.SessionAgentTurnIdle,
		IntegrationScopes: model.JSON{},
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	return session
}

// A PR comment mentioning Hivy on a PR that a session opened routes into that
// originating session — no fresh PR-scoped session is created.
func TestGitHubMentionRoutesToOriginatingSession(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	h := &AgentTriggerDispatchHandler{db: db, enqueuer: &fakeTaskEnqueuer{}}
	ctx := context.Background()

	trigger := seedMentionTrigger(t, db, org.ID, agent.ID, "acme/repo")
	orig := seedActiveSession(t, db, org.ID, agent.ID)
	if err := db.Create(&model.GitHubPullRequestSession{
		OrgID: org.ID, Repo: "acme/repo", PRNumber: 42, SessionID: orig.ID,
	}).Error; err != nil {
		t.Fatalf("create mapping: %v", err)
	}

	payload := AgentTriggerDispatchPayload{
		Provider: "github-app", EventType: "issue_comment", EventAction: "created",
		DeliveryID: "gh-1", OrgID: org.ID,
	}
	webhook := githubIssueCommentPayload("acme/repo", "bob", "hey @hivy take a look", true)

	if err := h.deliverGitHubMention(ctx, payload, trigger, webhook); err != nil {
		t.Fatalf("deliver: %v", err)
	}

	var routed int64
	if err := db.Model(&model.SessionMessageQueue{}).Where("session_id = ?", orig.ID).Count(&routed).Error; err != nil {
		t.Fatalf("count routed: %v", err)
	}
	if routed != 1 {
		t.Fatalf("originating session queued = %d, want 1", routed)
	}
	var fallback int64
	if err := db.Model(&model.Session{}).
		Where("org_id = ? AND source = ? AND source_resource_key = ?", org.ID, triggerConversationSource, "github/acme/repo/pull/42").
		Count(&fallback).Error; err != nil {
		t.Fatalf("count fallback: %v", err)
	}
	if fallback != 0 {
		t.Fatalf("fallback PR-scoped sessions = %d, want 0 (should route to originating)", fallback)
	}
}

// With no mapping, a PR mention falls back to a fresh PR-scoped trigger session.
func TestGitHubMentionFallsBackWhenNoMapping(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	h := &AgentTriggerDispatchHandler{db: db, enqueuer: &fakeTaskEnqueuer{}}
	ctx := context.Background()

	trigger := seedMentionTrigger(t, db, org.ID, agent.ID, "acme/repo")

	payload := AgentTriggerDispatchPayload{
		Provider: "github-app", EventType: "issue_comment", EventAction: "created",
		DeliveryID: "gh-2", OrgID: org.ID,
	}
	webhook := githubIssueCommentPayload("acme/repo", "bob", "hey @hivy take a look", true)

	if err := h.deliverGitHubMention(ctx, payload, trigger, webhook); err != nil {
		t.Fatalf("deliver: %v", err)
	}

	var session model.Session
	if err := db.Where("org_id = ? AND source = ? AND source_resource_key = ?", org.ID, triggerConversationSource, "github/acme/repo/pull/42").
		First(&session).Error; err != nil {
		t.Fatalf("expected PR-scoped fallback session: %v", err)
	}
	var queued int64
	db.Model(&model.SessionMessageQueue{}).Where("session_id = ?", session.ID).Count(&queued)
	if queued != 1 {
		t.Fatalf("fallback session queued = %d, want 1", queued)
	}
}
