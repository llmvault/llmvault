package tasks_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
	"github.com/usehivy/hivy/internal/testdb"
)

func connectFailedEventsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := testdb.DatabaseURL("DATABASE_URL", "HIVY_DATABASE_URL", "TEST_DATABASE_URL")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect Postgres: %v", err)
	}
	sqlDB, _ := db.DB()
	testdb.ApplyMigrations(t, db)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func TestPersistTerminalFailure_InsertsRow(t *testing.T) {
	db := connectFailedEventsTestDB(t)
	orgID := uuid.New()
	triggerID := uuid.New()
	payload, _ := json.Marshal(map[string]any{"k": "v"})

	err := tasks.PersistTerminalFailure(context.Background(), db, tasks.FailedEventInput{
		OrgID:        orgID,
		TriggerID:    triggerID,
		EventType:    tasks.TypeAgentTriggerDispatch,
		Payload:      payload,
		Err:          errors.New("boom"),
		AttemptCount: 2,
	})
	if err != nil {
		t.Fatalf("PersistTerminalFailure: %v", err)
	}

	var row model.FailedEvent
	if err := db.Where("org_id = ? AND trigger_id = ?", orgID, triggerID).First(&row).Error; err != nil {
		t.Fatalf("load row: %v", err)
	}
	if row.EventType != tasks.TypeAgentTriggerDispatch {
		t.Errorf("EventType = %q", row.EventType)
	}
	if row.Status != model.FailedEventStatusPending {
		t.Errorf("Status = %q, want pending", row.Status)
	}
	if row.AttemptCount != 2 {
		t.Errorf("AttemptCount = %d", row.AttemptCount)
	}
	if row.Error != "boom" {
		t.Errorf("Error = %q", row.Error)
	}
	var got map[string]any
	if err := json.Unmarshal(row.Payload, &got); err != nil {
		t.Fatalf("unmarshal stored payload: %v", err)
	}
	if got["k"] != "v" {
		t.Errorf("Payload[k] = %v, want v", got["k"])
	}
}

func TestRetryFailedEvent_EnqueuesAndMarksRetried(t *testing.T) {
	db := connectFailedEventsTestDB(t)
	orgID := uuid.New()
	triggerID := uuid.New()
	original := tasks.AgentTriggerDispatchPayload{
		OrgID:      orgID,
		TriggerID:  &triggerID,
		DeliveryID: "delivery-1",
	}
	payload, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := tasks.PersistTerminalFailure(context.Background(), db, tasks.FailedEventInput{
		OrgID:        orgID,
		TriggerID:    triggerID,
		EventType:    tasks.TypeAgentTriggerDispatch,
		Payload:      payload,
		Err:          errors.New("upstream down"),
		AttemptCount: 2,
	}); err != nil {
		t.Fatalf("PersistTerminalFailure: %v", err)
	}

	var row model.FailedEvent
	if err := db.Where("trigger_id = ?", triggerID).First(&row).Error; err != nil {
		t.Fatalf("load row: %v", err)
	}

	enqueuer := &enqueue.MockClient{}
	info, err := tasks.RetryFailedEvent(context.Background(), db, enqueuer, row.ID)
	if err != nil {
		t.Fatalf("RetryFailedEvent: %v", err)
	}
	if info == nil {
		t.Fatal("expected TaskInfo")
	}
	enqueuer.AssertEnqueued(t, tasks.TypeAgentTriggerDispatch)

	// the retry must re-apply the task's original critical-queue option
	// (passed at enqueue time) instead of falling back to asynq defaults.
	enqueuedTasks := enqueuer.Tasks()
	if len(enqueuedTasks) != 1 {
		t.Fatalf("enqueued %d tasks, want 1", len(enqueuedTasks))
	}
	var hasCriticalQueue bool
	for _, opt := range enqueuedTasks[0].Options {
		if opt.Type() == asynq.QueueOpt {
			if name, ok := opt.Value().(string); ok && name == tasks.QueueCritical {
				hasCriticalQueue = true
			}
		}
	}
	if !hasCriticalQueue {
		t.Fatalf("retry did not forward the critical queue option: %#v", enqueuedTasks[0].Options)
	}

	var reloaded model.FailedEvent
	if err := db.Where("id = ?", row.ID).First(&reloaded).Error; err != nil {
		t.Fatalf("reload row: %v", err)
	}
	if reloaded.Status != model.FailedEventStatusRetried {
		t.Errorf("Status = %q, want retried", reloaded.Status)
	}
	if reloaded.RetriedAt == nil {
		t.Error("RetriedAt is nil")
	}
}

// Two retries of the same pending row racing must enqueue exactly one task: the
// CAS claim lets one win, the others observe ErrFailedEventNotPending.
func TestRetryFailedEvent_ConcurrentRetriesEnqueueOnce(t *testing.T) {
	db := connectFailedEventsTestDB(t)
	orgID := uuid.New()
	triggerID := uuid.New()
	payload, _ := json.Marshal(tasks.AgentTriggerDispatchPayload{
		OrgID:     orgID,
		TriggerID: &triggerID,
	})
	if err := tasks.PersistTerminalFailure(context.Background(), db, tasks.FailedEventInput{
		OrgID:        orgID,
		TriggerID:    triggerID,
		EventType:    tasks.TypeAgentTriggerDispatch,
		Payload:      payload,
		Err:          errors.New("upstream down"),
		AttemptCount: 1,
	}); err != nil {
		t.Fatalf("PersistTerminalFailure: %v", err)
	}
	var row model.FailedEvent
	if err := db.Where("trigger_id = ?", triggerID).First(&row).Error; err != nil {
		t.Fatalf("load row: %v", err)
	}

	enqueuer := &enqueue.MockClient{}

	const n = 6
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		ok       int
		notPend  int
		otherErr error
	)
	start := make(chan struct{})
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := tasks.RetryFailedEvent(context.Background(), db, enqueuer, row.ID)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case errors.Is(err, tasks.ErrFailedEventNotPending):
				notPend++
			default:
				otherErr = err
			}
		}()
	}
	close(start)
	wg.Wait()

	if otherErr != nil {
		t.Fatalf("unexpected error from concurrent retry: %v", otherErr)
	}
	if ok != 1 {
		t.Fatalf("expected exactly 1 successful retry, got %d", ok)
	}
	if notPend != n-1 {
		t.Fatalf("expected %d ErrFailedEventNotPending, got %d", n-1, notPend)
	}
	if got := len(enqueuer.Tasks()); got != 1 {
		t.Fatalf("enqueued %d tasks, want 1", got)
	}

	var reloaded model.FailedEvent
	if err := db.Where("id = ?", row.ID).First(&reloaded).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Status != model.FailedEventStatusRetried {
		t.Fatalf("status = %q, want retried", reloaded.Status)
	}
	if reloaded.RetriedAt == nil {
		t.Fatal("retried_at not recorded on the winning retry")
	}
}

func TestRetryFailedEvent_RejectsAlreadyRetried(t *testing.T) {
	db := connectFailedEventsTestDB(t)
	orgID := uuid.New()
	triggerID := uuid.New()
	payload, _ := json.Marshal(tasks.AgentTriggerDispatchPayload{
		OrgID:     orgID,
		TriggerID: &triggerID,
	})
	if err := tasks.PersistTerminalFailure(context.Background(), db, tasks.FailedEventInput{
		OrgID:        orgID,
		TriggerID:    triggerID,
		EventType:    tasks.TypeAgentTriggerDispatch,
		Payload:      payload,
		Err:          errors.New("upstream down"),
		AttemptCount: 2,
	}); err != nil {
		t.Fatalf("PersistTerminalFailure: %v", err)
	}
	var row model.FailedEvent
	if err := db.Where("trigger_id = ?", triggerID).First(&row).Error; err != nil {
		t.Fatalf("load row: %v", err)
	}

	enqueuer := &enqueue.MockClient{}
	if _, err := tasks.RetryFailedEvent(context.Background(), db, enqueuer, row.ID); err != nil {
		t.Fatalf("first retry: %v", err)
	}
	if _, err := tasks.RetryFailedEvent(context.Background(), db, enqueuer, row.ID); !errors.Is(err, tasks.ErrFailedEventNotPending) {
		t.Fatalf("second retry err = %v, want ErrFailedEventNotPending", err)
	}
	if got := len(enqueuer.Tasks()); got != 1 {
		t.Errorf("enqueued %d tasks, want 1", got)
	}
}
