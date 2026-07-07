package tasks

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func countOrgQueue(t *testing.T, db *gorm.DB, orgID uuid.UUID) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&model.SessionMessageQueue{}).Where("org_id = ?", orgID).Count(&n).Error; err != nil {
		t.Fatalf("count org queue: %v", err)
	}
	return n
}

// Exact per-app matching: on the PRIMARY connection, "@usehivy" fires and
// "@usehivy-reviews" does NOT (it contains "usehivy" but is a different handle).
func TestDeliverGitHubMentionPrimaryExactMatch(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int64
	}{
		{"primary handle fires", "hey @usehivy take a look", 1},
		{"reviews handle does not fire primary", "hey @usehivy-reviews take a look", 0},
		{"bare hivy does not fire", "hey @hivy take a look", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := connectTestDB(t)
			org, agent, _ := seedTriggerSessionFixture(t, db)
			h := &AgentTriggerDispatchHandler{db: db, enqueuer: &fakeTaskEnqueuer{}}
			trigger := seedMentionTrigger(t, db, org.ID, agent.ID, "acme/repo")
			connID := seedGitHubAppConnection(t, db, org.ID, "github-app", "usehivy")

			payload := AgentTriggerDispatchPayload{
				Provider: "github-app", EventType: "issue_comment", EventAction: "created",
				DeliveryID: "gh-" + tc.name, OrgID: org.ID, ConnectionID: connID,
			}
			webhook := githubIssueCommentPayload("acme/repo", "bob", tc.body, true)

			if err := h.deliverGitHubMention(context.Background(), payload, trigger, webhook, nil); err != nil {
				t.Fatalf("deliver: %v", err)
			}
			if got := countOrgQueue(t, db, org.ID); got != tc.want {
				t.Fatalf("queued = %d, want %d", got, tc.want)
			}
		})
	}
}

// Exact per-app matching: on the CODE-REVIEWS connection, "@usehivy-reviews"
// fires and "@usehivy" does NOT.
func TestDeliverGitHubMentionCodeReviewsExactMatch(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int64
	}{
		{"reviews handle fires", "hey @usehivy-reviews please review", 1},
		{"primary handle does not fire reviews", "hey @usehivy please review", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := connectTestDB(t)
			org, agent, _ := seedTriggerSessionFixture(t, db)
			h := &AgentTriggerDispatchHandler{db: db, enqueuer: &fakeTaskEnqueuer{}}
			trigger := seedMentionTrigger(t, db, org.ID, agent.ID, "acme/repo")
			connID := seedGitHubAppConnection(t, db, org.ID, "github-app-code-reviews", "usehivy-reviews")

			payload := AgentTriggerDispatchPayload{
				Provider: "github-app-code-reviews", EventType: "issue_comment", EventAction: "created",
				DeliveryID: "cr-" + tc.name, OrgID: org.ID, ConnectionID: connID,
			}
			webhook := githubIssueCommentPayload("acme/repo", "bob", tc.body, true)

			if err := h.deliverGitHubMention(context.Background(), payload, trigger, webhook, nil); err != nil {
				t.Fatalf("deliver: %v", err)
			}
			if got := countOrgQueue(t, db, org.ID); got != tc.want {
				t.Fatalf("queued = %d, want %d", got, tc.want)
			}
		})
	}
}

// An integration with an empty bot_handle is misconfigured: mentions are skipped
// with a warning rather than firing.
func TestDeliverGitHubMentionEmptyBotHandleSkips(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	h := &AgentTriggerDispatchHandler{db: db, enqueuer: &fakeTaskEnqueuer{}}
	trigger := seedMentionTrigger(t, db, org.ID, agent.ID, "acme/repo")
	connID := seedGitHubAppConnection(t, db, org.ID, "github-app", "")

	payload := AgentTriggerDispatchPayload{
		Provider: "github-app", EventType: "issue_comment", EventAction: "created",
		DeliveryID: "gh-empty", OrgID: org.ID, ConnectionID: connID,
	}
	webhook := githubIssueCommentPayload("acme/repo", "bob", "hey @usehivy take a look", true)

	if err := h.deliverGitHubMention(context.Background(), payload, trigger, webhook, nil); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if got := countOrgQueue(t, db, org.ID); got != 0 {
		t.Fatalf("queued = %d, want 0 (empty bot handle must skip)", got)
	}
}

// A code-reviews mention on a PR that HAS a build-session mapping must create
// the code-reviews trigger's OWN review session and must NOT be hijacked into
// the build session. A primary mention on the same PR still routes into the
// build session.
func TestDeliverGitHubMentionCodeReviewsDoesNotHijackBuildSession(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	h := &AgentTriggerDispatchHandler{db: db, enqueuer: &fakeTaskEnqueuer{}}
	ctx := context.Background()

	build := seedActiveSession(t, db, org.ID, agent.ID)
	if err := db.Create(&model.GitHubPullRequestSession{
		OrgID: org.ID, Repo: "acme/repo", PRNumber: 42, SessionID: build.ID,
	}).Error; err != nil {
		t.Fatalf("create mapping: %v", err)
	}

	// Code-reviews mention on the mapped PR.
	crTrigger := seedMentionTrigger(t, db, org.ID, agent.ID, "acme/repo")
	crConn := seedGitHubAppConnection(t, db, org.ID, "github-app-code-reviews", "usehivy-reviews")
	crPayload := AgentTriggerDispatchPayload{
		Provider: "github-app-code-reviews", EventType: "issue_comment", EventAction: "created",
		DeliveryID: "cr-1", OrgID: org.ID, ConnectionID: crConn,
	}
	crWebhook := githubIssueCommentPayload("acme/repo", "human", "@usehivy-reviews please review", true)
	if err := h.deliverGitHubMention(ctx, crPayload, crTrigger, crWebhook, nil); err != nil {
		t.Fatalf("cr deliver: %v", err)
	}

	if q := countSessionQueue(t, db, build.ID); q != 0 {
		t.Fatalf("build session queued = %d, want 0 (code-reviews must not hijack it)", q)
	}
	// The code-reviews mention created its own resource-keyed review session.
	var crSession model.Session
	if err := db.Where("org_id = ? AND source = ? AND source_resource_key = ?",
		org.ID, triggerConversationSource, "github/acme/repo/pull/42").First(&crSession).Error; err != nil {
		t.Fatalf("expected code-reviews review session: %v", err)
	}
	if crSession.ID == build.ID {
		t.Fatal("code-reviews session must be distinct from the build session")
	}
	if q := countSessionQueue(t, db, crSession.ID); q != 1 {
		t.Fatalf("code-reviews session queued = %d, want 1", q)
	}

	// A primary mention on the same PR routes into the build session.
	primaryTrigger := seedMentionTrigger(t, db, org.ID, agent.ID, "acme/repo")
	primaryConn := seedGitHubAppConnection(t, db, org.ID, "github-app", "usehivy")
	primaryPayload := AgentTriggerDispatchPayload{
		Provider: "github-app", EventType: "issue_comment", EventAction: "created",
		DeliveryID: "gh-1", OrgID: org.ID, ConnectionID: primaryConn,
	}
	primaryWebhook := githubIssueCommentPayload("acme/repo", "human", "@usehivy can you rebase?", true)
	if err := h.deliverGitHubMention(ctx, primaryPayload, primaryTrigger, primaryWebhook, nil); err != nil {
		t.Fatalf("primary deliver: %v", err)
	}
	if q := countSessionQueue(t, db, build.ID); q != 1 {
		t.Fatalf("build session queued = %d, want 1 (primary mention must route into it)", q)
	}
}

// Auto-route is primary-only: the gate isPRRouteEventKey (checked in Handle
// before maybeRoutePREvent runs) rejects the code-reviews app's copy of every
// PR-route event, so a code-reviews event never auto-routes into a build
// session. The same event on the primary app is accepted.
func TestIsPRRouteEventKeyPrimaryOnly(t *testing.T) {
	routeKeys := []string{
		"check_suite.completed",
		"pull_request_review.submitted",
		"pull_request_review_comment.created",
		"issue_comment.created",
	}
	for _, key := range routeKeys {
		if !isPRRouteEventKey("github-app", key) {
			t.Errorf("primary %q should auto-route", key)
		}
		if isPRRouteEventKey("github-app-code-reviews", key) {
			t.Errorf("code-reviews %q must NOT auto-route", key)
		}
	}
	if isPRRouteEventKey("github-app", "push") {
		t.Error("push is not a PR-route event")
	}
}
