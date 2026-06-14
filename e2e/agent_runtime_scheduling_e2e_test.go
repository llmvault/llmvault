package e2e

import (
	"strings"
	"testing"
	"time"
)

func TestAgentRuntimeSchedulingE2E(t *testing.T) {
	ctx, cancel, trace := agentRuntimeE2EContext(t, 35*time.Minute)
	defer cancel()
	trace.Logf("start", "scheduling runtime E2E starting")

	schedules := []any{configScheduleE2EJob()}
	scenario := startAgentRuntimeE2EScenario(
		t,
		trace,
		ctx,
		agentRuntimeE2EScenarioOptions{name: "scheduling", workspaceRoot: t.TempDir(), schedules: schedules},
		func(proxyURL, controlPlaneURL, sandboxID string) map[string]any {
			return agentRuntimeFeatureDefinition(t, proxyURL, controlPlaneURL, sandboxID, "Runtime E2E Scheduler Agent", strings.Join([]string{
				"You are executing the Hivy runtime scheduling E2E.",
				"Use the exact cron and wake tools requested by the user.",
				"Keep the cron job interval long enough that it does not run before you cancel it.",
				"Finish only after every requested cron operation and wake scheduling operation succeeds.",
			}, "\n"), agentRuntimeSchedulingTools(), []any{}, []any{})
		},
	)

	response, events, responseEvents := runAgentRuntimeMessage(t, trace, ctx, scenario.baseURL, scenario.runtimeSecret, schedulingE2ERequest())
	assertToolCalls(t, events, "cron", "wake")
	assertRuntimeSessionFinal(t, trace, responseEvents, []string{"SCHEDULING_TOOLS_E2E_PASS"})
	assertAgentRuntimePostRunAPIs(t, trace, ctx, scenario.baseURL, scenario.runtimeSecret, response.SessionID, response.TraceID, scenario.workspaceRoot)

	for _, eventType := range []string{"schedule.created", "schedule.updated", "schedule.paused", "schedule.resumed", "schedule.cancelled"} {
		scenario.controlPlane.waitForWebhookEventType(t, eventType)
	}
	scenario.controlPlane.waitForWebhookPayloadContaining(t, "schedule.run_started", "config-schedule-e2e")
	scenario.controlPlane.waitForWebhookPayloadContaining(t, "schedule.run_completed", "config-schedule-e2e")
	scenario.controlPlane.waitForWebhookPayloadContaining(t, "final", "CONFIG_SCHEDULE_DONE")
	scenario.controlPlane.waitForWebhookPayloadContaining(t, "final", "WAKE_E2E_DONE")
	scenario.proxy.assertUsed(t)
	scenario.controlPlane.waitForActivity(t)
	trace.Logf("done", "scheduling runtime E2E completed")
}

func schedulingE2ERequest() map[string]any {
	return map[string]any{
		"session_id": "agent-runtime-scheduling-e2e",
		"user":       "agent-runtime-e2e",
		"text": strings.Join([]string{
			"Complete the scheduling E2E using the named tools.",
			"1. Call cron create with interval_seconds 120, repeat_count 1, description agent-managed-cron-e2e, and task_prompt 'Do not run during this E2E'.",
			"2. Call cron list.",
			"3. Call cron update for the created job_id with interval_seconds 180 and task_prompt 'Updated during scheduling E2E'.",
			"4. Call cron pause for the same job_id.",
			"5. Call cron resume for the same job_id.",
			"6. Call cron cancel for the same job_id.",
			"7. Call wake with seconds 2 and task_prompt 'Final answer WAKE_E2E_DONE.'",
			"8. Final answer for this turn must include SCHEDULING_TOOLS_E2E_PASS.",
		}, "\n"),
		"raw": map[string]any{"source": "session", "test": "agent-runtime-scheduling-e2e"},
	}
}

func configScheduleE2EJob() map[string]any {
	now := time.Now().UTC()
	return map[string]any{
		"id":                      "config-schedule-e2e",
		"description":             "Config schedule E2E",
		"channel":                 "http",
		"task_prompt":             "Final answer CONFIG_SCHEDULE_DONE.",
		"cron_expression":         nil,
		"interval_seconds":        2,
		"repeat_count":            1,
		"repeat_completed":        0,
		"state":                   "active",
		"next_run_at":             now.Add(2 * time.Second).Format(time.RFC3339Nano),
		"last_run_at":             nil,
		"last_status":             nil,
		"last_error":              nil,
		"session_continuation_id": nil,
		"created_at":              now.Format(time.RFC3339Nano),
		"created_by_session":      "config-schedule-e2e",
	}
}
