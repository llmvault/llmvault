package tasks

import (
	"context"
	"testing"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/nango"
)

func newReactionRouteHandler(db *gorm.DB, client *nango.Client) *AgentTriggerDispatchHandler {
	return &AgentTriggerDispatchHandler{db: db, enqueuer: &fakeTaskEnqueuer{}, nangoClient: client}
}

func TestPRRouteIssueCommentMentionReactsOnce(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	orig := seedActiveSession(t, db, org.ID, agent.ID)
	seedPRMapping(t, db, org.ID, "acme/repo", 42, orig.ID)
	connID := seedGitHubConnection(t, db, org.ID)
	client, rc := newReactionCapture(t)
	h := newReactionRouteHandler(db, client)

	payload := AgentTriggerDispatchPayload{
		Provider: "github-app", EventType: "issue_comment", EventAction: "created",
		OrgID: org.ID, ConnectionID: connID,
		DeliveryID: prRouteDeliveryID("conn-a", "issue_comment.created", "492700400"),
	}
	webhook := githubIssueCommentPayload("acme/repo", "human", "hey @hivy can you rebase?", true)

	if _, err := h.maybeRoutePREvent(context.Background(), payload, webhook); err != nil {
		t.Fatalf("route: %v", err)
	}
	if rc.count() != 1 {
		t.Fatalf("first delivery reactions=%d, want 1", rc.count())
	}
	if got := rc.calls[0].path; got != "/repos/acme/repo/issues/comments/492700400/reactions" {
		t.Fatalf("path=%q", got)
	}
	assertEyesBody(t, rc.calls[0].body)

	// The same event redelivered via the second GitHub App connection reuses the
	// queue row (created=false) and must not react again.
	if _, err := h.maybeRoutePREvent(context.Background(), payload, webhook); err != nil {
		t.Fatalf("redelivery: %v", err)
	}
	if rc.count() != 1 {
		t.Fatalf("after redelivery reactions=%d, want 1", rc.count())
	}
}

func TestPRRouteReviewCommentMentionReacts(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	orig := seedActiveSession(t, db, org.ID, agent.ID)
	seedPRMapping(t, db, org.ID, "acme/repo", 11, orig.ID)
	connID := seedGitHubConnection(t, db, org.ID)
	client, rc := newReactionCapture(t)
	h := newReactionRouteHandler(db, client)

	payload := AgentTriggerDispatchPayload{
		Provider: "github-app", EventType: "pull_request_review_comment", EventAction: "created",
		OrgID: org.ID, ConnectionID: connID,
		DeliveryID: prRouteDeliveryID("conn-a", "pull_request_review_comment.created", "555000333"),
	}
	webhook := githubReviewCommentPayload("acme/repo", 11, "human", "@hivy please fix", "usehivy[bot]")

	if _, err := h.maybeRoutePREvent(context.Background(), payload, webhook); err != nil {
		t.Fatalf("route: %v", err)
	}
	if rc.count() != 1 {
		t.Fatalf("reactions=%d, want 1", rc.count())
	}
	if got := rc.calls[0].path; got != "/repos/acme/repo/pulls/comments/555000333/reactions" {
		t.Fatalf("path=%q", got)
	}
}

// A routed comment with no Hivy mention gets no reaction.
func TestPRRouteCommentWithoutMentionDoesNotReact(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	orig := seedActiveSession(t, db, org.ID, agent.ID)
	seedPRMapping(t, db, org.ID, "acme/repo", 42, orig.ID)
	connID := seedGitHubConnection(t, db, org.ID)
	client, rc := newReactionCapture(t)
	h := newReactionRouteHandler(db, client)

	payload := AgentTriggerDispatchPayload{
		Provider: "github-app", EventType: "issue_comment", EventAction: "created",
		OrgID: org.ID, ConnectionID: connID,
		DeliveryID: prRouteDeliveryID("conn-a", "issue_comment.created", "492700400"),
	}
	webhook := githubIssueCommentPayload("acme/repo", "human", "can you rebase this?", true)

	if _, err := h.maybeRoutePREvent(context.Background(), payload, webhook); err != nil {
		t.Fatalf("route: %v", err)
	}
	if rc.count() != 0 {
		t.Fatalf("non-mention reactions=%d, want 0", rc.count())
	}
}

// pull_request_review.submitted has no reactions endpoint, so it never reacts.
func TestPRRouteReviewSubmittedDoesNotReact(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	orig := seedActiveSession(t, db, org.ID, agent.ID)
	seedPRMapping(t, db, org.ID, "acme/repo", 7, orig.ID)
	connID := seedGitHubConnection(t, db, org.ID)
	client, rc := newReactionCapture(t)
	h := newReactionRouteHandler(db, client)

	payload := AgentTriggerDispatchPayload{
		Provider: "github-app", EventType: "pull_request_review", EventAction: "submitted",
		OrgID: org.ID, ConnectionID: connID,
		DeliveryID: prRouteDeliveryID("conn-a", "pull_request_review.submitted", "777000222"),
	}
	webhook := githubReviewPayload("acme/repo", 7, "human", "changes_requested", "@hivy please fix", "usehivy[bot]")

	if _, err := h.maybeRoutePREvent(context.Background(), payload, webhook); err != nil {
		t.Fatalf("route: %v", err)
	}
	if rc.count() != 0 {
		t.Fatalf("review-submitted reactions=%d, want 0", rc.count())
	}
}
