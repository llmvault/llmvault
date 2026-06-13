package e2e

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestAgentRuntimeUpdatePlanE2E(t *testing.T) {
	ctx, cancel, trace := agentRuntimeE2EContext(t, 10*time.Minute)
	defer cancel()

	scenario := startAgentRuntimeE2EScenario(t, trace, ctx, agentRuntimeE2EScenarioOptions{name: "update-plan"}, func(proxyURL, controlPlaneURL, sandboxID string) map[string]any {
		return agentRuntimeUpdatePlanDefinition(t, trace, proxyURL, controlPlaneURL, sandboxID)
	})

	sessionID := "agent-runtime-update-plan-e2e"
	response := sendAgentRuntimeMessage(t, trace, ctx, scenario.baseURL, scenario.runtimeSecret, map[string]any{
		"session_id": sessionID,
		"user":       "agent-runtime-e2e",
		"text": strings.Join([]string{
			"You must call update_plan exactly twice before finalizing.",
			"First call: explanation 'Starting update plan E2E' and a three-step plan.",
			"Use these exact steps: Inspect update_plan tool, Publish live plan update, Finish update_plan E2E.",
			"First statuses must be in_progress, pending, pending.",
			"Second call: explanation 'Completed update plan E2E' with the same three steps all completed.",
			"Final answer must include UPDATE_PLAN_E2E_PASS.",
		}, "\n"),
		"raw": map[string]any{"source": "direct-browser-update-plan-e2e"},
	})

	events := readDirectRuntimeSSEAsync(trace, ctx, directRuntimeStreamURL(t, scenario.baseURL, response.StreamURL)).wait(t)
	assertToolCalls(t, events, "update_plan")
	assertUpdatePlanRuntimeEvents(t, trace, events, sessionID)
	assertRuntimeSessionFinal(t, trace, events, []string{"UPDATE_PLAN_E2E_PASS"})
	assertAgentRuntimePostRunAPIs(t, trace, ctx, scenario.baseURL, scenario.runtimeSecret, sessionID, response.TraceID, scenario.workspaceRoot)
}

func agentRuntimeUpdatePlanDefinition(t *testing.T, trace *agentRuntimeE2ETrace, proxyURL, controlPlaneURL, sandboxID string) map[string]any {
	t.Helper()
	modelID := strings.TrimSpace(os.Getenv("HIVY_AGENT_RUNTIME_E2E_MODEL"))
	if modelID == "" {
		modelID = "openai/gpt-4o-mini"
	}
	trace.Logf("definition", "update_plan model_id=%s proxy_url=%s control_plane_url=%s", modelID, proxyURL, controlPlaneURL)
	model := map[string]any{
		"provider":          "openai_compatible",
		"base_url":          strings.TrimRight(proxyURL, "/") + "/v1",
		"model_id":          modelID,
		"api_key_env":       "HIVY_PROXY_API_KEY",
		"temperature":       0,
		"max_output_tokens": 2048,
		"reasoning_effort":  "low",
		"extra_headers": map[string]string{
			"HTTP-Referer": "https://usehivy.com",
			"X-Title":      "Hivy update_plan runtime E2E",
		},
	}
	return map[string]any{
		"agent": map[string]any{
			"name":        "Runtime E2E Plan Agent",
			"description": "Verifies live plan updates in the runtime.",
		},
		"system_prompt": map[string]any{
			"cacheable_segments": []any{map[string]any{"type": "static_text", "config": map[string]any{
				"title": "Plan Contract",
				"content": strings.Join([]string{
					"You are verifying the Hivy runtime update_plan tool.",
					"When the user requests plan updates, call update_plan with the exact requested plan state.",
					"Use update_plan as a full replacement checklist. Do not finalize before both requested calls succeed.",
				}, "\n"),
			}}},
			"dynamic_segments": []any{},
		},
		"model":            model,
		"multimodal_model": nil,
		"limits":           map[string]any{"max_turns_per_session": 20, "input_token_budget": 30000, "output_token_budget": 3000, "tool_call_timeout_seconds": 120},
		"context":          map[string]any{"max_history_events": 20, "memory": map[string]any{"entries": []any{}, "token_budget": 1000}},
		"tools":            []any{map[string]any{"type": "builtin.update_plan"}},
		"mcp_servers":      []any{},
		"skills":           []any{},
		"outbound_channels": []any{map[string]any{
			"name":       "control-plane",
			"type":       "webhook",
			"url":        controlPlaneURL + "/internal/webhooks/agent/" + sandboxID,
			"secret_env": "HIVY_RUNTIME_SECRET",
		}},
		"sub_agents": map[string]any{},
		"safety":     map[string]any{},
	}
}

func assertUpdatePlanRuntimeEvents(t *testing.T, trace *agentRuntimeE2ETrace, events []runtimeSSEEvent, sessionID string) {
	t.Helper()
	var updates []runtimeSSEEvent
	toolCalls := 0
	toolResults := 0
	for _, event := range events {
		if event.Name == "plan_updated" {
			updates = append(updates, event)
		}
		if event.Name == "tool_call" && event.Payload["tool"] == "update_plan" {
			toolCalls++
		}
		if event.Name == "tool_result" && strings.Contains(event.RawData, `"ok":true`) {
			toolResults++
		}
	}
	trace.Logf("assert", "update_plan calls=%d results=%d plan_updated=%d", toolCalls, toolResults, len(updates))
	if len(updates) < 2 || toolCalls < 2 || toolResults < 2 {
		t.Fatalf("expected at least two update_plan calls/results/events; calls=%d results=%d updates=%d events=%s", toolCalls, toolResults, len(updates), summarizeEvents(events))
	}
	assertPlanUpdateEvent(t, updates[0], sessionID, "Starting update plan E2E", []string{"in_progress", "pending", "pending"})
	assertPlanUpdateEvent(t, updates[len(updates)-1], sessionID, "Completed update plan E2E", []string{"completed", "completed", "completed"})
}

func assertPlanUpdateEvent(t *testing.T, event runtimeSSEEvent, sessionID, explanation string, statuses []string) {
	t.Helper()
	if got, _ := event.Payload["session_id"].(string); got != sessionID {
		t.Fatalf("plan_updated session_id=%q want=%q payload=%v", got, sessionID, event.Payload)
	}
	if got, _ := event.Payload["explanation"].(string); got != explanation {
		t.Fatalf("plan_updated explanation=%q want=%q payload=%v", got, explanation, event.Payload)
	}
	plan, ok := event.Payload["plan"].([]any)
	if !ok || len(plan) != len(statuses) {
		t.Fatalf("plan_updated plan=%v want %d items", event.Payload["plan"], len(statuses))
	}
	for i, wantStatus := range statuses {
		item, ok := plan[i].(map[string]any)
		if !ok {
			t.Fatalf("plan item %d shape=%T", i, plan[i])
		}
		if step, _ := item["step"].(string); strings.TrimSpace(step) == "" {
			t.Fatalf("plan item %d missing step payload=%v", i, item)
		}
		if got, _ := item["status"].(string); got != wantStatus {
			t.Fatalf("plan item %d status=%q want=%q payload=%v", i, got, wantStatus, item)
		}
	}
}
