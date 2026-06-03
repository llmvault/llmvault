package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/hindsight"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/precontext"
)

const (
	employeeMemoryRetainTimeout = 220 * time.Second
	employeeMemoryQuietWindow   = 3 * time.Minute
	employeeMemoryCheckDelay    = 10 * time.Minute
)

type EmployeeMemoryRetainHandler struct {
	db       *gorm.DB
	memory   *hindsight.Client
	enqueuer enqueue.TaskEnqueuer
	cache    precontext.Cache
}

func NewEmployeeMemoryRetainHandler(db *gorm.DB, memory *hindsight.Client, enqueuer enqueue.TaskEnqueuer, caches ...precontext.Cache) *EmployeeMemoryRetainHandler {
	h := &EmployeeMemoryRetainHandler{db: db, memory: memory, enqueuer: enqueuer}
	if len(caches) > 0 {
		h.cache = caches[0]
	}
	return h
}

func (h *EmployeeMemoryRetainHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h == nil || h.db == nil || h.memory == nil {
		logging.FromContext(ctx).WarnContext(ctx, "employee memory retain skipped: handler dependencies missing")
		return nil
	}
	var payload EmployeeMemoryRetainPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal employee memory retain payload: %w", err)
	}
	fields := employeeMemoryRetainFields(payload)
	start := time.Now()
	logging.FromContext(ctx).InfoContext(ctx, "employee memory retain started", fieldsToArgs(fields)...)
	if payload.EmployeeID == uuid.Nil || payload.SandboxID == uuid.Nil {
		logEmployeeMemoryRetainSkip(ctx, fields, "missing_employee_or_sandbox_id")
		return nil
	}

	var agent model.Employee
	if err := h.db.WithContext(ctx).Where("id = ?", payload.EmployeeID).First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logEmployeeMemoryRetainSkip(ctx, fields, "employee_not_found")
			return nil
		}
		return fmt.Errorf("load employee for memory retain: %w", err)
	}
	if agent.OrgID == nil {
		logEmployeeMemoryRetainSkip(ctx, fields, "employee_missing_org")
		return nil
	}
	fields["org_id"] = agent.OrgID.String()

	session, err := h.loadSession(ctx, payload)
	if err != nil {
		return err
	}
	if session == nil {
		logEmployeeMemoryRetainSkip(ctx, fields, "session_not_found")
		return nil
	}
	if payload.EmployeeSessionID == uuid.Nil {
		payload.EmployeeSessionID = session.ID
		fields["employee_session_id"] = session.ID.String()
	}
	if strings.TrimSpace(payload.SessionID) == "" {
		payload.SessionID = session.RuntimeConversationID
		fields["runtime_session_id"] = payload.SessionID
	}
	latest, err := h.latestSessionActivity(ctx, session.ID)
	if err != nil {
		return err
	}
	if latest.IsZero() {
		latest = session.CreatedAt
	}
	if time.Since(latest) < employeeMemoryQuietWindow {
		fields["latest_event_at"] = latest.Format(time.RFC3339Nano)
		fields["quiet_window_seconds"] = int(employeeMemoryQuietWindow.Seconds())
		logging.FromContext(ctx).InfoContext(ctx, "employee memory retain deferred: session still active", fieldsToArgs(fields)...)
		h.enqueueRetainCheck(ctx, payload)
		return nil
	}

	events, err := h.loadSessionEvents(ctx, session.ID)
	if err != nil {
		return err
	}
	fields["event_count"] = len(events)
	fields["memory_candidate_event_count"] = countEmployeeMemoryCandidateEvents(events)
	item, ok, reason := buildEmployeeRetainItemWithReason(&agent, payload, events)
	if !ok {
		logEmployeeMemoryRetainSkip(ctx, fields, reason)
		return nil
	}
	fields["document_id"] = item.DocumentID
	fields["source"] = dominantEmployeeMemorySource(events)

	bankID := hindsight.OrgBankID(*agent.OrgID)
	fields["bank_id"] = bankID
	if err := h.memory.ConfigureBank(ctx, bankID, hindsight.DefaultMemoryConfig().ToBankConfigUpdate()); err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("employee memory retain: configure bank %s: %w", bankID, err), fields)
		return fmt.Errorf("configure memory bank: %w", err)
	}
	retainCtx, cancel := context.WithTimeout(ctx, employeeMemoryRetainTimeout)
	defer cancel()
	result, err := h.memory.Retain(retainCtx, bankID, &hindsight.RetainRequest{Items: []hindsight.RetainItem{item}, Async: true})
	if err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("employee memory retain: retain bank_id=%s employee_id=%s: %w", bankID, agent.ID, err), fields)
		return fmt.Errorf("retain employee memory: %w", err)
	}
	if result != nil {
		fields["hindsight_success"] = result.Success
		fields["hindsight_items_count"] = result.ItemsCount
		fields["hindsight_async"] = result.Async
		fields["hindsight_operation_id"] = result.OperationID
	}
	precontext.InvalidateMemories(ctx, h.cache, *agent.OrgID, agent.ID)

	now := time.Now().UTC()
	update := h.db.WithContext(ctx).
		Model(&model.EmployeeSessionEvent{}).
		Where("id IN ?", employeeSessionEventIDs(events)).
		Update("retained_at", now)
	if update.Error != nil {
		return fmt.Errorf("mark employee session events retained: %w", update.Error)
	}
	fields["retained_event_count"] = update.RowsAffected
	fields["duration_ms"] = time.Since(start).Milliseconds()
	logging.FromContext(ctx).InfoContext(ctx, "employee memory retain completed", fieldsToArgs(fields)...)

	h.enqueueRefresh(ctx, payload.EmployeeID, payload.SandboxID)
	return nil
}

func (h *EmployeeMemoryRetainHandler) loadPendingEvents(ctx context.Context, payload EmployeeMemoryRetainPayload) ([]model.EmployeeSessionEvent, error) {
	if payload.EmployeeSessionID != uuid.Nil {
		return h.loadSessionEvents(ctx, payload.EmployeeSessionID)
	}
	var events []model.EmployeeSessionEvent
	if err := h.db.WithContext(ctx).
		Where("employee_id = ? AND sandbox_id = ? AND runtime_session_id = ? AND retained_at IS NULL",
			payload.EmployeeID, payload.SandboxID, payload.SessionID).
		Order("event_at ASC, created_at ASC").
		Find(&events).Error; err != nil {
		return nil, fmt.Errorf("load employee session events: %w", err)
	}
	return events, nil
}

func (h *EmployeeMemoryRetainHandler) loadSession(ctx context.Context, payload EmployeeMemoryRetainPayload) (*model.EmployeeSession, error) {
	var session model.EmployeeSession
	query := h.db.WithContext(ctx).Where("employee_id = ? AND sandbox_id = ?", payload.EmployeeID, payload.SandboxID)
	if payload.EmployeeSessionID != uuid.Nil {
		query = query.Where("id = ?", payload.EmployeeSessionID)
	} else if strings.TrimSpace(payload.SessionID) != "" {
		query = query.Where("runtime_conversation_id = ?", strings.TrimSpace(payload.SessionID))
	} else {
		return nil, nil
	}
	if err := query.First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("load employee session for memory retain: %w", err)
	}
	return &session, nil
}

func (h *EmployeeMemoryRetainHandler) latestSessionActivity(ctx context.Context, employeeSessionID uuid.UUID) (time.Time, error) {
	var latest sql.NullTime
	if err := h.db.WithContext(ctx).
		Model(&model.EmployeeSessionEvent{}).
		Where("employee_session_id = ?", employeeSessionID).
		Select("MAX(event_at)").
		Scan(&latest).Error; err != nil {
		return time.Time{}, fmt.Errorf("load latest employee session activity: %w", err)
	}
	if !latest.Valid {
		return time.Time{}, nil
	}
	return latest.Time.UTC(), nil
}

func (h *EmployeeMemoryRetainHandler) loadSessionEvents(ctx context.Context, employeeSessionID uuid.UUID) ([]model.EmployeeSessionEvent, error) {
	var events []model.EmployeeSessionEvent
	if err := h.db.WithContext(ctx).
		Where("employee_session_id = ?", employeeSessionID).
		Order("event_at ASC, created_at ASC").
		Find(&events).Error; err != nil {
		return nil, fmt.Errorf("load employee session events: %w", err)
	}
	return events, nil
}

func (h *EmployeeMemoryRetainHandler) enqueueRetainCheck(ctx context.Context, payload EmployeeMemoryRetainPayload) {
	if h.enqueuer == nil {
		logging.FromContext(ctx).WarnContext(ctx, "employee memory retain requeue skipped: enqueuer missing", fieldsToArgs(employeeMemoryRetainFields(payload))...)
		return
	}
	payload.Reason = "session_still_active"
	task, err := NewEmployeeMemoryRetainTask(payload)
	if err != nil {
		logging.Capture(ctx, err)
		return
	}
	taskID := "employee-memory-retain:" + payload.EmployeeSessionID.String()
	if payload.EmployeeSessionID == uuid.Nil {
		taskID = "employee-memory-retain:" + payload.SandboxID.String() + ":" + payload.SessionID
	}
	if _, err := h.enqueuer.EnqueueContext(ctx, task,
		asynq.ProcessIn(employeeMemoryCheckDelay),
		asynq.TaskID(taskID),
	); err != nil && !errors.Is(err, asynq.ErrDuplicateTask) {
		fields := employeeMemoryRetainFields(payload)
		fields["task_id"] = taskID
		logging.CaptureWithFields(ctx, fmt.Errorf("employee memory retain: requeue quiet check: %w", err), fields)
	} else {
		fields := employeeMemoryRetainFields(payload)
		fields["task_id"] = taskID
		fields["delay_seconds"] = int(employeeMemoryCheckDelay.Seconds())
		fields["duplicate"] = errors.Is(err, asynq.ErrDuplicateTask)
		logging.FromContext(ctx).InfoContext(ctx, "employee memory retain requeued", fieldsToArgs(fields)...)
	}
}
