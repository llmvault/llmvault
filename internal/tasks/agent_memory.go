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
	agentMemoryRetainTimeout = 220 * time.Second
	agentMemoryQuietWindow   = 3 * time.Minute
	agentMemoryCheckDelay    = 10 * time.Minute
)

type AgentMemoryRetainHandler struct {
	db       *gorm.DB
	memory   *hindsight.Client
	enqueuer enqueue.TaskEnqueuer
	cache    precontext.Cache
	banks    *hindsight.BankProvisioner
}

func NewAgentMemoryRetainHandler(db *gorm.DB, memory *hindsight.Client, enqueuer enqueue.TaskEnqueuer, caches ...precontext.Cache) *AgentMemoryRetainHandler {
	h := &AgentMemoryRetainHandler{db: db, memory: memory, enqueuer: enqueuer, banks: hindsight.NewBankProvisioner(db, memory)}
	if len(caches) > 0 {
		h.cache = caches[0]
	}
	return h
}

func (h *AgentMemoryRetainHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h == nil || h.db == nil || h.memory == nil {
		logging.FromContext(ctx).WarnContext(ctx, "agent memory retain skipped: handler dependencies missing")
		return nil
	}
	var payload AgentMemoryRetainPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal agent memory retain payload: %w", err)
	}
	fields := agentMemoryRetainFields(payload)
	start := time.Now()
	logging.FromContext(ctx).InfoContext(ctx, "agent memory retain started", fieldsToArgs(fields)...)
	if payload.AgentID == uuid.Nil || payload.SandboxID == uuid.Nil {
		logAgentMemoryRetainSkip(ctx, fields, "missing_agent_or_sandbox_id")
		return nil
	}

	var agent model.Agent
	if err := h.db.WithContext(ctx).Where("id = ?", payload.AgentID).First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logAgentMemoryRetainSkip(ctx, fields, "agent_not_found")
			return nil
		}
		return fmt.Errorf("load agent for memory retain: %w", err)
	}
	if agent.OrgID == nil {
		logAgentMemoryRetainSkip(ctx, fields, "agent_missing_org")
		return nil
	}
	fields["org_id"] = agent.OrgID.String()

	session, err := h.loadSession(ctx, payload)
	if err != nil {
		return err
	}
	if session == nil {
		logAgentMemoryRetainSkip(ctx, fields, "session_not_found")
		return nil
	}
	if payload.AgentSessionID == uuid.Nil {
		payload.AgentSessionID = session.ID
		fields["agent_session_id"] = session.ID.String()
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
	if time.Since(latest) < agentMemoryQuietWindow {
		fields["latest_event_at"] = latest.Format(time.RFC3339Nano)
		fields["quiet_window_seconds"] = int(agentMemoryQuietWindow.Seconds())
		logging.FromContext(ctx).InfoContext(ctx, "agent memory retain deferred: session still active", fieldsToArgs(fields)...)
		h.enqueueRetainCheck(ctx, payload)
		return nil
	}

	events, err := h.loadSessionEvents(ctx, session.ID)
	if err != nil {
		return err
	}
	fields["event_count"] = len(events)
	fields["memory_candidate_event_count"] = countAgentMemoryCandidateEvents(events)
	item, ok, reason := buildAgentRetainItemWithReason(&agent, payload, events)
	if !ok {
		logAgentMemoryRetainSkip(ctx, fields, reason)
		return nil
	}
	fields["document_id"] = item.DocumentID
	fields["source"] = dominantAgentMemorySource(events)

	bankID := hindsight.OrgBankID(*agent.OrgID)
	fields["bank_id"] = bankID
	if err := h.banks.EnsureOrgBank(ctx, *agent.OrgID); err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("agent memory retain: ensure bank %s: %w", bankID, err), fields)
		return fmt.Errorf("configure memory bank: %w", err)
	}
	retainCtx, cancel := context.WithTimeout(ctx, agentMemoryRetainTimeout)
	defer cancel()
	result, err := h.memory.Retain(retainCtx, bankID, &hindsight.RetainRequest{Items: []hindsight.RetainItem{item}, Async: true})
	if err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("agent memory retain: retain bank_id=%s agent_id=%s: %w", bankID, agent.ID, err), fields)
		return fmt.Errorf("retain agent memory: %w", err)
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
		Model(&model.AgentSessionEvent{}).
		Where("id IN ?", agentSessionEventIDs(events)).
		Update("retained_at", now)
	if update.Error != nil {
		return fmt.Errorf("mark agent session events retained: %w", update.Error)
	}
	fields["retained_event_count"] = update.RowsAffected
	fields["duration_ms"] = time.Since(start).Milliseconds()
	logging.FromContext(ctx).InfoContext(ctx, "agent memory retain completed", fieldsToArgs(fields)...)

	h.enqueueRefresh(ctx, payload.AgentID, payload.SandboxID)
	return nil
}

func (h *AgentMemoryRetainHandler) loadPendingEvents(ctx context.Context, payload AgentMemoryRetainPayload) ([]model.AgentSessionEvent, error) {
	if payload.AgentSessionID != uuid.Nil {
		return h.loadSessionEvents(ctx, payload.AgentSessionID)
	}
	var events []model.AgentSessionEvent
	if err := h.db.WithContext(ctx).
		Where("agent_id = ? AND sandbox_id = ? AND runtime_session_id = ? AND retained_at IS NULL",
			payload.AgentID, payload.SandboxID, payload.SessionID).
		Order("event_at ASC, created_at ASC").
		Find(&events).Error; err != nil {
		return nil, fmt.Errorf("load agent session events: %w", err)
	}
	return events, nil
}

func (h *AgentMemoryRetainHandler) loadSession(ctx context.Context, payload AgentMemoryRetainPayload) (*model.AgentSession, error) {
	var session model.AgentSession
	query := h.db.WithContext(ctx).Where("agent_id = ? AND sandbox_id = ?", payload.AgentID, payload.SandboxID)
	if payload.AgentSessionID != uuid.Nil {
		query = query.Where("id = ?", payload.AgentSessionID)
	} else if strings.TrimSpace(payload.SessionID) != "" {
		query = query.Where("runtime_conversation_id = ?", strings.TrimSpace(payload.SessionID))
	} else {
		return nil, nil
	}
	if err := query.First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("load agent session for memory retain: %w", err)
	}
	return &session, nil
}

func (h *AgentMemoryRetainHandler) latestSessionActivity(ctx context.Context, agentSessionID uuid.UUID) (time.Time, error) {
	var latest sql.NullTime
	if err := h.db.WithContext(ctx).
		Model(&model.AgentSessionEvent{}).
		Where("agent_session_id = ?", agentSessionID).
		Select("MAX(event_at)").
		Scan(&latest).Error; err != nil {
		return time.Time{}, fmt.Errorf("load latest agent session activity: %w", err)
	}
	if !latest.Valid {
		return time.Time{}, nil
	}
	return latest.Time.UTC(), nil
}

func (h *AgentMemoryRetainHandler) loadSessionEvents(ctx context.Context, agentSessionID uuid.UUID) ([]model.AgentSessionEvent, error) {
	var events []model.AgentSessionEvent
	if err := h.db.WithContext(ctx).
		Where("agent_session_id = ?", agentSessionID).
		Order("event_at ASC, created_at ASC").
		Find(&events).Error; err != nil {
		return nil, fmt.Errorf("load agent session events: %w", err)
	}
	return events, nil
}

func (h *AgentMemoryRetainHandler) enqueueRetainCheck(ctx context.Context, payload AgentMemoryRetainPayload) {
	if h.enqueuer == nil {
		logging.FromContext(ctx).WarnContext(ctx, "agent memory retain requeue skipped: enqueuer missing", fieldsToArgs(agentMemoryRetainFields(payload))...)
		return
	}
	payload.Reason = "session_still_active"
	taskID := AgentMemoryRetainTaskID(payload)
	task, opts, err := NewAgentMemoryRetainTask(payload)
	if err != nil {
		logging.Capture(ctx, err)
		return
	}
	opts = append(opts,
		asynq.ProcessIn(agentMemoryCheckDelay),
		asynq.TaskID(taskID),
	)
	_, err = h.enqueuer.EnqueueContext(ctx, task, opts...)
	duplicate := errors.Is(err, asynq.ErrDuplicateTask)
	if err != nil && !duplicate {
		fields := agentMemoryRetainFields(payload)
		fields["task_id"] = taskID
		logging.CaptureWithFields(ctx, fmt.Errorf("agent memory retain: requeue quiet check: %w", err), fields)
	} else {
		fields := agentMemoryRetainFields(payload)
		fields["task_id"] = taskID
		fields["delay_seconds"] = int(agentMemoryCheckDelay.Seconds())
		fields["duplicate"] = duplicate
		logging.FromContext(ctx).InfoContext(ctx, "agent memory retain requeued", fieldsToArgs(fields)...)
	}
}
