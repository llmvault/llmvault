package tasks

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
)

func newPRAPIRouteHandler(db *gorm.DB, client *nango.Client) *AgentTriggerDispatchHandler {
	return &AgentTriggerDispatchHandler{db: db, enqueuer: &fakeTaskEnqueuer{}, nangoClient: client}
}

func firstQueueMessage(t *testing.T, db *gorm.DB, sessionID uuid.UUID) string {
	t.Helper()
	var q model.SessionMessageQueue
	if err := db.Where("session_id = ?", sessionID).First(&q).Error; err != nil {
		t.Fatalf("load queue message: %v", err)
	}
	return q.MessageText
}

func reviewCommentPayloadWithReview(repo string, prNumber int, author, body, prAuthor string, reviewID, inReplyTo int64) map[string]any {
	wp := githubReviewCommentPayload(repo, prNumber, author, body, prAuthor)
	c := wp["comment"].(map[string]any)
	if reviewID != 0 {
		c["pull_request_review_id"] = float64(reviewID)
	}
	if inReplyTo != 0 {
		c["in_reply_to_id"] = float64(inReplyTo)
	}
	return wp
}

// --- Fix 1: check-suite settling + aggregate summary + head-SHA dedup ---

func TestPRRouteCheckSuiteWaitsUntilSettled(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	orig := seedActiveSession(t, db, org.ID, agent.ID)
	seedPRMapping(t, db, org.ID, "acme/repo", 42, orig.ID)
	connID := seedGitHubConnection(t, db, org.ID)
	client, stub := newGitHubAPIStub(t)
	h := newPRAPIRouteHandler(db, client)

	stub.checkSuites = stubResponse{body: checkSuitesBody(
		githubCheckSuite{ID: 1, Status: "completed", Conclusion: "success", LatestCheckRunsCount: 2},
		githubCheckSuite{ID: 2, Status: "in_progress", LatestCheckRunsCount: 1},
	)}

	payload := AgentTriggerDispatchPayload{
		Provider: "github-app", EventType: "check_suite", EventAction: "completed",
		OrgID: org.ID, ConnectionID: connID,
		DeliveryID: prRouteDeliveryID("conn-a", "check_suite.completed", "1:success"),
	}
	webhook := githubCheckSuitePayload("acme/repo", []int{42}, "success")

	routed, err := h.maybeRoutePREvent(context.Background(), payload, webhook)
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if len(routed) != 0 || countSessionQueue(t, db, orig.ID) != 0 {
		t.Fatalf("unsettled check suite routed=%v queued=%d, want none", routed, countSessionQueue(t, db, orig.ID))
	}

	// Once the last suite finishes, the aggregate settles and the event routes.
	stub.checkSuites = stubResponse{body: checkSuitesBody(
		githubCheckSuite{ID: 1, Status: "completed", Conclusion: "success", LatestCheckRunsCount: 2},
		githubCheckSuite{ID: 2, Status: "completed", Conclusion: "success", LatestCheckRunsCount: 1},
	)}
	if _, err := h.maybeRoutePREvent(context.Background(), payload, webhook); err != nil {
		t.Fatalf("route settled: %v", err)
	}
	assertRoutedOnce(t, db, org.ID, orig.ID)
}

func TestPRRouteCheckSuiteHeadSHADedup(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	orig := seedActiveSession(t, db, org.ID, agent.ID)
	seedPRMapping(t, db, org.ID, "acme/repo", 42, orig.ID)
	connID := seedGitHubConnection(t, db, org.ID)
	client, stub := newGitHubAPIStub(t)
	h := newPRAPIRouteHandler(db, client)
	stub.checkSuites = stubResponse{body: checkSuitesBody(
		githubCheckSuite{ID: 1, Status: "completed", Conclusion: "success", LatestCheckRunsCount: 1},
	)}

	// Two distinct suite completions for the same head SHA (different delivery
	// suffixes) must collapse to one message via the head-SHA event id.
	base := AgentTriggerDispatchPayload{
		Provider: "github-app", EventType: "check_suite", EventAction: "completed",
		OrgID: org.ID, ConnectionID: connID,
	}
	first := base
	first.DeliveryID = prRouteDeliveryID("conn-a", "check_suite.completed", "1:success")
	second := base
	second.DeliveryID = prRouteDeliveryID("conn-a", "check_suite.completed", "2:success")

	webhook := githubCheckSuitePayload("acme/repo", []int{42}, "success")
	if _, err := h.maybeRoutePREvent(context.Background(), first, webhook); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := h.maybeRoutePREvent(context.Background(), second, webhook); err != nil {
		t.Fatalf("second: %v", err)
	}
	assertRoutedOnce(t, db, org.ID, orig.ID)
}

func TestPRRouteCheckSuiteAggregateFailureNotMaskedByGreenSuite(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	orig := seedActiveSession(t, db, org.ID, agent.ID)
	seedPRMapping(t, db, org.ID, "acme/repo", 42, orig.ID)
	connID := seedGitHubConnection(t, db, org.ID)
	client, stub := newGitHubAPIStub(t)
	h := newPRAPIRouteHandler(db, client)
	stub.checkSuites = stubResponse{body: checkSuitesBody(
		githubCheckSuite{ID: 1, Status: "completed", Conclusion: "success", LatestCheckRunsCount: 1},
		githubCheckSuite{ID: 2, Status: "completed", Conclusion: "failure", LatestCheckRunsCount: 1},
	)}

	// The triggering webhook is the GREEN suite, but the aggregate is red.
	payload := AgentTriggerDispatchPayload{
		Provider: "github-app", EventType: "check_suite", EventAction: "completed",
		OrgID: org.ID, ConnectionID: connID,
		DeliveryID: prRouteDeliveryID("conn-a", "check_suite.completed", "1:success"),
	}
	if _, err := h.maybeRoutePREvent(context.Background(), payload, githubCheckSuitePayload("acme/repo", []int{42}, "success")); err != nil {
		t.Fatalf("route: %v", err)
	}
	msg := firstQueueMessage(t, db, orig.ID)
	if !strings.Contains(msg, "overall: failure") || !strings.Contains(msg, "did not all pass") {
		t.Fatalf("aggregate failure not surfaced in message:\n%s", msg)
	}
}

// --- Fix 2a: review aggregation renders inline comments ---

func TestPRRouteReviewSubmittedRendersInlineComments(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	orig := seedActiveSession(t, db, org.ID, agent.ID)
	seedPRMapping(t, db, org.ID, "acme/repo", 7, orig.ID)
	connID := seedGitHubConnection(t, db, org.ID)
	client, stub := newGitHubAPIStub(t)
	h := newPRAPIRouteHandler(db, client)
	stub.permission["human-reviewer"] = stubResponse{body: `{"permission":"write","role_name":"write"}`}
	stub.reviewCmts = stubResponse{body: string(readGitHubRESTFixture(t, "rest_review_comments.list.json"))}

	// Empty review body — the reviewer only left inline comments.
	payload := AgentTriggerDispatchPayload{
		Provider: "github-app", EventType: "pull_request_review", EventAction: "submitted",
		OrgID: org.ID, ConnectionID: connID,
		DeliveryID: prRouteDeliveryID("conn-a", "pull_request_review.submitted", "777000222"),
	}
	webhook := githubReviewPayload("acme/repo", 7, "human-reviewer", "changes_requested", "", "usehivy[bot]")
	if _, err := h.maybeRoutePREvent(context.Background(), payload, webhook); err != nil {
		t.Fatalf("route: %v", err)
	}
	msg := firstQueueMessage(t, db, orig.ID)
	if !strings.Contains(msg, "Inline comments:") || !strings.Contains(msg, "file1.txt:2") ||
		!strings.Contains(msg, "Great stuff!") || !strings.Contains(msg, "Address the review feedback") {
		t.Fatalf("inline comments not rendered:\n%s", msg)
	}
}

func TestPRRouteReviewSubmittedEmptyOmitsGuidance(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	orig := seedActiveSession(t, db, org.ID, agent.ID)
	seedPRMapping(t, db, org.ID, "acme/repo", 7, orig.ID)
	connID := seedGitHubConnection(t, db, org.ID)
	client, stub := newGitHubAPIStub(t)
	h := newPRAPIRouteHandler(db, client)
	stub.permission["human-reviewer"] = stubResponse{body: `{"permission":"write","role_name":"write"}`}
	stub.reviewCmts = stubResponse{body: `[]`}

	payload := AgentTriggerDispatchPayload{
		Provider: "github-app", EventType: "pull_request_review", EventAction: "submitted",
		OrgID: org.ID, ConnectionID: connID,
		DeliveryID: prRouteDeliveryID("conn-a", "pull_request_review.submitted", "777000222"),
	}
	webhook := githubReviewPayload("acme/repo", 7, "human-reviewer", "commented", "", "usehivy[bot]")
	if _, err := h.maybeRoutePREvent(context.Background(), payload, webhook); err != nil {
		t.Fatalf("route: %v", err)
	}
	msg := firstQueueMessage(t, db, orig.ID)
	if strings.Contains(msg, "Address the review feedback") {
		t.Fatalf("empty review must not tell the agent to address feedback:\n%s", msg)
	}
	if !strings.Contains(msg, "no written feedback") {
		t.Fatalf("empty review guidance missing:\n%s", msg)
	}
}

func TestPRRouteReviewSubmittedBatchesForThirtySeconds(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	orig := seedActiveSession(t, db, org.ID, agent.ID)
	seedPRMapping(t, db, org.ID, "acme/repo", 7, orig.ID)
	connID := seedGitHubConnection(t, db, org.ID)
	client, stub := newGitHubAPIStub(t)
	enq := &fakeTaskEnqueuer{}
	h := &AgentTriggerDispatchHandler{db: db, enqueuer: enq, nangoClient: client}
	stub.permission["human-reviewer"] = stubResponse{body: `{"permission":"write","role_name":"write"}`}
	stub.reviewCmts = stubResponse{body: `[]`}

	for _, reviewID := range []string{"777000221", "777000222", "777000223"} {
		payload := AgentTriggerDispatchPayload{
			Provider: "github-app", EventType: "pull_request_review", EventAction: "submitted",
			OrgID: org.ID, ConnectionID: connID,
			DeliveryID: prRouteDeliveryID("conn-a", "pull_request_review.submitted", reviewID),
		}
		webhook := githubReviewPayload("acme/repo", 7, "human-reviewer", "commented", "review "+reviewID, "usehivy[bot]")
		if _, err := h.maybeRoutePREvent(context.Background(), payload, webhook); err != nil {
			t.Fatalf("route review %s: %v", reviewID, err)
		}
	}

	var queued []model.SessionMessageQueue
	if err := db.Where("session_id = ?", orig.ID).Find(&queued).Error; err != nil {
		t.Fatalf("load review batch queue: %v", err)
	}
	if len(queued) != 1 {
		t.Fatalf("review batch queue rows = %d, want 1", len(queued))
	}
	if queued[0].Status != reviewBatchBufferingStatus {
		t.Fatalf("review batch status = %q, want %q", queued[0].Status, reviewBatchBufferingStatus)
	}
	for _, reviewID := range []string{"777000221", "777000222", "777000223"} {
		if !strings.Contains(queued[0].MessageText, "review "+reviewID) {
			t.Fatalf("batched text missing review %s: %s", reviewID, queued[0].MessageText)
		}
	}
	if n := countTasksOfType(enq, TypeSessionReviewBatchFlush); n != 3 {
		t.Fatalf("review batch flush tasks = %d, want 3 idempotent timers", n)
	}
	if n := countTasksOfType(enq, TypeSessionMessageDeliver); n != 0 {
		t.Fatalf("message delivery started before debounce: %d", n)
	}

	flushTask, _, err := NewSessionReviewBatchFlushTask(SessionReviewBatchFlushPayload{QueueID: queued[0].ID})
	if err != nil {
		t.Fatalf("build flush task: %v", err)
	}
	if err := NewSessionReviewBatchFlushHandler(db, enq).Handle(context.Background(), flushTask); err != nil {
		t.Fatalf("flush review batch: %v", err)
	}
	if err := db.First(&queued[0], "id = ?", queued[0].ID).Error; err != nil {
		t.Fatalf("reload review batch: %v", err)
	}
	if queued[0].Status != "pending" {
		t.Fatalf("flushed review batch status = %q, want pending", queued[0].Status)
	}
	if n := countTasksOfType(enq, TypeSessionMessageDeliver); n != 1 {
		t.Fatalf("message delivery tasks after flush = %d, want 1", n)
	}

	redelivery := AgentTriggerDispatchPayload{
		Provider: "github-app", EventType: "pull_request_review", EventAction: "submitted",
		OrgID: org.ID, ConnectionID: connID,
		DeliveryID: prRouteDeliveryID("conn-a", "pull_request_review.submitted", "777000223"),
	}
	if _, err := h.maybeRoutePREvent(context.Background(), redelivery, githubReviewPayload("acme/repo", 7, "human-reviewer", "commented", "review 777000223", "usehivy[bot]")); err != nil {
		t.Fatalf("redeliver flushed review: %v", err)
	}
	var afterRedelivery int64
	if err := db.Model(&model.SessionMessageQueue{}).Where("session_id = ?", orig.ID).Count(&afterRedelivery).Error; err != nil {
		t.Fatalf("count review queue after redelivery: %v", err)
	}
	if afterRedelivery != 1 {
		t.Fatalf("review queue rows after redelivery = %d, want 1", afterRedelivery)
	}
}

// --- Fix 2b: review-bound comment suppression, thread replies still route ---

func TestPRRouteReviewCommentSuppressedWhenReviewBound(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	orig := seedActiveSession(t, db, org.ID, agent.ID)
	seedPRMapping(t, db, org.ID, "acme/repo", 11, orig.ID)
	connID := seedGitHubConnection(t, db, org.ID)
	client, stub := newGitHubAPIStub(t)
	h := newPRAPIRouteHandler(db, client)
	stub.permission["human-reviewer"] = stubResponse{body: `{"permission":"write","role_name":"write"}`}

	payload := AgentTriggerDispatchPayload{
		Provider: "github-app", EventType: "pull_request_review_comment", EventAction: "created",
		OrgID: org.ID, ConnectionID: connID,
		DeliveryID: prRouteDeliveryID("conn-a", "pull_request_review_comment.created", "555000333"),
	}
	webhook := reviewCommentPayloadWithReview("acme/repo", 11, "human-reviewer", "nit", "usehivy[bot]", 42, 0)
	routed, err := h.maybeRoutePREvent(context.Background(), payload, webhook)
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if len(routed) != 0 || countSessionQueue(t, db, orig.ID) != 0 {
		t.Fatalf("review-bound comment routed=%v queued=%d, want none", routed, countSessionQueue(t, db, orig.ID))
	}
}

func TestPRRouteReviewCommentReplyStillRoutes(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	orig := seedActiveSession(t, db, org.ID, agent.ID)
	seedPRMapping(t, db, org.ID, "acme/repo", 11, orig.ID)
	connID := seedGitHubConnection(t, db, org.ID)
	client, stub := newGitHubAPIStub(t)
	h := newPRAPIRouteHandler(db, client)
	stub.permission["human-reviewer"] = stubResponse{body: `{"permission":"write","role_name":"write"}`}

	payload := AgentTriggerDispatchPayload{
		Provider: "github-app", EventType: "pull_request_review_comment", EventAction: "created",
		OrgID: org.ID, ConnectionID: connID,
		DeliveryID: prRouteDeliveryID("conn-a", "pull_request_review_comment.created", "555000333"),
	}
	webhook := reviewCommentPayloadWithReview("acme/repo", 11, "human-reviewer", "replying", "usehivy[bot]", 42, 500)
	routed, err := h.maybeRoutePREvent(context.Background(), payload, webhook)
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if _, ok := routed[orig.ID]; !ok {
		t.Fatalf("thread reply not routed: %v", routed)
	}
}
