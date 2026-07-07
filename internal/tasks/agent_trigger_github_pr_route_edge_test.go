package tasks

import (
	"context"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/model"
)

// Comments/reviews Hivy or the PR author itself authored must not loop back;
// a distinct code-review bot's review must still route.
func TestPRRouteSelfAuthoredGuard(t *testing.T) {
	t.Run("usehivy bot comment skips", func(t *testing.T) {
		db := connectTestDB(t)
		org, agent, _ := seedTriggerSessionFixture(t, db)
		orig := seedActiveSession(t, db, org.ID, agent.ID)
		seedPRMapping(t, db, org.ID, "acme/repo", 42, orig.ID)
		h := newPRRouteHandler(db)

		payload := AgentTriggerDispatchPayload{
			Provider: "github-app", EventType: "issue_comment", EventAction: "created",
			OrgID: org.ID, DeliveryID: prRouteDeliveryID("conn-a", "issue_comment.created", "492700400"),
		}
		webhook := githubIssueCommentPayload("acme/repo", "usehivy[bot]", "pushed a fix", true)

		routed, err := h.maybeRoutePREvent(context.Background(), payload, webhook)
		if err != nil {
			t.Fatalf("route: %v", err)
		}
		if len(routed) != 0 || countSessionQueue(t, db, orig.ID) != 0 {
			t.Fatalf("self comment routed=%v queued=%d, want none", routed, countSessionQueue(t, db, orig.ID))
		}
	})

	t.Run("pr author comment skips", func(t *testing.T) {
		db := connectTestDB(t)
		org, agent, _ := seedTriggerSessionFixture(t, db)
		orig := seedActiveSession(t, db, org.ID, agent.ID)
		seedPRMapping(t, db, org.ID, "acme/repo", 42, orig.ID)
		h := newPRRouteHandler(db)

		payload := AgentTriggerDispatchPayload{
			Provider: "github-app", EventType: "issue_comment", EventAction: "created",
			OrgID: org.ID, DeliveryID: prRouteDeliveryID("conn-a", "issue_comment.created", "492700400"),
		}
		// githubIssueCommentPayload fixes issue.user.login = "alice"; a comment
		// authored by "alice" is the PR author talking to itself.
		webhook := githubIssueCommentPayload("acme/repo", "alice", "self note", true)

		routed, err := h.maybeRoutePREvent(context.Background(), payload, webhook)
		if err != nil {
			t.Fatalf("route: %v", err)
		}
		if len(routed) != 0 {
			t.Fatalf("pr-author comment routed = %v, want none", routed)
		}
	})

	t.Run("distinct review bot routes", func(t *testing.T) {
		db := connectTestDB(t)
		org, agent, _ := seedTriggerSessionFixture(t, db)
		orig := seedActiveSession(t, db, org.ID, agent.ID)
		seedPRMapping(t, db, org.ID, "acme/repo", 9, orig.ID)
		h := newPRRouteHandler(db)

		payload := AgentTriggerDispatchPayload{
			Provider: "github-app", EventType: "pull_request_review", EventAction: "submitted",
			OrgID: org.ID, DeliveryID: prRouteDeliveryID("conn-a", "pull_request_review.submitted", "777000222"),
		}
		// "hivy-code-reviews[bot]" contains "hivy" — the broad isHivyIdentityValue
		// would wrongly skip it; the login-equality guard must route it.
		webhook := githubReviewPayload("acme/repo", 9, "hivy-code-reviews[bot]", "commented", "LGTM with nits", "usehivy[bot]")

		routed, err := h.maybeRoutePREvent(context.Background(), payload, webhook)
		if err != nil {
			t.Fatalf("route: %v", err)
		}
		if _, ok := routed[orig.ID]; !ok {
			t.Fatalf("distinct review bot not routed: %v", routed)
		}
		assertRoutedOnce(t, db, org.ID, orig.ID)
	})
}

// The same event redelivered via the second GitHub App connection (delivery id
// differs only in its connection prefix) dedups to one transcript event + queue
// row.
func TestPRRouteDedupAcrossConnectionPrefix(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	orig := seedActiveSession(t, db, org.ID, agent.ID)
	seedPRMapping(t, db, org.ID, "acme/repo", 42, orig.ID)
	h := newPRRouteHandler(db)
	ctx := context.Background()

	base := githubIssueCommentPayload("acme/repo", "human-commenter", "please rebase", true)
	first := AgentTriggerDispatchPayload{
		Provider: "github-app", EventType: "issue_comment", EventAction: "created",
		OrgID: org.ID, DeliveryID: prRouteDeliveryID("conn-a", "issue_comment.created", "492700400"),
	}
	second := AgentTriggerDispatchPayload{
		Provider: "github-app", EventType: "issue_comment", EventAction: "created",
		OrgID: org.ID, DeliveryID: prRouteDeliveryID("conn-b", "issue_comment.created", "492700400"),
	}

	if _, err := h.maybeRoutePREvent(ctx, first, base); err != nil {
		t.Fatalf("first route: %v", err)
	}
	if _, err := h.maybeRoutePREvent(ctx, second, base); err != nil {
		t.Fatalf("second route: %v", err)
	}
	assertRoutedOnce(t, db, org.ID, orig.ID)
}

// A busy session queues the routed message behind the active turn without
// disturbing the turn.
func TestPRRouteBusySessionQueues(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	orig := seedActiveSession(t, db, org.ID, agent.ID)
	if err := db.Model(&model.Session{}).Where("id = ?", orig.ID).Updates(map[string]any{
		"agent_turn_status":     model.SessionAgentTurnActive,
		"agent_turn_started_at": time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("mark active: %v", err)
	}
	seedPRMapping(t, db, org.ID, "acme/repo", 42, orig.ID)
	h := newPRRouteHandler(db)

	payload := AgentTriggerDispatchPayload{
		Provider: "github-app", EventType: "check_suite", EventAction: "completed",
		OrgID: org.ID, DeliveryID: prRouteDeliveryID("conn-a", "check_suite.completed", "999000111:failure"),
	}
	if _, err := h.maybeRoutePREvent(context.Background(), payload, githubCheckSuitePayload("acme/repo", []int{42}, "failure")); err != nil {
		t.Fatalf("route: %v", err)
	}
	assertRoutedOnce(t, db, org.ID, orig.ID)

	var reloaded model.Session
	if err := db.First(&reloaded, "id = ?", orig.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.AgentTurnStatus != model.SessionAgentTurnActive {
		t.Fatalf("turn status = %q, want active (routing must not post directly)", reloaded.AgentTurnStatus)
	}
}

// An inactive/archived originating session has no routable mapping: no delivery,
// no error (triggers still run).
func TestPRRouteArchivedSessionNoRoute(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	orig := seedActiveSession(t, db, org.ID, agent.ID)
	if err := db.Model(&model.Session{}).Where("id = ?", orig.ID).Update("status", "archived").Error; err != nil {
		t.Fatalf("archive: %v", err)
	}
	seedPRMapping(t, db, org.ID, "acme/repo", 42, orig.ID)
	h := newPRRouteHandler(db)

	payload := AgentTriggerDispatchPayload{
		Provider: "github-app", EventType: "check_suite", EventAction: "completed",
		OrgID: org.ID, DeliveryID: prRouteDeliveryID("conn-a", "check_suite.completed", "999000111:success"),
	}
	routed, err := h.maybeRoutePREvent(context.Background(), payload, githubCheckSuitePayload("acme/repo", []int{42}, "success"))
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if len(routed) != 0 {
		t.Fatalf("archived session routed = %v, want none", routed)
	}
	if q := countSessionQueue(t, db, orig.ID); q != 0 {
		t.Fatalf("queued = %d, want 0", q)
	}
}

// When a PR mention is also auto-routed, the mention path must not create a
// second delivery: exactly one queue row + transcript event in the session.
func TestPRRouteMentionOverrideNoDuplicate(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	orig := seedActiveSession(t, db, org.ID, agent.ID)
	seedPRMapping(t, db, org.ID, "acme/repo", 42, orig.ID)
	trigger := seedMentionTrigger(t, db, org.ID, agent.ID, "acme/repo")
	h := newPRRouteHandler(db)
	ctx := context.Background()

	payload := AgentTriggerDispatchPayload{
		Provider: "github-app", EventType: "issue_comment", EventAction: "created",
		OrgID: org.ID, DeliveryID: prRouteDeliveryID("conn-a", "issue_comment.created", "492700400"),
	}
	webhook := githubIssueCommentPayload("acme/repo", "human-commenter", "hey @hivy please review", true)

	// Reproduce Handle's ordering: auto-route first, then the mention trigger.
	routed, err := h.maybeRoutePREvent(ctx, payload, webhook)
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if _, ok := routed[orig.ID]; !ok {
		t.Fatalf("mention comment not auto-routed: %v", routed)
	}
	if err := h.deliverGitHubMention(ctx, payload, trigger, webhook, routed); err != nil {
		t.Fatalf("mention deliver: %v", err)
	}
	if q := countSessionQueue(t, db, orig.ID); q != 1 {
		t.Fatalf("queued = %d, want 1 (mention must not duplicate the auto-route)", q)
	}
	if e := countAutomatedEvents(t, db, orig.ID); e != 1 {
		t.Fatalf("automated events = %d, want 1", e)
	}
}
