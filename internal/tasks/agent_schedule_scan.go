package tasks

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/agentschedule"
	"github.com/usehivy/hivy/internal/model"
)

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
