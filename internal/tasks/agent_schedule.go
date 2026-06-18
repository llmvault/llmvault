package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/agentsandbox"
	"github.com/usehivy/hivy/internal/agentschedule"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

const scheduleScanLimit = 50

func init() {
	RegisterTaskBuilder(TypeAgentScheduleDeliver, func(payload []byte) (*asynq.Task, []asynq.Option, error) {
		var p AgentScheduleDeliverPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, nil, fmt.Errorf("unmarshal agent schedule deliver payload: %w", err)
		}
		return NewAgentScheduleDeliverTask(p)
	})
}

type AgentScheduleDeliverPayload struct {
	RunID uuid.UUID `json:"run_id"`
}

func NewAgentScheduleDeliverTask(payload AgentScheduleDeliverPayload) (*asynq.Task, []asynq.Option, error) {
	if payload.RunID == uuid.Nil {
		return nil, nil, fmt.Errorf("run_id is required")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal agent schedule deliver payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueCritical),
		asynq.MaxRetry(5),
		asynq.Timeout(5 * time.Minute),
		asynq.TaskID("agent-schedule-deliver:" + payload.RunID.String()),
	}
	return asynq.NewTask(TypeAgentScheduleDeliver, body), opts, nil
}

type AgentScheduleScanHandler struct {
	db       *gorm.DB
	enqueuer enqueue.TaskEnqueuer
}

func NewAgentScheduleScanHandler(db *gorm.DB, enqueuer enqueue.TaskEnqueuer) *AgentScheduleScanHandler {
	return &AgentScheduleScanHandler{db: db, enqueuer: enqueuer}
}

func (h *AgentScheduleScanHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	if h == nil || h.db == nil || h.enqueuer == nil {
		return nil
	}
	runs, err := h.claimDueRuns(ctx, time.Now().UTC(), scheduleScanLimit)
	if err != nil {
		return err
	}
	for _, run := range runs {
		task, opts, err := NewAgentScheduleDeliverTask(AgentScheduleDeliverPayload{RunID: run.ID})
		if err != nil {
			return err
		}
		if _, err := h.enqueuer.EnqueueContext(ctx, task, opts...); err != nil && !errors.Is(err, asynq.ErrDuplicateTask) {
			return fmt.Errorf("enqueue schedule delivery: %w", err)
		}
	}
	return nil
}

func (h *AgentScheduleScanHandler) claimDueRuns(ctx context.Context, now time.Time, limit int) ([]model.AgentScheduleRun, error) {
	var created []model.AgentScheduleRun
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var schedules []model.AgentSchedule
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND cancelled_at IS NULL AND next_run_at IS NOT NULL AND next_run_at <= ?", agentschedule.StatusActive, now).
			Order("next_run_at ASC").
			Limit(limit).
			Find(&schedules).Error; err != nil {
			return fmt.Errorf("query due schedules: %w", err)
		}
		for _, schedule := range schedules {
			if schedule.NextRunAt == nil {
				continue
			}
			run, inserted, err := claimScheduleRun(tx, schedule, *schedule.NextRunAt, now)
			if err != nil {
				return err
			}
			if inserted {
				created = append(created, run)
			}
		}
		return nil
	})
	return created, err
}

func claimScheduleRun(tx *gorm.DB, schedule model.AgentSchedule, scheduledAt, now time.Time) (model.AgentScheduleRun, bool, error) {
	runKey := agentschedule.RunKey(schedule.RuntimeJobID, scheduledAt)
	run := model.AgentScheduleRun{
		ID:           uuid.New(),
		OrgID:        schedule.OrgID,
		AgentID:      schedule.AgentID,
		ScheduleID:   schedule.ID,
		SandboxID:    cloneUUIDPtr(schedule.SandboxID),
		RuntimeJobID: schedule.RuntimeJobID,
		RunKey:       runKey,
		Status:       agentschedule.RunStatusQueued,
		ScheduledAt:  ptrTime(scheduledAt.UTC()),
		EventPayload: model.RawJSON("{}"),
	}
	result := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "schedule_id"}, {Name: "run_key"}},
		DoNothing: true,
	}).Create(&run)
	if result.Error != nil {
		return model.AgentScheduleRun{}, false, fmt.Errorf("create schedule run: %w", result.Error)
	}
	if err := advanceClaimedSchedule(tx, schedule, now); err != nil {
		return model.AgentScheduleRun{}, false, err
	}
	if result.RowsAffected == 0 {
		return run, false, nil
	}
	return run, true, nil
}

func advanceClaimedSchedule(tx *gorm.DB, schedule model.AgentSchedule, now time.Time) error {
	completed := schedule.RepeatCompleted + 1
	updates := map[string]any{
		"repeat_completed": completed,
		"last_run_at":      now,
		"last_status":      agentschedule.RunStatusQueued,
		"updated_at":       now,
	}
	if schedule.RepeatCount != nil && completed >= *schedule.RepeatCount {
		updates["status"] = agentschedule.StatusCompleted
		updates["next_run_at"] = nil
	} else {
		next, err := nextScheduleRunAfter(schedule, now)
		if err != nil {
			updates["status"] = agentschedule.StatusPaused
			updates["last_error"] = err.Error()
			updates["next_run_at"] = nil
		} else {
			updates["next_run_at"] = next
			updates["last_error"] = ""
		}
	}
	if err := tx.Model(&model.AgentSchedule{}).Where("id = ?", schedule.ID).Updates(updates).Error; err != nil {
		return fmt.Errorf("advance schedule: %w", err)
	}
	return nil
}

func nextScheduleRunAfter(schedule model.AgentSchedule, now time.Time) (time.Time, error) {
	if strings.EqualFold(schedule.ScheduleKind, agentschedule.KindInterval) || strings.TrimSpace(schedule.CronExpression) == "" {
		if schedule.IntervalSeconds == nil || *schedule.IntervalSeconds <= 0 {
			return time.Time{}, fmt.Errorf("interval_seconds must be positive")
		}
		step := time.Duration(*schedule.IntervalSeconds) * time.Second
		base := now.UTC()
		if schedule.NextRunAt != nil {
			base = schedule.NextRunAt.UTC()
		}
		if !base.After(now) {
			missed := int64(now.Sub(base)/step) + 1
			base = base.Add(time.Duration(missed) * step)
		}
		return base.UTC(), nil
	}
	return agentschedule.NextRunAfter(schedule, now)
}

type AgentScheduleDeliverHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	compileDeps  agentruntime.CompileDeps
	enqueuer     enqueue.TaskEnqueuer
}

func NewAgentScheduleDeliverHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator, compileDeps agentruntime.CompileDeps, enqueuer enqueue.TaskEnqueuer) *AgentScheduleDeliverHandler {
	return &AgentScheduleDeliverHandler{db: db, orchestrator: orchestrator, compileDeps: compileDeps, enqueuer: enqueuer}
}

func (h *AgentScheduleDeliverHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h == nil || h.db == nil {
		return nil
	}
	var payload AgentScheduleDeliverPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal schedule deliver payload: %w", err)
	}
	sessionID, err := h.ensureRunSession(ctx, payload.RunID)
	if err != nil {
		return err
	}
	if err := EnqueueSessionMessageDeliver(ctx, h.enqueuer, sessionID); err != nil {
		return fmt.Errorf("enqueue scheduled session delivery: %w", err)
	}
	return nil
}

func (h *AgentScheduleDeliverHandler) ensureRunSession(ctx context.Context, runID uuid.UUID) (uuid.UUID, error) {
	var sessionID uuid.UUID
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run model.AgentScheduleRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("Schedule").
			Where("id = ?", runID).
			First(&run).Error; err != nil {
			return fmt.Errorf("load schedule run: %w", err)
		}
		if run.SessionID != nil {
			sessionID = *run.SessionID
			return nil
		}
		var agent model.Agent
		if err := tx.Where("id = ? AND org_id = ? AND status <> ?", run.AgentID, run.OrgID, "archived").First(&agent).Error; err != nil {
			return fmt.Errorf("load schedule agent: %w", err)
		}
		channelID, err := uuid.Parse(strings.TrimSpace(run.Schedule.Channel))
		if err != nil || channelID == uuid.Nil {
			return fmt.Errorf("scheduled channel_id is invalid")
		}
		sandboxID := scheduledSessionSandboxID(ctx, tx, agent, run.Schedule)
		now := time.Now().UTC()
		sessionID = uuid.New()
		session := model.Session{
			ID:                sessionID,
			OrgID:             run.OrgID,
			ChannelID:         channelID,
			AgentID:           run.AgentID,
			SandboxID:         sandboxID,
			Model:             agent.Model,
			AccessMode:        "full",
			ReasoningEffort:   "high",
			Source:            "schedule",
			SourceID:          &run.ScheduleID,
			SourceResourceKey: run.RunKey,
			Name:              scheduledSessionName(run.Schedule),
			Status:            "active",
			AgentTurnStatus:   model.SessionAgentTurnIdle,
			IntegrationScopes: model.JSON{},
		}
		if err := tx.Create(&session).Error; err != nil {
			return fmt.Errorf("create schedule session: %w", err)
		}
		event := model.SessionEvent{
			OrgID:            run.OrgID,
			SessionID:        sessionID,
			AgentID:          run.AgentID,
			SandboxID:        sandboxID,
			RuntimeSessionID: sessionID.String(),
			EventID:          "schedule-" + run.ID.String(),
			EventType:        "user.message",
			Source:           "schedule",
			SequenceNumber:   1,
			Payload:          scheduledMessagePayload(run, now),
			EventAt:          now,
		}
		if err := tx.Create(&event).Error; err != nil {
			return fmt.Errorf("create schedule session event: %w", err)
		}
		queue := model.SessionMessageQueue{
			OrgID:          run.OrgID,
			SessionID:      sessionID,
			SessionEventID: event.ID,
			SequenceNumber: 1,
			Status:         "pending",
		}
		if err := tx.Create(&queue).Error; err != nil {
			return fmt.Errorf("create schedule message queue: %w", err)
		}
		updates := map[string]any{
			"session_id":   sessionID,
			"sandbox_id":   sandboxID,
			"status":       agentschedule.RunStatusProcessing,
			"started_at":   now,
			"lease_owner":  "",
			"leased_until": nil,
			"updated_at":   now,
		}
		if err := tx.Model(&model.AgentScheduleRun{}).Where("id = ?", run.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("mark schedule run processing: %w", err)
		}
		return nil
	})
	return sessionID, err
}

func scheduledSessionSandboxID(ctx context.Context, db *gorm.DB, agent model.Agent, schedule model.AgentSchedule) *uuid.UUID {
	if agent.OrgID == nil || agent.SandboxStrategy != agentSandboxStrategyAlwaysOn {
		return nil
	}
	if schedule.SandboxID != nil {
		return cloneUUIDPtr(schedule.SandboxID)
	}
	sb, err := agentsandbox.Selector{DB: db}.MainRuntime(ctx, *agent.OrgID, agent.ID)
	if err != nil || sb == nil {
		return nil
	}
	id := sb.ID
	return &id
}

func scheduledSessionName(schedule model.AgentSchedule) string {
	name := strings.TrimSpace(schedule.Description)
	if name == "" {
		name = strings.TrimSpace(schedule.TaskPrompt)
	}
	if name == "" {
		return "Scheduled run"
	}
	if len(name) > 80 {
		name = name[:80]
	}
	return name
}

func scheduledMessagePayload(run model.AgentScheduleRun, startedAt time.Time) model.JSON {
	scheduledAt := startedAt
	if run.ScheduledAt != nil {
		scheduledAt = *run.ScheduledAt
	}
	return model.JSON{
		"text":                  run.Schedule.TaskPrompt,
		"source":                "cron",
		"job_id":                run.RuntimeJobID,
		"schedule_id":           run.ScheduleID.String(),
		"schedule_run_id":       run.ID.String(),
		"schedule_run_key":      run.RunKey,
		"schedule_scheduled_at": scheduledAt.UTC().Format(time.RFC3339),
		"schedule_started_at":   startedAt.UTC().Format(time.RFC3339),
		"schedule_is_one_shot":  false,
		"schedule_is_wake":      false,
	}
}

func cloneUUIDPtr(value *uuid.UUID) *uuid.UUID {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

func ptrTime(value time.Time) *time.Time {
	return &value
}

func CaptureScheduleDeliveryError(ctx context.Context, err error, runID uuid.UUID) {
	if err == nil {
		return
	}
	logging.CaptureWithFields(ctx, err, map[string]any{"schedule_run_id": runID.String()})
}
