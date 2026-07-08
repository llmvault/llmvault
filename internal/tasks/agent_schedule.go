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
	"github.com/usehivy/hivy/internal/agentschedule"
	"github.com/usehivy/hivy/internal/channelagents"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

const scheduleScanLimit = 50

// scheduleSource labels sessions and transcript events created by the agent
// scheduler, mirroring triggerConversationSource for the trigger path.
const scheduleSource = "schedule"

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
	sessionID, created, err := h.ensureRunSession(ctx, payload.RunID)
	if err != nil {
		return err
	}
	// Best-effort auto-naming for freshly created schedule sessions, same as
	// trigger/web/Slack; replaces the interim scheduledSessionName placeholder.
	// The naming handler guards on session_name_auto_generated_at and reads the
	// first user.message.received event we just persisted, so ordering (event
	// committed before this enqueue) matters.
	if created && h.enqueuer != nil {
		if task, opts, nameErr := NewSessionNameTask(sessionID); nameErr != nil {
			logging.FromContext(ctx).WarnContext(ctx, "build schedule session naming task",
				"session_id", sessionID, "error", nameErr)
		} else if _, nameErr := h.enqueuer.Enqueue(task, opts...); nameErr != nil {
			logging.FromContext(ctx).WarnContext(ctx, "enqueue schedule session naming",
				"session_id", sessionID, "error", nameErr)
		}
	}
	if err := EnqueueSessionMessageDeliver(ctx, h.enqueuer, sessionID); err != nil {
		return fmt.Errorf("enqueue scheduled session delivery: %w", err)
	}
	return nil
}

func (h *AgentScheduleDeliverHandler) ensureRunSession(ctx context.Context, runID uuid.UUID) (uuid.UUID, bool, error) {
	var sessionID uuid.UUID
	var created bool
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
		// Hard enforcement: the agent must belong to the schedule's channel's team
		// (system/external channels are unbounded via channelagents.ActsInChannel).
		acts, err := channelagents.ActsInChannel(ctx, tx, run.OrgID, channelID, run.AgentID)
		if err != nil {
			return fmt.Errorf("check schedule channel agent: %w", err)
		}
		if !acts {
			return fmt.Errorf("agent does not belong to this channel's team")
		}
		now := time.Now().UTC()
		sessionID = uuid.New()
		session := model.Session{
			ID:                sessionID,
			OrgID:             run.OrgID,
			ChannelID:         channelID,
			AgentID:           run.AgentID,
			Model:             agent.Model,
			ReasoningEffort:   "high",
			Source:            scheduleSource,
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
		messagePayload := scheduledMessagePayload(run, now)
		// Persist the scheduled prompt to the transcript so it is visible in the
		// session UI and available to the naming task, which reads the first
		// user.message.received event. Idempotent on (session_id, event_id).
		event, err := ensureAutomatedSessionEvent(tx, session, "schedule:"+run.RunKey, scheduleSource, messagePayload)
		if err != nil {
			return err
		}
		sessionEventID := event.ID
		queue := model.SessionMessageQueue{
			OrgID:          run.OrgID,
			SessionID:      sessionID,
			SessionEventID: &sessionEventID,
			MessageText:    run.Schedule.TaskPrompt,
			MessagePayload: messagePayload,
			SequenceNumber: 1,
			Status:         "pending",
		}
		if err := tx.Create(&queue).Error; err != nil {
			return fmt.Errorf("create schedule message queue: %w", err)
		}
		created = true
		updates := map[string]any{
			"session_id":   sessionID,
			"sandbox_id":   nil,
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
	return sessionID, created, err
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
	}
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
