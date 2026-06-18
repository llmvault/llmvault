package handler

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/agentschedule"
	"github.com/usehivy/hivy/internal/model"
)

func syncAgentScheduleEvent(tx *gorm.DB, event model.SessionEvent) error {
	if event.EventType == "turn_completed" || event.EventType == "turn_failed" {
		return completeBackendScheduleRunFromTerminalTurn(tx, event)
	}
	if !strings.HasPrefix(event.EventType, "schedule.") {
		return nil
	}
	payload := map[string]any{}
	if len(event.Payload) > 0 {
		payload = map[string]any(event.Payload)
	}
	jobID := stringValue(payload, "job_id")
	if jobID == "" {
		return fmt.Errorf("schedule payload missing job_id")
	}
	if source := strings.ToLower(stringValue(payload, "source")); source != "" && source != "cron" {
		return nil
	}

	if strings.HasPrefix(event.EventType, "schedule.run_") {
		var schedule model.AgentSchedule
		err := tx.Where("agent_id = ? AND runtime_job_id = ?", event.AgentID, jobID).First(&schedule).Error
		if err == nil {
			return upsertAgentScheduleRunFromEvent(tx, event, payload, &schedule, jobID)
		}
		if err != gorm.ErrRecordNotFound {
			return fmt.Errorf("load agent schedule: %w", err)
		}
		schedulePtr, err := upsertAgentScheduleFromEvent(tx, event, payload, jobID)
		if err != nil {
			return err
		}
		return upsertAgentScheduleRunFromEvent(tx, event, payload, schedulePtr, jobID)
	}
	if _, err := upsertAgentScheduleFromEvent(tx, event, payload, jobID); err != nil {
		return err
	}
	return nil
}

func completeBackendScheduleRunFromTerminalTurn(tx *gorm.DB, event model.SessionEvent) error {
	if event.SessionID == uuid.Nil {
		return nil
	}
	status := agentschedule.RunStatusCompleted
	if event.EventType == "turn_failed" {
		status = agentschedule.RunStatusFailed
	}
	var run model.AgentScheduleRun
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("session_id = ? AND status IN ?", event.SessionID, []string{
			agentschedule.RunStatusQueued,
			agentschedule.RunStatusProcessing,
			"running",
		}).
		Order("created_at DESC").
		First(&run).Error
	if err == gorm.ErrRecordNotFound {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load schedule run for terminal turn: %w", err)
	}
	payload := map[string]any{}
	if len(event.Payload) > 0 {
		payload = map[string]any(event.Payload)
	}
	var durationMS *int64
	if run.StartedAt != nil {
		duration := event.EventAt.Sub(*run.StartedAt).Milliseconds()
		durationMS = &duration
	}
	updates := map[string]any{
		"status":        status,
		"completed_at":  event.EventAt,
		"duration_ms":   durationMS,
		"error":         "",
		"event_payload": rawScheduleEventPayload(event.Payload),
		"lease_owner":   "",
		"leased_until":  nil,
		"updated_at":    time.Now(),
	}
	if event.SandboxID != nil {
		updates["sandbox_id"] = *event.SandboxID
	}
	if status == agentschedule.RunStatusFailed {
		updates["error"] = firstNonEmpty(stringValue(payload, "error"), stringValue(payload, "message"))
	}
	if err := tx.Model(&model.AgentScheduleRun{}).Where("id = ?", run.ID).Updates(updates).Error; err != nil {
		return fmt.Errorf("complete schedule run from terminal turn: %w", err)
	}
	scheduleUpdates := map[string]any{
		"last_status": status,
		"last_error":  updates["error"],
		"updated_at":  time.Now(),
	}
	if err := tx.Model(&model.AgentSchedule{}).Where("id = ?", run.ScheduleID).Updates(scheduleUpdates).Error; err != nil {
		return fmt.Errorf("update schedule terminal status: %w", err)
	}
	return nil
}

func upsertAgentScheduleFromEvent(tx *gorm.DB, event model.SessionEvent, payload map[string]any, jobID string) (*model.AgentSchedule, error) {
	status := scheduleStatusFromEvent(event.EventType, payload)
	cancelledAt := (*time.Time)(nil)
	if event.EventType == "schedule.cancelled" {
		cancelledAt = &event.EventAt
	}
	schedule := model.AgentSchedule{
		OrgID:            event.OrgID,
		AgentID:          event.AgentID,
		SandboxID:        event.SandboxID,
		RuntimeJobID:     jobID,
		Status:           status,
		Channel:          stringValue(payload, "channel"),
		Description:      stringValue(payload, "description"),
		TaskPrompt:       stringValue(payload, "task_prompt"),
		IntervalSeconds:  int64PtrFromPayload(payload, "interval_seconds"),
		RepeatCount:      int64PtrFromPayload(payload, "repeat_count"),
		RepeatCompleted:  int64Value(payload, "repeat_completed"),
		NextRunAt:        timePtrFromPayload(payload, "next_run_at"),
		LastRunAt:        timePtrFromPayload(payload, "last_run_at"),
		LastStatus:       latestScheduleStatus(event.EventType, payload),
		LastError:        firstNonEmpty(stringValue(payload, "last_error"), stringValue(payload, "error")),
		CreatedBySession: stringValue(payload, "created_by_session"),
		RuntimeCreatedAt: timePtrFromPayload(payload, "created_at"),
		CancelledAt:      cancelledAt,
	}
	err := tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "agent_id"}, {Name: "runtime_job_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"org_id":             schedule.OrgID,
			"sandbox_id":         schedule.SandboxID,
			"status":             schedule.Status,
			"channel":            schedule.Channel,
			"description":        schedule.Description,
			"task_prompt":        schedule.TaskPrompt,
			"interval_seconds":   schedule.IntervalSeconds,
			"repeat_count":       schedule.RepeatCount,
			"repeat_completed":   schedule.RepeatCompleted,
			"next_run_at":        schedule.NextRunAt,
			"last_run_at":        schedule.LastRunAt,
			"last_status":        schedule.LastStatus,
			"last_error":         schedule.LastError,
			"created_by_session": schedule.CreatedBySession,
			"runtime_created_at": schedule.RuntimeCreatedAt,
			"cancelled_at":       schedule.CancelledAt,
			"updated_at":         time.Now(),
		}),
	}).Create(&schedule).Error
	if err != nil {
		return nil, fmt.Errorf("upsert agent schedule: %w", err)
	}
	if err := tx.Where("agent_id = ? AND runtime_job_id = ?", event.AgentID, jobID).First(&schedule).Error; err != nil {
		return nil, fmt.Errorf("load agent schedule: %w", err)
	}
	return &schedule, nil
}

func upsertAgentScheduleRunFromEvent(tx *gorm.DB, event model.SessionEvent, payload map[string]any, schedule *model.AgentSchedule, jobID string) error {
	scheduledAt := timePtrFromPayload(payload, "scheduled_at")
	runKey := stringValue(payload, "run_key")
	if runKey == "" {
		if scheduledAt != nil {
			runKey = jobID + ":" + scheduledAt.Format(time.RFC3339)
		} else {
			runKey = jobID + ":" + event.EventAt.Format(time.RFC3339Nano)
		}
	}
	run := model.AgentScheduleRun{
		OrgID:        event.OrgID,
		AgentID:      event.AgentID,
		ScheduleID:   schedule.ID,
		SandboxID:    sessionEventSandboxID(event),
		RuntimeJobID: jobID,
		RunKey:       runKey,
		Status:       runStatusFromEvent(event.EventType),
		ScheduledAt:  scheduledAt,
		StartedAt:    timePtrFromPayload(payload, "started_at"),
		CompletedAt:  timePtrFromPayload(payload, "completed_at"),
		DurationMS:   int64PtrFromPayload(payload, "duration_ms"),
		Error:        firstNonEmpty(stringValue(payload, "error"), stringValue(payload, "last_error")),
		EventPayload: rawScheduleEventPayload(event.Payload),
	}
	if run.StartedAt == nil && event.EventType == "schedule.run_started" {
		run.StartedAt = &event.EventAt
	}
	if run.CompletedAt == nil && (event.EventType == "schedule.run_completed" || event.EventType == "schedule.run_failed") {
		run.CompletedAt = &event.EventAt
	}
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "schedule_id"}, {Name: "run_key"}},
		DoUpdates: clause.Assignments(map[string]any{
			"org_id":         run.OrgID,
			"agent_id":       run.AgentID,
			"sandbox_id":     run.SandboxID,
			"runtime_job_id": run.RuntimeJobID,
			"status":         run.Status,
			"scheduled_at":   run.ScheduledAt,
			"started_at":     run.StartedAt,
			"completed_at":   run.CompletedAt,
			"duration_ms":    run.DurationMS,
			"error":          run.Error,
			"event_payload":  run.EventPayload,
			"updated_at":     time.Now(),
		}),
	}).Create(&run).Error
}
