package agentschedule

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
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
	channelID := session.ChannelID.String()
	if strings.TrimSpace(input.ChannelID) != "" {
		if parsed, err := uuid.Parse(strings.TrimSpace(input.ChannelID)); err == nil && parsed != uuid.Nil {
			channelID = parsed.String()
		} else {
			return nil, fmt.Errorf("channel_id must be a uuid")
		}
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
	var sandboxID *uuid.UUID
	if agent.SandboxStrategy == "always_on" && session.SandboxID != nil {
		id := *session.SandboxID
		sandboxID = &id
	}
	schedule := model.AgentSchedule{
		OrgID:            *agent.OrgID,
		AgentID:          agent.ID,
		SandboxID:        sandboxID,
		RuntimeJobID:     jobID,
		Status:           StatusActive,
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
		channelID := strings.TrimSpace(*input.ChannelID)
		if channelID != "" {
			parsed, err := uuid.Parse(channelID)
			if err != nil || parsed == uuid.Nil {
				return nil, fmt.Errorf("channel_id must be a uuid")
			}
			channelID = parsed.String()
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

func NextRunAfter(schedule model.AgentSchedule, after time.Time) (time.Time, error) {
	kind := strings.ToLower(strings.TrimSpace(schedule.ScheduleKind))
	if kind == "" && strings.TrimSpace(schedule.CronExpression) != "" {
		kind = KindCron
	}
	switch kind {
	case KindCron:
		spec, err := cron.ParseStandard(strings.TrimSpace(schedule.CronExpression))
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid cron_expression: %w", err)
		}
		next := spec.Next(after.UTC())
		if next.IsZero() {
			return time.Time{}, fmt.Errorf("cron_expression has no future occurrence")
		}
		return next.UTC(), nil
	default:
		if schedule.IntervalSeconds == nil || *schedule.IntervalSeconds <= 0 {
			return time.Time{}, fmt.Errorf("interval_seconds must be positive")
		}
		next := after.UTC().Add(time.Duration(*schedule.IntervalSeconds) * time.Second)
		return next.UTC(), nil
	}
}

func RunKey(jobID string, scheduledAt time.Time) string {
	return strings.TrimSpace(jobID) + ":" + scheduledAt.UTC().Format(time.RFC3339)
}

func normalizeCadence(now time.Time, interval *int64, expression string) (string, time.Time, error) {
	expression = strings.TrimSpace(expression)
	hasInterval := interval != nil
	hasCron := expression != ""
	if hasInterval == hasCron {
		return "", time.Time{}, fmt.Errorf("provide exactly one of interval_seconds or cron_expression")
	}
	if hasCron {
		spec, err := cron.ParseStandard(expression)
		if err != nil {
			return "", time.Time{}, fmt.Errorf("invalid cron_expression: %w", err)
		}
		next := spec.Next(now.UTC())
		if next.IsZero() {
			return "", time.Time{}, fmt.Errorf("cron_expression has no future occurrence")
		}
		return KindCron, next.UTC(), nil
	}
	if *interval <= 0 {
		return "", time.Time{}, fmt.Errorf("interval_seconds must be positive")
	}
	return KindInterval, now.UTC().Add(time.Duration(*interval) * time.Second), nil
}

func defaultDescription(description, taskPrompt string) string {
	description = strings.TrimSpace(description)
	if description != "" {
		return description
	}
	taskPrompt = strings.TrimSpace(taskPrompt)
	if len(taskPrompt) <= 80 {
		return taskPrompt
	}
	return taskPrompt[:80]
}
