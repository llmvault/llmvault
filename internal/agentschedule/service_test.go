package agentschedule

import (
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/model"
)

func TestNextRunAfterRejectsInvalidCadence(t *testing.T) {
	t.Run("invalid cron", func(t *testing.T) {
		_, err := NextRunAfter(model.AgentSchedule{
			ScheduleKind:   KindCron,
			CronExpression: "not a cron",
		}, time.Now())
		if err == nil {
			t.Fatal("expected invalid cron expression to fail")
		}
	})

	t.Run("invalid interval", func(t *testing.T) {
		interval := int64(0)
		_, err := NextRunAfter(model.AgentSchedule{
			ScheduleKind:    KindInterval,
			IntervalSeconds: &interval,
		}, time.Now())
		if err == nil {
			t.Fatal("expected non-positive interval to fail")
		}
	})
}

func TestNextRunAfterComputesRecurringCadence(t *testing.T) {
	after := time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)

	nextCron, err := NextRunAfter(model.AgentSchedule{
		ScheduleKind:   KindCron,
		CronExpression: "*/5 * * * *",
	}, after)
	if err != nil {
		t.Fatalf("cron next run: %v", err)
	}
	if want := after.Add(5 * time.Minute); !nextCron.Equal(want) {
		t.Fatalf("cron next run = %s, want %s", nextCron, want)
	}

	interval := int64(30)
	nextInterval, err := NextRunAfter(model.AgentSchedule{
		ScheduleKind:    KindInterval,
		IntervalSeconds: &interval,
	}, after)
	if err != nil {
		t.Fatalf("interval next run: %v", err)
	}
	if want := after.Add(30 * time.Second); !nextInterval.Equal(want) {
		t.Fatalf("interval next run = %s, want %s", nextInterval, want)
	}
}
