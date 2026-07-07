package tasks

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/runtimeevents"
)

// --- payload fixtures (shapes per docs.github.com webhook payload reference) ---

func githubCheckSuitePayload(repo string, prNumbers []int, conclusion string) map[string]any {
	prs := make([]any, 0, len(prNumbers))
	for _, n := range prNumbers {
		prs = append(prs, map[string]any{"number": float64(n)})
	}
	return map[string]any{
		"action":     "completed",
		"repository": map[string]any{"full_name": repo},
		"check_suite": map[string]any{
			"id":            float64(999000111),
			"head_branch":   "fix-login",
			"head_sha":      "abc123def456",
			"status":        "completed",
			"conclusion":    conclusion,
			"pull_requests": prs,
		},
	}
}

func githubReviewPayload(repo string, prNumber int, reviewer, state, body, prAuthor string) map[string]any {
	return map[string]any{
		"action":     "submitted",
		"repository": map[string]any{"full_name": repo},
		"review": map[string]any{
			"id":    float64(777000222),
			"state": state,
			"body":  body,
			"user":  map[string]any{"login": reviewer, "type": "User"},
		},
		"pull_request": map[string]any{
			"number": float64(prNumber),
			"user":   map[string]any{"login": prAuthor, "type": "Bot"},
		},
	}
}

func githubReviewCommentPayload(repo string, prNumber int, author, body, prAuthor string) map[string]any {
	return map[string]any{
		"action":     "created",
		"repository": map[string]any{"full_name": repo},
		"comment": map[string]any{
			"id":        float64(555000333),
			"body":      body,
			"path":      "internal/foo.go",
			"line":      float64(42),
			"diff_hunk": "@@ -1,3 +1,3 @@\n-old\n+new",
			"user":      map[string]any{"login": author, "type": "User"},
		},
		"pull_request": map[string]any{
			"number": float64(prNumber),
			"user":   map[string]any{"login": prAuthor, "type": "Bot"},
		},
	}
}

func prRouteDeliveryID(conn, eventKey, stableID string) string {
	return conn + ":github:" + eventKey + ":" + stableID
}

func seedPRMapping(t *testing.T, db *gorm.DB, orgID uuid.UUID, repo string, prNumber int, sessionID uuid.UUID) {
	t.Helper()
	if err := db.Create(&model.GitHubPullRequestSession{
		OrgID: orgID, Repo: repo, PRNumber: prNumber, SessionID: sessionID,
	}).Error; err != nil {
		t.Fatalf("create pr mapping: %v", err)
	}
}

func countSessionQueue(t *testing.T, db *gorm.DB, sessionID uuid.UUID) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&model.SessionMessageQueue{}).Where("session_id = ?", sessionID).Count(&n).Error; err != nil {
		t.Fatalf("count queue: %v", err)
	}
	return n
}

func countAutomatedEvents(t *testing.T, db *gorm.DB, sessionID uuid.UUID) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&model.SessionEvent{}).
		Where("session_id = ? AND event_type = ? AND source = ?", sessionID, runtimeevents.EventUserMessageReceived, triggerConversationSource).
		Count(&n).Error; err != nil {
		t.Fatalf("count automated events: %v", err)
	}
	return n
}

// assertRoutedOnce checks the routed session got exactly one linked transcript
// event + queue row and no agent_triggers were involved.
func assertRoutedOnce(t *testing.T, db *gorm.DB, orgID, sessionID uuid.UUID) {
	t.Helper()
	if q := countSessionQueue(t, db, sessionID); q != 1 {
		t.Fatalf("queued = %d, want 1", q)
	}
	if e := countAutomatedEvents(t, db, sessionID); e != 1 {
		t.Fatalf("automated events = %d, want 1", e)
	}
	var queue model.SessionMessageQueue
	if err := db.Where("session_id = ?", sessionID).First(&queue).Error; err != nil {
		t.Fatalf("load queue: %v", err)
	}
	if queue.SessionEventID == nil {
		t.Fatalf("queue row missing SessionEventID link")
	}
	var triggers int64
	if err := db.Model(&model.AgentTrigger{}).Where("org_id = ?", orgID).Count(&triggers).Error; err != nil {
		t.Fatalf("count triggers: %v", err)
	}
	if triggers != 0 {
		t.Fatalf("agent_triggers = %d, want 0 (auto-route needs none)", triggers)
	}
}

func newPRRouteHandler(db *gorm.DB) *AgentTriggerDispatchHandler {
	return &AgentTriggerDispatchHandler{db: db, enqueuer: &fakeTaskEnqueuer{}}
}

// --- each event type routes into the mapped session, zero triggers installed ---

func TestPRRouteCheckSuiteRoutes(t *testing.T) {
	for _, conclusion := range []string{"success", "failure"} {
		t.Run(conclusion, func(t *testing.T) {
			db := connectTestDB(t)
			org, agent, _ := seedTriggerSessionFixture(t, db)
			orig := seedActiveSession(t, db, org.ID, agent.ID)
			seedPRMapping(t, db, org.ID, "acme/repo", 42, orig.ID)
			h := newPRRouteHandler(db)

			payload := AgentTriggerDispatchPayload{
				Provider: "github-app", EventType: "check_suite", EventAction: "completed",
				OrgID:      org.ID,
				DeliveryID: prRouteDeliveryID("conn-a", "check_suite.completed", "999000111:"+conclusion),
			}
			webhook := githubCheckSuitePayload("acme/repo", []int{42}, conclusion)

			routed, err := h.maybeRoutePREvent(context.Background(), payload, webhook)
			if err != nil {
				t.Fatalf("route: %v", err)
			}
			if _, ok := routed[orig.ID]; !ok {
				t.Fatalf("expected session %s in routed=%v", orig.ID, routed)
			}
			assertRoutedOnce(t, db, org.ID, orig.ID)
		})
	}
}

func TestPRRouteReviewRoutes(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	orig := seedActiveSession(t, db, org.ID, agent.ID)
	seedPRMapping(t, db, org.ID, "acme/repo", 7, orig.ID)
	h := newPRRouteHandler(db)

	payload := AgentTriggerDispatchPayload{
		Provider: "github-app", EventType: "pull_request_review", EventAction: "submitted",
		OrgID:      org.ID,
		DeliveryID: prRouteDeliveryID("conn-a", "pull_request_review.submitted", "777000222"),
	}
	webhook := githubReviewPayload("acme/repo", 7, "human-reviewer", "changes_requested", "please fix the nil check", "usehivy[bot]")

	routed, err := h.maybeRoutePREvent(context.Background(), payload, webhook)
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if _, ok := routed[orig.ID]; !ok {
		t.Fatalf("review not routed: %v", routed)
	}
	assertRoutedOnce(t, db, org.ID, orig.ID)
}

func TestPRRouteReviewCommentRoutes(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	orig := seedActiveSession(t, db, org.ID, agent.ID)
	seedPRMapping(t, db, org.ID, "acme/repo", 11, orig.ID)
	h := newPRRouteHandler(db)

	payload := AgentTriggerDispatchPayload{
		Provider: "github-app", EventType: "pull_request_review_comment", EventAction: "created",
		OrgID:      org.ID,
		DeliveryID: prRouteDeliveryID("conn-a", "pull_request_review_comment.created", "555000333"),
	}
	webhook := githubReviewCommentPayload("acme/repo", 11, "human-reviewer", "this line looks off", "usehivy[bot]")

	routed, err := h.maybeRoutePREvent(context.Background(), payload, webhook)
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if _, ok := routed[orig.ID]; !ok {
		t.Fatalf("review comment not routed: %v", routed)
	}
	assertRoutedOnce(t, db, org.ID, orig.ID)
}

func TestPRRouteIssueCommentRoutes(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	orig := seedActiveSession(t, db, org.ID, agent.ID)
	seedPRMapping(t, db, org.ID, "acme/repo", 42, orig.ID)
	h := newPRRouteHandler(db)

	payload := AgentTriggerDispatchPayload{
		Provider: "github-app", EventType: "issue_comment", EventAction: "created",
		OrgID:      org.ID,
		DeliveryID: prRouteDeliveryID("conn-a", "issue_comment.created", "492700400"),
	}
	webhook := githubIssueCommentPayload("acme/repo", "human-commenter", "can you rebase this?", true)

	routed, err := h.maybeRoutePREvent(context.Background(), payload, webhook)
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if _, ok := routed[orig.ID]; !ok {
		t.Fatalf("issue comment not routed: %v", routed)
	}
	assertRoutedOnce(t, db, org.ID, orig.ID)
}

// A comment on a plain issue (no issue.pull_request) is not a PR event.
func TestPRRouteIssueCommentOnPlainIssueDoesNotRoute(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	orig := seedActiveSession(t, db, org.ID, agent.ID)
	seedPRMapping(t, db, org.ID, "acme/repo", 42, orig.ID)
	h := newPRRouteHandler(db)

	payload := AgentTriggerDispatchPayload{
		Provider: "github-app", EventType: "issue_comment", EventAction: "created",
		OrgID: org.ID, DeliveryID: prRouteDeliveryID("conn-a", "issue_comment.created", "492700400"),
	}
	webhook := githubIssueCommentPayload("acme/repo", "human-commenter", "just an issue", false)

	routed, err := h.maybeRoutePREvent(context.Background(), payload, webhook)
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if len(routed) != 0 {
		t.Fatalf("plain issue routed = %v, want none", routed)
	}
	if q := countSessionQueue(t, db, orig.ID); q != 0 {
		t.Fatalf("queued = %d, want 0", q)
	}
}

// check_suite with empty pull_requests[] (branch push / fork PR) is not routable.
func TestPRRouteCheckSuiteEmptyPullRequestsSkips(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	orig := seedActiveSession(t, db, org.ID, agent.ID)
	seedPRMapping(t, db, org.ID, "acme/repo", 42, orig.ID)
	h := newPRRouteHandler(db)

	payload := AgentTriggerDispatchPayload{
		Provider: "github-app", EventType: "check_suite", EventAction: "completed",
		OrgID: org.ID, DeliveryID: prRouteDeliveryID("conn-a", "check_suite.completed", "999000111:success"),
	}
	webhook := githubCheckSuitePayload("acme/repo", nil, "success")

	routed, err := h.maybeRoutePREvent(context.Background(), payload, webhook)
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if len(routed) != 0 {
		t.Fatalf("empty check_suite routed = %v, want none", routed)
	}
	if q := countSessionQueue(t, db, orig.ID); q != 0 {
		t.Fatalf("queued = %d, want 0", q)
	}
}
