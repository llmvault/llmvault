package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sheets"
	"github.com/usehivy/hivy/internal/storage"
)

// SheetCSVImportPayload is the payload for TypeSheetCSVImport tasks (IDs only).
type SheetCSVImportPayload struct {
	JobID uuid.UUID `json:"job_id"`
}

func init() {
	RegisterTaskBuilder(TypeSheetCSVImport, func(payload []byte) (*asynq.Task, []asynq.Option, error) {
		var p SheetCSVImportPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, nil, fmt.Errorf("unmarshal sheet csv import payload: %w", err)
		}
		return NewSheetCSVImportTask(p)
	})
}

// NewSheetCSVImportTask creates the CSV import task. Options are returned
// separately (see WebhookForwardPayload's NewWebhookForwardTask).
func NewSheetCSVImportTask(payload SheetCSVImportPayload) (*asynq.Task, []asynq.Option, error) {
	if payload.JobID == uuid.Nil {
		return nil, nil, fmt.Errorf("job_id is required")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal sheet csv import payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueBulk),
		asynq.MaxRetry(3),
		asynq.Timeout(10 * time.Minute),
	}
	return asynq.NewTask(TypeSheetCSVImport, encoded), opts, nil
}

// EnqueueSheetCSVImport schedules the import worker for a persisted job.
func EnqueueSheetCSVImport(ctx context.Context, enq enqueueTaskEnqueuer, jobID uuid.UUID) error {
	if enq == nil {
		return fmt.Errorf("sheet csv import enqueuer is not configured")
	}
	task, opts, err := NewSheetCSVImportTask(SheetCSVImportPayload{JobID: jobID})
	if err != nil {
		return err
	}
	_, err = enq.EnqueueContext(ctx, task, opts...)
	return err
}

// SheetImportEnqueuer adapts enqueue.TaskEnqueuer to sheets.ImportEnqueuer so
// the sheets service can schedule the worker without importing this package.
type SheetImportEnqueuer struct {
	enq enqueue.TaskEnqueuer
}

func NewSheetImportEnqueuer(enq enqueue.TaskEnqueuer) *SheetImportEnqueuer {
	return &SheetImportEnqueuer{enq: enq}
}

// EnqueueSheetCSVImport implements sheets.ImportEnqueuer. An error here makes
// the service mark the job failed (handled in Service.CreateImportJob).
func (e *SheetImportEnqueuer) EnqueueSheetCSVImport(ctx context.Context, jobID uuid.UUID) error {
	if e == nil || e.enq == nil {
		return fmt.Errorf("sheet csv import enqueuer is not configured")
	}
	return EnqueueSheetCSVImport(ctx, e.enq, jobID)
}

// SheetCSVImportHandler streams a CSV object from storage into sheet rows.
type SheetCSVImportHandler struct {
	db      *gorm.DB
	storage storage.Reader
	svc     *sheets.Service
}

func NewSheetCSVImportHandler(db *gorm.DB, reader storage.Reader, events sheets.EventPublisher) *SheetCSVImportHandler {
	svc := sheets.NewService(db)
	if events != nil {
		svc.WithPublisher(events)
	}
	return &SheetCSVImportHandler{db: db, storage: reader, svc: svc}
}

// Handle runs one import attempt. Gone or already-finished jobs return nil
// (no retry). Terminal data errors (bad CSV, limit or coercion violations)
// mark the job failed, roll its rows back, and return nil; transient errors
// leave the job running and return the error so Asynq retries — each retry
// starts by wiping the job's rows, so partial failures reimport cleanly.
func (h *SheetCSVImportHandler) Handle(ctx context.Context, task *asynq.Task) error {
	var payload SheetCSVImportPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}
	job, err := h.svc.ImportJobByID(ctx, payload.JobID)
	if errors.Is(err, sheets.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load import job: %w", err)
	}
	switch job.Status {
	case sheets.ImportStatusPending, sheets.ImportStatusRunning:
	default:
		return nil // completed or failed — stale delivery
	}

	runErr := h.run(ctx, job)
	if runErr == nil {
		return nil
	}
	if isTerminalImportError(runErr) {
		h.failImport(ctx, job, runErr)
		return nil
	}
	if lastAttempt(ctx) {
		h.failImport(ctx, job, runErr)
		return runErr
	}
	return runErr
}

// failImport rolls back the job's rows and marks it failed with the error
// string; a failed publish never masks the original failure.
func (h *SheetCSVImportHandler) failImport(ctx context.Context, job *model.SheetImportJob, cause error) {
	_ = h.svc.ResetImportRows(ctx, job)
	_ = h.svc.MarkImportJobFailed(ctx, job, cause.Error())
	h.svc.PublishImportProgress(ctx, job)
}

// lastAttempt reports whether this delivery is the task's final retry.
func lastAttempt(ctx context.Context) bool {
	retried, ok := asynq.GetRetryCount(ctx)
	if !ok {
		return false
	}
	maxRetry, ok := asynq.GetMaxRetry(ctx)
	if !ok {
		return false
	}
	return retried >= maxRetry
}

// isTerminalImportError reports whether the failure is a property of the data
// or configuration (retrying cannot help) rather than of the environment.
func isTerminalImportError(err error) bool {
	if errors.Is(err, sheets.ErrNotFound) {
		return true
	}
	var (
		limitErr     *sheets.LimitError
		valueErr     *sheets.ValueError
		unknownField *sheets.UnknownFieldError
		unknownType  *sheets.UnknownFieldTypeError
		optionsErr   *sheets.OptionsError
		attachErr    *sheets.AttachmentError
		relationErr  *sheets.RelationError
		importErr    *importOptionsError
		csvErr       *csvFormatError
	)
	return errors.As(err, &limitErr) ||
		errors.As(err, &valueErr) ||
		errors.As(err, &unknownField) ||
		errors.As(err, &unknownType) ||
		errors.As(err, &optionsErr) ||
		errors.As(err, &attachErr) ||
		errors.As(err, &relationErr) ||
		errors.As(err, &importErr) ||
		errors.As(err, &csvErr)
}
