package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	coremodel "github.com/usehivy/hivy/internal/model"
	ragmodel "github.com/usehivy/hivy/internal/rag/model"
	ragtasks "github.com/usehivy/hivy/internal/rag/tasks"
)

type ragQueueSpy struct {
	events     []string
	enqueued   []enqueue.EnqueuedTask
	deleted    []enqueue.DeletedTask
	enqueueErr error
	deleteErr  error
}

func (q *ragQueueSpy) Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	return q.EnqueueContext(context.Background(), task, opts...)
}

func (q *ragQueueSpy) EnqueueContext(_ context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	q.events = append(q.events, "enqueue")
	q.enqueued = append(q.enqueued, enqueue.EnqueuedTask{
		TypeName: task.Type(),
		Payload:  append([]byte(nil), task.Payload()...),
		Options:  append([]asynq.Option(nil), opts...),
	})
	if q.enqueueErr != nil {
		return nil, q.enqueueErr
	}
	return &asynq.TaskInfo{}, nil
}

func (q *ragQueueSpy) DeleteTask(queue, id string) error {
	q.events = append(q.events, "cleanup")
	q.deleted = append(q.deleted, enqueue.DeletedTask{Queue: queue, ID: id})
	return q.deleteErr
}

func (q *ragQueueSpy) Close() error { return nil }

func taskIDOption(opts []asynq.Option) string {
	for _, opt := range opts {
		if opt.Type() == asynq.TaskIDOpt {
			value, _ := opt.Value().(string)
			return value
		}
	}
	return ""
}

func createRAGActionSource(
	t *testing.T,
	db *gorm.DB,
	orgID uuid.UUID,
	status ragmodel.RAGSourceStatus,
) ragmodel.RAGSource {
	t.Helper()
	src := ragmodel.RAGSource{
		ID:          uuid.New(),
		OrgIDValue:  orgID,
		KindValue:   ragmodel.RAGSourceKindWebsite,
		Name:        "Product docs",
		Status:      status,
		Enabled:     true,
		ConfigValue: coremodel.JSON{"urls": []any{"https://example.com/docs"}},
	}
	if err := db.Create(&src).Error; err != nil {
		t.Fatalf("create rag source: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", src.ID).Delete(&ragmodel.RAGSource{})
	})
	return src
}

func callRAGAction(
	t *testing.T,
	db *gorm.DB,
	org *coremodel.Org,
	sourceID uuid.UUID,
	handle func(http.ResponseWriter, *http.Request),
) *httptest.ResponseRecorder {
	t.Helper()
	user := createTestUser(t, db, "rag-action-"+uuid.NewString()+"@example.com")
	addTestOrgOwner(t, db, org.ID, user.ID)
	req := httptest.NewRequest(http.MethodPost, "/v1/rag/sources/"+sourceID.String(), nil)
	req = middleware.WithOrg(req, org)
	req = middleware.WithUser(req, &user)
	req = withChiURLParam(req, "id", sourceID.String())
	rr := httptest.NewRecorder()
	handle(rr, req)
	return rr
}

func TestRetryIngestionCleansFailedJobBeforeDispatchingNewAttempt(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	src := createRAGActionSource(t, db, org.ID, ragmodel.RAGSourceStatusError)
	if err := db.Model(&src).Updates(map[string]any{
		"enabled":                 false,
		"in_repeated_error_state": true,
	}).Error; err != nil {
		t.Fatalf("mark source failed: %v", err)
	}

	message := "upstream connection reset"
	failed := ragmodel.RAGIndexAttempt{
		ID:          uuid.New(),
		OrgID:       org.ID,
		RAGSourceID: src.ID,
		Status:      ragmodel.IndexingStatusFailed,
		ErrorMsg:    &message,
		TimeCreated: time.Now().Add(-time.Minute),
		TimeUpdated: time.Now().Add(-time.Minute),
	}
	if err := db.Create(&failed).Error; err != nil {
		t.Fatalf("create failed attempt: %v", err)
	}

	queue := &ragQueueSpy{}
	h := handler.NewRAGSourceHandler(db, queue, nil, nil, nil, nil)
	rr := callRAGAction(t, db, &org, src.ID, h.RetryIngestion)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("retry status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if len(queue.events) != 2 || queue.events[0] != "cleanup" || queue.events[1] != "enqueue" {
		t.Fatalf("queue events = %v, want [cleanup enqueue]", queue.events)
	}
	if len(queue.deleted) != 1 || queue.deleted[0].Queue != ragtasks.QueueRagWork || queue.deleted[0].ID != ragtasks.IngestTaskID(src.ID) {
		t.Fatalf("deleted tasks = %+v", queue.deleted)
	}
	if len(queue.enqueued) != 1 || queue.enqueued[0].TypeName != ragtasks.TypeRagIngest {
		t.Fatalf("enqueued tasks = %+v", queue.enqueued)
	}
	if got := taskIDOption(queue.enqueued[0].Options); got != ragtasks.IngestTaskID(src.ID) {
		t.Fatalf("enqueued task id = %q, want %q", got, ragtasks.IngestTaskID(src.ID))
	}
	payload, err := ragtasks.UnmarshalIngest(queue.enqueued[0].Payload)
	if err != nil {
		t.Fatalf("decode retry payload: %v", err)
	}
	if payload.RAGSourceID != src.ID || payload.AttemptID == nil {
		t.Fatalf("retry payload = %+v", payload)
	}

	var attempts []ragmodel.RAGIndexAttempt
	if err := db.Where("rag_source_id = ?", src.ID).Order("time_created ASC").Find(&attempts).Error; err != nil {
		t.Fatalf("load attempts: %v", err)
	}
	if len(attempts) != 2 || attempts[0].ID != failed.ID || attempts[1].ID != *payload.AttemptID {
		t.Fatalf("attempt history = %+v", attempts)
	}
	if attempts[1].Status != ragmodel.IndexingStatusNotStarted {
		t.Fatalf("replacement status = %q, want not_started", attempts[1].Status)
	}

	var reloaded ragmodel.RAGSource
	if err := db.First(&reloaded, "id = ?", src.ID).Error; err != nil {
		t.Fatalf("reload source: %v", err)
	}
	if !reloaded.Enabled || reloaded.InRepeatedErrorState || reloaded.Status != ragmodel.RAGSourceStatusInitialIndexing {
		t.Fatalf("source after retry = enabled:%v repeated:%v status:%q", reloaded.Enabled, reloaded.InRepeatedErrorState, reloaded.Status)
	}

	var response struct {
		AttemptID string `json:"attempt_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.AttemptID != payload.AttemptID.String() {
		t.Fatalf("response attempt_id = %q, want %q", response.AttemptID, payload.AttemptID.String())
	}
}

func TestResumeIngestionActivatesPausedSourceAndQueuesFromCheckpoint(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	src := createRAGActionSource(t, db, org.ID, ragmodel.RAGSourceStatusPaused)
	checkpoint := `{"cursor":"next-page"}`
	previous := ragmodel.RAGIndexAttempt{
		ID:                uuid.New(),
		OrgID:             org.ID,
		RAGSourceID:       src.ID,
		Status:            ragmodel.IndexingStatusSuccess,
		CheckpointPointer: &checkpoint,
		TimeCreated:       time.Now().Add(-time.Hour),
		TimeUpdated:       time.Now().Add(-time.Hour),
	}
	if err := db.Create(&previous).Error; err != nil {
		t.Fatalf("create previous attempt: %v", err)
	}

	queue := &ragQueueSpy{}
	h := handler.NewRAGSourceHandler(db, queue, nil, nil, nil, nil)
	rr := callRAGAction(t, db, &org, src.ID, h.ResumeIngestion)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("resume status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if len(queue.events) != 1 || queue.events[0] != "enqueue" {
		t.Fatalf("queue events = %v, want [enqueue]", queue.events)
	}

	var reloaded ragmodel.RAGSource
	if err := db.First(&reloaded, "id = ?", src.ID).Error; err != nil {
		t.Fatalf("reload source: %v", err)
	}
	if reloaded.Status != ragmodel.RAGSourceStatusActive || !reloaded.Enabled {
		t.Fatalf("source after resume = enabled:%v status:%q", reloaded.Enabled, reloaded.Status)
	}

	var latest ragmodel.RAGIndexAttempt
	if err := db.Where("rag_source_id = ?", src.ID).Order("time_created DESC, id DESC").First(&latest).Error; err != nil {
		t.Fatalf("load resumed attempt: %v", err)
	}
	if latest.Status != ragmodel.IndexingStatusNotStarted || latest.CheckpointPointer == nil || *latest.CheckpointPointer != checkpoint {
		t.Fatalf("resumed attempt = status:%q checkpoint:%v", latest.Status, latest.CheckpointPointer)
	}
}

func TestRetryIngestionDoesNotDispatchWhenFailedJobCleanupFails(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	src := createRAGActionSource(t, db, org.ID, ragmodel.RAGSourceStatusError)
	failed := ragmodel.RAGIndexAttempt{
		ID:          uuid.New(),
		OrgID:       org.ID,
		RAGSourceID: src.ID,
		Status:      ragmodel.IndexingStatusFailed,
		TimeCreated: time.Now(),
		TimeUpdated: time.Now(),
	}
	if err := db.Create(&failed).Error; err != nil {
		t.Fatalf("create failed attempt: %v", err)
	}

	queue := &ragQueueSpy{deleteErr: errors.New("redis unavailable")}
	h := handler.NewRAGSourceHandler(db, queue, nil, nil, nil, nil)
	rr := callRAGAction(t, db, &org, src.ID, h.RetryIngestion)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("retry status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if len(queue.events) != 1 || queue.events[0] != "cleanup" {
		t.Fatalf("queue events = %v, want cleanup only", queue.events)
	}
	var count int64
	if err := db.Model(&ragmodel.RAGIndexAttempt{}).Where("rag_source_id = ?", src.ID).Count(&count).Error; err != nil {
		t.Fatalf("count attempts: %v", err)
	}
	if count != 1 {
		t.Fatalf("attempt count = %d, want 1", count)
	}
}

func TestRetryIngestionRejectsNonManagerInHandler(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	src := createRAGActionSource(t, db, org.ID, ragmodel.RAGSourceStatusError)
	user := createTestUser(t, db, "rag-member-"+uuid.NewString()+"@example.com")
	membership := coremodel.OrgMembership{OrgID: org.ID, UserID: user.ID, Role: "member"}
	if err := db.Create(&membership).Error; err != nil {
		t.Fatalf("create member: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ? AND user_id = ?", org.ID, user.ID).Delete(&coremodel.OrgMembership{})
	})

	queue := &ragQueueSpy{}
	h := handler.NewRAGSourceHandler(db, queue, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/rag/sources/"+src.ID.String()+"/retry", nil)
	req = middleware.WithOrg(req, &org)
	req = middleware.WithUser(req, &user)
	req = withChiURLParam(req, "id", src.ID.String())
	rr := httptest.NewRecorder()
	h.RetryIngestion(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("retry status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if len(queue.events) != 0 {
		t.Fatalf("queue events = %v, want none", queue.events)
	}
}
