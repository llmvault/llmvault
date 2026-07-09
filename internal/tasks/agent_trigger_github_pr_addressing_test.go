package tasks

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// seedBothGitHubApps installs both Hivy GitHub App connections on the org and
// returns the primary connection id (the one auto-route delivers under).
func seedBothGitHubApps(t *testing.T, db *gorm.DB, orgID uuid.UUID, primaryHandle, reviewsHandle string) uuid.UUID {
	t.Helper()
	primary := seedGitHubAppConnection(t, db, orgID, "github-app", primaryHandle)
	seedGitHubAppConnection(t, db, orgID, "github-app-code-reviews", reviewsHandle)
	return primary
}

func issueCommentRoutePayload(orgID, connID uuid.UUID) AgentTriggerDispatchPayload {
	return AgentTriggerDispatchPayload{
		Provider: "github-app", EventType: "issue_comment", EventAction: "created",
		OrgID: orgID, ConnectionID: connID,
		DeliveryID: prRouteDeliveryID("conn-a", "issue_comment.created", "492700400"),
	}
}

// --- decision table, issue_comment.created on a primary-owned PR session ---

func TestPRRouteAddressingOnlyOtherAppSkips(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	orig := seedActiveSession(t, db, org.ID, agent.ID)
	seedPRMapping(t, db, org.ID, "acme/repo", 42, orig.ID)
	connID := seedBothGitHubApps(t, db, org.ID, "usehivy", "usehivy-reviews")
	h := newPRRouteHandler(db)

	payload := issueCommentRoutePayload(org.ID, connID)
	webhook := githubIssueCommentPayload("acme/repo", "human-commenter", "@usehivy-reviews please review", true)
	routed, err := h.maybeRoutePREvent(context.Background(), payload, webhook)
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if len(routed) != 0 || countSessionQueue(t, db, orig.ID) != 0 {
		t.Fatalf("comment addressed to other app routed=%v queued=%d, want none",
			routed, countSessionQueue(t, db, orig.ID))
	}
}

func TestPRRouteAddressingOwnHandleRoutes(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	orig := seedActiveSession(t, db, org.ID, agent.ID)
	seedPRMapping(t, db, org.ID, "acme/repo", 42, orig.ID)
	connID := seedBothGitHubApps(t, db, org.ID, "usehivy", "usehivy-reviews")
	h := newPRRouteHandler(db)

	payload := issueCommentRoutePayload(org.ID, connID)
	webhook := githubIssueCommentPayload("acme/repo", "human-commenter", "@usehivy please fix the tests", true)
	routed, err := h.maybeRoutePREvent(context.Background(), payload, webhook)
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if _, ok := routed[orig.ID]; !ok {
		t.Fatalf("comment addressed to own app not routed: %v", routed)
	}
	assertRoutedOnce(t, db, org.ID, orig.ID)
}

func TestPRRouteAddressingBothHandlesRoutes(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	orig := seedActiveSession(t, db, org.ID, agent.ID)
	seedPRMapping(t, db, org.ID, "acme/repo", 42, orig.ID)
	connID := seedBothGitHubApps(t, db, org.ID, "usehivy", "usehivy-reviews")
	h := newPRRouteHandler(db)

	payload := issueCommentRoutePayload(org.ID, connID)
	webhook := githubIssueCommentPayload("acme/repo", "human-commenter", "@usehivy @usehivy-reviews please both look", true)
	routed, err := h.maybeRoutePREvent(context.Background(), payload, webhook)
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if _, ok := routed[orig.ID]; !ok {
		t.Fatalf("comment mentioning both apps not routed: %v", routed)
	}
}

func TestPRRouteAddressingNoHandleRoutes(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	orig := seedActiveSession(t, db, org.ID, agent.ID)
	seedPRMapping(t, db, org.ID, "acme/repo", 42, orig.ID)
	connID := seedBothGitHubApps(t, db, org.ID, "usehivy", "usehivy-reviews")
	h := newPRRouteHandler(db)

	payload := issueCommentRoutePayload(org.ID, connID)
	webhook := githubIssueCommentPayload("acme/repo", "human-commenter", "please take another pass at this", true)
	routed, err := h.maybeRoutePREvent(context.Background(), payload, webhook)
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if _, ok := routed[orig.ID]; !ok {
		t.Fatalf("plain comment not routed: %v", routed)
	}
}

// --- only-other-app addressing on the remaining PR-scoped shapes ---

func TestPRRouteAddressingReviewCommentOnlyOtherAppSkips(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	orig := seedActiveSession(t, db, org.ID, agent.ID)
	seedPRMapping(t, db, org.ID, "acme/repo", 11, orig.ID)
	connID := seedBothGitHubApps(t, db, org.ID, "usehivy", "usehivy-reviews")
	h := newPRRouteHandler(db)

	payload := AgentTriggerDispatchPayload{
		Provider: "github-app", EventType: "pull_request_review_comment", EventAction: "created",
		OrgID: org.ID, ConnectionID: connID,
		DeliveryID: prRouteDeliveryID("conn-a", "pull_request_review_comment.created", "555000333"),
	}
	webhook := githubReviewCommentPayload("acme/repo", 11, "human-reviewer", "@usehivy-reviews thoughts?", "alice")
	routed, err := h.maybeRoutePREvent(context.Background(), payload, webhook)
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if len(routed) != 0 || countSessionQueue(t, db, orig.ID) != 0 {
		t.Fatalf("review comment addressed to other app routed=%v queued=%d, want none",
			routed, countSessionQueue(t, db, orig.ID))
	}
}

func TestPRRouteAddressingReviewBodyOnlyOtherAppSkips(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	orig := seedActiveSession(t, db, org.ID, agent.ID)
	seedPRMapping(t, db, org.ID, "acme/repo", 7, orig.ID)
	connID := seedBothGitHubApps(t, db, org.ID, "usehivy", "usehivy-reviews")
	h := newPRRouteHandler(db)

	payload := AgentTriggerDispatchPayload{
		Provider: "github-app", EventType: "pull_request_review", EventAction: "submitted",
		OrgID: org.ID, ConnectionID: connID,
		DeliveryID: prRouteDeliveryID("conn-a", "pull_request_review.submitted", "777000222"),
	}
	webhook := githubReviewPayload("acme/repo", 7, "human-reviewer", "commented", "@usehivy-reviews please review", "alice")
	routed, err := h.maybeRoutePREvent(context.Background(), payload, webhook)
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if len(routed) != 0 || countSessionQueue(t, db, orig.ID) != 0 {
		t.Fatalf("review body addressed to other app routed=%v queued=%d, want none",
			routed, countSessionQueue(t, db, orig.ID))
	}
}

// --- prefix collision + dynamic handle resolution ---

// @usehivy-reviews must never count as @usehivy (skip), and @usehivy must never
// count as @usehivy-reviews (route) — the two handles share a prefix.
func TestPRRouteAddressingPrefixCollision(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	orig := seedActiveSession(t, db, org.ID, agent.ID)
	seedPRMapping(t, db, org.ID, "acme/repo", 42, orig.ID)
	connID := seedBothGitHubApps(t, db, org.ID, "usehivy", "usehivy-reviews")
	h := newPRRouteHandler(db)
	ctx := context.Background()

	// @usehivy-reviews only → does not count as the primary handle → skip.
	skipPayload := issueCommentRoutePayload(org.ID, connID)
	skipWebhook := githubIssueCommentPayload("acme/repo", "human-commenter", "@usehivy-reviews only you", true)
	if routed, err := h.maybeRoutePREvent(ctx, skipPayload, skipWebhook); err != nil || len(routed) != 0 {
		t.Fatalf("prefix: @usehivy-reviews routed=%v err=%v, want skip", routed, err)
	}

	// @usehivy only → does not count as the reviews handle → own-mention → route.
	routePayload := issueCommentRoutePayload(org.ID, connID)
	routeWebhook := githubIssueCommentPayload("acme/repo", "human-commenter", "@usehivy only you", true)
	routed, err := h.maybeRoutePREvent(ctx, routePayload, routeWebhook)
	if err != nil {
		t.Fatalf("prefix route: %v", err)
	}
	if _, ok := routed[orig.ID]; !ok {
		t.Fatalf("prefix: @usehivy not routed: %v", routed)
	}
}

// Handles come from the connections, not from hardcoded literals: renaming the
// apps changes which mention is exclusive.
func TestPRRouteAddressingHandlesResolvedFromConnections(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	orig := seedActiveSession(t, db, org.ID, agent.ID)
	seedPRMapping(t, db, org.ID, "acme/repo", 42, orig.ID)
	connID := seedBothGitHubApps(t, db, org.ID, "acme-bot", "acme-reviews")
	h := newPRRouteHandler(db)
	ctx := context.Background()

	// @acme-reviews only → other app → skip.
	skipWebhook := githubIssueCommentPayload("acme/repo", "human-commenter", "@acme-reviews please review", true)
	if routed, err := h.maybeRoutePREvent(ctx, issueCommentRoutePayload(org.ID, connID), skipWebhook); err != nil || len(routed) != 0 {
		t.Fatalf("custom handles: @acme-reviews routed=%v err=%v, want skip", routed, err)
	}

	// @acme-bot only → own app → route.
	routeWebhook := githubIssueCommentPayload("acme/repo", "human-commenter", "@acme-bot please review", true)
	routed, err := h.maybeRoutePREvent(ctx, issueCommentRoutePayload(org.ID, connID), routeWebhook)
	if err != nil {
		t.Fatalf("custom handles route: %v", err)
	}
	if _, ok := routed[orig.ID]; !ok {
		t.Fatalf("custom handles: @acme-bot not routed: %v", routed)
	}
}

// With no code-reviews app installed, the addressing gate cannot claim the event
// for another app, so a comment naming an unrelated handle still routes (the
// only Hivy app present is the PR owner).
func TestPRRouteAddressingNoSecondAppRoutes(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	orig := seedActiveSession(t, db, org.ID, agent.ID)
	seedPRMapping(t, db, org.ID, "acme/repo", 42, orig.ID)
	connID := seedGitHubAppConnection(t, db, org.ID, "github-app", "usehivy")
	h := newPRRouteHandler(db)

	webhook := githubIssueCommentPayload("acme/repo", "human-commenter", "@usehivy-reviews please review", true)
	routed, err := h.maybeRoutePREvent(context.Background(), issueCommentRoutePayload(org.ID, connID), webhook)
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if _, ok := routed[orig.ID]; !ok {
		t.Fatalf("comment routed=%v, want route (no second app to claim it)", routed)
	}
}
