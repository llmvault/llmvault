package agentschedule

import (
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/usehivy/hivy/internal/model"
)

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
