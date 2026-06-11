package employeeruntime

import (
	"context"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/model"
	"gorm.io/gorm"
)

func BuildRuntimeSchedules(ctx context.Context, db *gorm.DB, agent *model.Employee, sb *model.Sandbox) ([]RuntimeSchedule, error) {
	if db == nil || agent == nil {
		return nil, nil
	}
	var rows []model.EmployeeSchedule
	query := db.WithContext(ctx).
		Where("employee_id = ? AND cancelled_at IS NULL", agent.ID).
		Where("status IN ?", []string{"active", "paused"})
	if agent.OrgID != nil {
		query = query.Where("org_id = ?", *agent.OrgID)
	}
	if err := query.Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]RuntimeSchedule, 0, len(rows))
	for _, row := range rows {
		if row.RuntimeJobID == "" || strings.TrimSpace(row.TaskPrompt) == "" {
			continue
		}
		out = append(out, runtimeScheduleFromModel(row))
	}
	return out, nil
}

// RepointEmployeeSchedules repoints every active/paused schedule for the agent to the
// given sandbox. It must only be called after a successful main-runtime config push,
// and never for specialist sandboxes — schedules belong to the main employee runtime.
//
// Repointing is intentionally separated from BuildRuntimeSchedules (a read path) so a
// failed compile/push never mutates schedule rows. Because the schedule->sandbox FK is
// ON DELETE SET NULL, a later sandbox hard-delete nulls sandbox_id instead of cascading
// the schedule away.
func RepointEmployeeSchedules(ctx context.Context, db *gorm.DB, agent *model.Employee, sb *model.Sandbox) error {
	if db == nil || agent == nil || sb == nil {
		return nil
	}
	query := db.WithContext(ctx).Model(&model.EmployeeSchedule{}).
		Where("employee_id = ? AND cancelled_at IS NULL", agent.ID).
		Where("status IN ?", []string{"active", "paused"}).
		Where("sandbox_id IS DISTINCT FROM ?", sb.ID)
	if agent.OrgID != nil {
		query = query.Where("org_id = ?", *agent.OrgID)
	}
	return query.Update("sandbox_id", sb.ID).Error
}

func runtimeScheduleFromModel(row model.EmployeeSchedule) RuntimeSchedule {
	nextRunAt := row.NextRunAt
	if nextRunAt == nil {
		at := time.Now().UTC().Add(time.Minute)
		nextRunAt = &at
	}
	createdAt := row.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	state := "active"
	switch strings.ToLower(row.Status) {
	case "paused":
		state = "paused"
	case "completed":
		state = "completed"
	}
	return RuntimeSchedule{
		ID:               row.RuntimeJobID,
		Description:      row.Description,
		Channel:          row.Channel,
		TaskPrompt:       row.TaskPrompt,
		IntervalSeconds:  uint64PtrFromInt64(row.IntervalSeconds),
		RepeatCount:      uint32PtrFromInt64(row.RepeatCount),
		RepeatCompleted:  uint32Clamp(row.RepeatCompleted),
		State:            state,
		Source:           "cron",
		NextRunAt:        nextRunAt.UTC(),
		LastRunAt:        row.LastRunAt,
		LastStatus:       stringPtrNonEmpty(row.LastStatus),
		LastError:        stringPtrNonEmpty(row.LastError),
		CreatedAt:        createdAt.UTC(),
		CreatedBySession: row.CreatedBySession,
	}
}

func uint64PtrFromInt64(value *int64) *uint64 {
	if value == nil || *value < 0 {
		return nil
	}
	out := uint64(*value)
	return &out
}

func uint32PtrFromInt64(value *int64) *uint32 {
	if value == nil || *value < 0 {
		return nil
	}
	max := int64(^uint32(0))
	if *value > max {
		out := ^uint32(0)
		return &out
	}
	out := uint32(*value)
	return &out
}

func uint32Clamp(value int64) uint32 {
	if value <= 0 {
		return 0
	}
	max := int64(^uint32(0))
	if value > max {
		return ^uint32(0)
	}
	return uint32(value)
}

func stringPtrNonEmpty(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
