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

	scenario := startAgentRuntimeE2EScenario(
		t,
		trace,
		ctx,
		agentRuntimeE2EScenarioOptions{name: "scheduling", workspaceRoot: t.TempDir()},
		func(proxyURL, controlPlaneURL, sandboxID string) map[string]any {
			return agentRuntimeFeatureDefinition(t, proxyURL, controlPlaneURL, sandboxID, "Runtime E2E Scheduler Agent", strings.Join([]string{
				"You are executing the Hivy runtime scheduling E2E.",
				"Use the exact wake tool requested by the user.",
				"Finish only after wake scheduling succeeds.",
			}, "\n"), agentRuntimeSchedulingTools(), []any{}, []any{})
		},
	)

	response, events, responseEvents := runAgentRuntimeMessage(t, trace, ctx, scenario.baseURL, scenario.runtimeSecret, schedulingE2ERequest())
	assertToolCalls(t, events, "wake")
	assertRuntimeSessionFinal(t, trace, responseEvents, []string{"SCHEDULING_TOOLS_E2E_PASS"})
	assertAgentRuntimePostRunAPIs(t, trace, ctx, scenario.baseURL, scenario.runtimeSecret, response.SessionID, response.TraceID, scenario.workspaceRoot)

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
			"Complete the wake E2E using the named tool.",
			"1. Call wake with seconds 2 and task_prompt 'Final answer WAKE_E2E_DONE.'",
			"2. Final answer for this turn must include SCHEDULING_TOOLS_E2E_PASS.",
		}, "\n"),
		"raw": map[string]any{"source": "session", "test": "agent-runtime-scheduling-e2e"},
	}
}
