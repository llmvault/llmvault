package agentschedule

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

const (
	StatusActive    = "active"
	StatusPaused    = "paused"
	StatusCancelled = "cancelled"
	StatusCompleted = "completed"

	RunStatusQueued     = "queued"
	RunStatusProcessing = "processing"
	RunStatusCompleted  = "completed"
	RunStatusFailed     = "failed"

	KindInterval = "interval"
	KindCron     = "cron"
)

type CreateInput struct {
	JobID           string
	SourceSlug      string
	Description     string
	TaskPrompt      string
	ChannelID       string
	IntervalSeconds *int64
	CronExpression  string
	RepeatCount     *int64
}

type UpdateInput struct {
	Description     *string
	TaskPrompt      *string
	ChannelID       *string
	IntervalSeconds *int64
	CronExpression  *string
	RepeatCount     *int64
}

func CreateFromSession(ctx context.Context, db *gorm.DB, agent *model.Agent, sessionID string, input CreateInput) (*model.AgentSchedule, error) {
	if db == nil {
		return nil, fmt.Errorf("database is required")
	}
	if agent == nil || agent.ID == uuid.Nil || agent.OrgID == nil {
		return nil, fmt.Errorf("agent context is required")
	}
	sessionUUID, err := uuid.Parse(strings.TrimSpace(sessionID))
	if err != nil || sessionUUID == uuid.Nil {
		return nil, fmt.Errorf("current session is required")
	}
	var session model.Session
	if err := db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND agent_id = ?", sessionUUID, *agent.OrgID, agent.ID).
		First(&session).Error; err != nil {
		return nil, fmt.Errorf("load current session: %w", err)
	}
	channelID, err := resolveScheduleChannel(ctx, db, *agent.OrgID, agent.ID, input.ChannelID)
	if err != nil {
		return nil, err
	}
	kind, nextRunAt, err := normalizeCadence(time.Now().UTC(), input.IntervalSeconds, input.CronExpression)
	if err != nil {
		return nil, err
	}
	taskPrompt := strings.TrimSpace(input.TaskPrompt)
	if taskPrompt == "" {
		return nil, fmt.Errorf("task_prompt is required")
	}
	jobID := strings.TrimSpace(input.JobID)
	if jobID == "" {
		jobID = "cron-" + uuid.NewString()
	}
	now := time.Now().UTC()
	schedule := model.AgentSchedule{
		OrgID:            *agent.OrgID,
		AgentID:          agent.ID,
		RuntimeJobID:     jobID,
		Status:           StatusActive,
		SourceSlug:       strings.TrimSpace(input.SourceSlug),
		ScheduleKind:     kind,
		Channel:          channelID,
		Description:      defaultDescription(input.Description, taskPrompt),
		TaskPrompt:       taskPrompt,
		CronExpression:   strings.TrimSpace(input.CronExpression),
		IntervalSeconds:  input.IntervalSeconds,
		RepeatCount:      input.RepeatCount,
		RepeatCompleted:  0,
		NextRunAt:        &nextRunAt,
		CreatedBySession: session.ID.String(),
		RuntimeCreatedAt: &now,
	}
	if err := db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "agent_id"}, {Name: "runtime_job_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"org_id":             schedule.OrgID,
			"sandbox_id":         schedule.SandboxID,
			"status":             schedule.Status,
			"source_slug":        gorm.Expr("CASE WHEN EXCLUDED.source_slug <> '' THEN EXCLUDED.source_slug ELSE agent_schedules.source_slug END"),
			"schedule_kind":      schedule.ScheduleKind,
			"channel":            schedule.Channel,
			"description":        schedule.Description,
			"task_prompt":        schedule.TaskPrompt,
			"cron_expression":    schedule.CronExpression,
			"interval_seconds":   schedule.IntervalSeconds,
			"repeat_count":       schedule.RepeatCount,
			"next_run_at":        schedule.NextRunAt,
			"created_by_session": schedule.CreatedBySession,
			"runtime_created_at": schedule.RuntimeCreatedAt,
			"cancelled_at":       nil,
			"last_error":         "",
			"updated_at":         now,
		}),
	}).Create(&schedule).Error; err != nil {
		return nil, fmt.Errorf("create schedule: %w", err)
	}
	if err := db.WithContext(ctx).
		Where("agent_id = ? AND runtime_job_id = ?", agent.ID, jobID).
		First(&schedule).Error; err != nil {
		return nil, fmt.Errorf("load schedule: %w", err)
	}
	return &schedule, nil
}

func List(ctx context.Context, db *gorm.DB, agent *model.Agent) ([]model.AgentSchedule, error) {
	if db == nil || agent == nil || agent.ID == uuid.Nil || agent.OrgID == nil {
		return nil, fmt.Errorf("agent context is required")
	}
	var rows []model.AgentSchedule
	if err := db.WithContext(ctx).
		Where("org_id = ? AND agent_id = ? AND cancelled_at IS NULL", *agent.OrgID, agent.ID).
		Order("created_at ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list schedules: %w", err)
	}
	return rows, nil
}

func Update(ctx context.Context, db *gorm.DB, agent *model.Agent, jobID string, input UpdateInput) (*model.AgentSchedule, error) {
	schedule, err := LoadForAgent(ctx, db, agent, jobID)
	if err != nil {
		return nil, err
	}
	updates := map[string]any{}
	if input.Description != nil {
		updates["description"] = strings.TrimSpace(*input.Description)
	}
	if input.TaskPrompt != nil {
		taskPrompt := strings.TrimSpace(*input.TaskPrompt)
		if taskPrompt == "" {
			return nil, fmt.Errorf("task_prompt is required")
		}
		updates["task_prompt"] = taskPrompt
	}
	if input.ChannelID != nil {
		channelID, err := resolveScheduleChannel(ctx, db, *agent.OrgID, agent.ID, *input.ChannelID)
		if err != nil {
			return nil, err
		}
		updates["channel"] = channelID
	}
	if input.IntervalSeconds != nil || input.CronExpression != nil {
		expr := schedule.CronExpression
		interval := schedule.IntervalSeconds
		if input.CronExpression != nil {
			expr = strings.TrimSpace(*input.CronExpression)
			interval = nil
		}
		if input.IntervalSeconds != nil {
			interval = input.IntervalSeconds
			expr = ""
		}
		kind, nextRunAt, err := normalizeCadence(time.Now().UTC(), interval, expr)
		if err != nil {
			return nil, err
		}
		updates["schedule_kind"] = kind
		updates["cron_expression"] = expr
		updates["interval_seconds"] = interval
		updates["next_run_at"] = nextRunAt
	}
	if input.RepeatCount != nil {
		updates["repeat_count"] = input.RepeatCount
	}
	if len(updates) == 0 {
		return schedule, nil
	}
	updates["updated_at"] = time.Now().UTC()
	if err := db.WithContext(ctx).Model(&model.AgentSchedule{}).
		Where("id = ?", schedule.ID).
		Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("update schedule: %w", err)
	}
	return LoadForAgent(ctx, db, agent, jobID)
}

func SetStatus(ctx context.Context, db *gorm.DB, agent *model.Agent, jobID, status string) (*model.AgentSchedule, error) {
	schedule, err := LoadForAgent(ctx, db, agent, jobID)
	if err != nil {
		return nil, err
	}
	status = strings.ToLower(strings.TrimSpace(status))
	updates := map[string]any{"status": status, "updated_at": time.Now().UTC()}
	switch status {
	case StatusActive:
		updates["cancelled_at"] = nil
		if schedule.NextRunAt == nil || schedule.NextRunAt.Before(time.Now().UTC()) {
			next, err := NextRunAfter(*schedule, time.Now().UTC())
			if err != nil {
				return nil, err
			}
			updates["next_run_at"] = next
		}
	case StatusPaused:
	case StatusCancelled:
		now := time.Now().UTC()
		updates["cancelled_at"] = &now
	default:
		return nil, fmt.Errorf("unsupported status %q", status)
	}
	if err := db.WithContext(ctx).Model(&model.AgentSchedule{}).
		Where("id = ?", schedule.ID).
		Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("set schedule status: %w", err)
	}
	return LoadForAgent(ctx, db, agent, jobID)
}

func LoadForAgent(ctx context.Context, db *gorm.DB, agent *model.Agent, jobID string) (*model.AgentSchedule, error) {
	if db == nil || agent == nil || agent.ID == uuid.Nil || agent.OrgID == nil {
		return nil, fmt.Errorf("agent context is required")
	}
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}
	var schedule model.AgentSchedule
	err := db.WithContext(ctx).
		Where("org_id = ? AND agent_id = ? AND runtime_job_id = ?", *agent.OrgID, agent.ID, jobID).
		First(&schedule).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("schedule not found")
	}
	if err != nil {
		return nil, fmt.Errorf("load schedule: %w", err)
	}
	return &schedule, nil
}
