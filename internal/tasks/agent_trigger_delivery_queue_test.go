package tasks

import (
	"context"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/model"
)

func triggerDispatchPayload(deliveryID string) AgentTriggerDispatchPayload {
	return AgentTriggerDispatchPayload{
		Provider:    "github-app",
		EventType:   "issue_comment",
		EventAction: "created",
		DeliveryID:  deliveryID,
		PayloadJSON: []byte("{}"),
	}
}

func compiledMessage(resourceKey, text string) compiledTriggerMessage {
	return compiledTriggerMessage{
		Text:        text,
		ResourceKey: resourceKey,
		Raw:         map[string]any{"resource_key": resourceKey},
	}
}

func countTasksOfType(enq *fakeTaskEnqueuer, taskType string) int {
	n := 0
	for _, task := range enq.tasks {
		if task.Type() == taskType {
			n++
		}
	}
	return n
}

// Follow-up mentions on the same issue/PR must land in the same session and
// queue in order; a different resource (a PR vs the issue) gets its own session;
// a redelivered event dedups to a single queued message.
func TestTriggerDeliverySameResourceSharesSessionAndQueues(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	queueChannel := seedTriggerChannel(t, db, org.ID, agent.ID, "queue")
	trigger := seedTriggerForSession(t, db, org.ID, agent.ID, &queueChannel.ID)
	enq := &fakeTaskEnqueuer{}
	handler := &AgentTriggerDispatchHandler{db: db, enqueuer: enq}
	ctx := context.Background()

	issueKey := "github/acme/repo/issue/42"
	p1 := triggerDispatchPayload("d1")
	p1.OrgID = org.ID
	if err := handler.deliverCompiled(ctx, p1, trigger, agent, compiledMessage(issueKey, "first mention")); err != nil {
		t.Fatalf("first deliver: %v", err)
	}
	p2 := triggerDispatchPayload("d2")
	p2.OrgID = org.ID
	if err := handler.deliverCompiled(ctx, p2, trigger, agent, compiledMessage(issueKey, "second mention")); err != nil {
		t.Fatalf("second deliver: %v", err)
	}

	// Both mentions resolve to one session for the issue.
	var sessions []model.Session
	if err := db.Where("org_id = ? AND source = ? AND source_resource_key = ?", org.ID, triggerConversationSource, issueKey).Find(&sessions).Error; err != nil {
		t.Fatalf("load sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("issue sessions = %d, want 1", len(sessions))
	}
	issueSession := sessions[0]

	// Two queued messages, in order (sequence 1, 2), delivered by the generic
	// session queue rather than a direct post.
	var queued []model.SessionMessageQueue
	if err := db.Where("session_id = ?", issueSession.ID).Order("sequence_number ASC").Find(&queued).Error; err != nil {
		t.Fatalf("load queue: %v", err)
	}
	if len(queued) != 2 {
		t.Fatalf("queued messages = %d, want 2", len(queued))
	}
	if queued[0].SequenceNumber != 1 || queued[1].SequenceNumber != 2 {
		t.Fatalf("sequences = %d,%d want 1,2", queued[0].SequenceNumber, queued[1].SequenceNumber)
	}
	if queued[0].MessageText != "first mention" || queued[1].MessageText != "second mention" {
		t.Fatalf("queued text = %q,%q", queued[0].MessageText, queued[1].MessageText)
	}

	// Delivered trigger messages now persist to the transcript (visible in the
	// UI and to the naming task) with each queue row linked to its event.
	var events []model.SessionEvent
	if err := db.Where("session_id = ? AND event_type = ? AND source = ?",
		issueSession.ID, "user.message.received", triggerConversationSource).
		Find(&events).Error; err != nil {
		t.Fatalf("load events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("transcript events = %d, want 2", len(events))
	}
	for _, q := range queued {
		if q.SessionEventID == nil {
			t.Fatalf("queue row %d missing SessionEventID link", q.SequenceNumber)
		}
	}

	// Redelivery of the first event (same delivery id) dedups: no third row.
	if err := handler.deliverCompiled(ctx, p1, trigger, agent, compiledMessage(issueKey, "first mention")); err != nil {
		t.Fatalf("redeliver: %v", err)
	}
	var afterRedelivery int64
	if err := db.Model(&model.SessionMessageQueue{}).Where("session_id = ?", issueSession.ID).Count(&afterRedelivery).Error; err != nil {
		t.Fatalf("count after redelivery: %v", err)
	}
	if afterRedelivery != 2 {
		t.Fatalf("queued after redelivery = %d, want 2", afterRedelivery)
	}

	// A pull request with the same number is a distinct resource → distinct session.
	prKey := "github/acme/repo/pull/42"
	p3 := triggerDispatchPayload("d3")
	p3.OrgID = org.ID
	if err := handler.deliverCompiled(ctx, p3, trigger, agent, compiledMessage(prKey, "pr mention")); err != nil {
		t.Fatalf("pr deliver: %v", err)
	}
	var prSessions int64
	if err := db.Model(&model.Session{}).Where("org_id = ? AND source_resource_key = ?", org.ID, prKey).Count(&prSessions).Error; err != nil {
		t.Fatalf("count pr sessions: %v", err)
	}
	if prSessions != 1 {
		t.Fatalf("pr sessions = %d, want 1", prSessions)
	}

	// Every delivery fires the generic drain (idempotent, fire-and-forget).
	if n := countTasksOfType(enq, TypeSessionMessageDeliver); n < 3 {
		t.Fatalf("session message deliver enqueues = %d, want >= 3", n)
	}
}

// A mention arriving while the agent's turn is active must queue behind it
// rather than collide — the delivery path never posts directly.
func TestTriggerDeliveryQueuesWhileTurnActive(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	queueChannel := seedTriggerChannel(t, db, org.ID, agent.ID, "queue")
	trigger := seedTriggerForSession(t, db, org.ID, agent.ID, &queueChannel.ID)
	enq := &fakeTaskEnqueuer{}
	handler := &AgentTriggerDispatchHandler{db: db, enqueuer: enq}
	ctx := context.Background()
	issueKey := "github/acme/repo/issue/7"

	// Seed the session with an active turn.
	p1 := triggerDispatchPayload("d1")
	p1.OrgID = org.ID
	session, err := handler.findOrCreateTriggerSession(ctx, &agent, trigger, issueKey)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	now := time.Now().UTC()
	if err := db.Model(&model.Session{}).Where("id = ?", session.ID).Updates(map[string]any{
		"agent_turn_status":     model.SessionAgentTurnActive,
		"agent_turn_started_at": now,
	}).Error; err != nil {
		t.Fatalf("mark turn active: %v", err)
	}

	if err := handler.deliverCompiled(ctx, p1, trigger, agent, compiledMessage(issueKey, "mid-turn mention")); err != nil {
		t.Fatalf("deliver: %v", err)
	}

	// The turn is untouched (still active) and the message is queued for the drain.
	var reloaded model.Session
	if err := db.First(&reloaded, "id = ?", session.ID).Error; err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if reloaded.AgentTurnStatus != model.SessionAgentTurnActive {
		t.Fatalf("turn status = %q, want active (delivery must not post directly)", reloaded.AgentTurnStatus)
	}
	var queued int64
	if err := db.Model(&model.SessionMessageQueue{}).Where("session_id = ?", session.ID).Count(&queued).Error; err != nil {
		t.Fatalf("count queue: %v", err)
	}
	if queued != 1 {
		t.Fatalf("queued = %d, want 1", queued)
	}
}
