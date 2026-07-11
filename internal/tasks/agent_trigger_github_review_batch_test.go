package tasks

import (
	"context"
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

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
