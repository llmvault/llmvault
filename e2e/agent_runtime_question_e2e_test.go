package e2e

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestAgentRuntimeQuestionE2E(t *testing.T) {
	ctx, cancel, trace := agentRuntimeE2EContext(t, 10*time.Minute)
	defer cancel()

	scenario := startAgentRuntimeE2EScenario(t, trace, ctx, agentRuntimeE2EScenarioOptions{name: "question"}, func(proxyURL, controlPlaneURL, sandboxID string) map[string]any {
		return agentRuntimeQuestionDefinition(t, trace, proxyURL, controlPlaneURL, sandboxID)
	})

	sessionID := "agent-runtime-question-e2e"
	response := sendAgentRuntimeMessage(t, trace, ctx, scenario.baseURL, scenario.runtimeSecret, map[string]any{
		"session_id": sessionID,
		"user":       "agent-runtime-e2e",
		"text": strings.Join([]string{
			"You must call request_user_input before finalizing.",
			"Ask exactly one question with id deployment_path, header Deploy, question 'Which deployment path should the runtime E2E choose?', and options Ship It and Hold.",
			"After the answer arrives, final answer must include QUESTION_E2E_PASS and SELECTED_SHIP_IT if Ship It was selected.",
		}, "\n"),
		"raw": map[string]any{"source": "direct-browser-question-e2e"},
	})

	questionCh := make(chan runtimeSSEEvent, 1)
	streamDone := make(chan directRuntimeSSEResult, 1)
	directURL := directRuntimeStreamURL(t, scenario.baseURL, response.StreamURL)
	observer := func(event runtimeSSEEvent) {
		if event.Name != "question_requested" {
			return
		}
		select {
		case questionCh <- event:
		default:
		}
	}
	go func() {
		events, err := readRuntimeSSEClient(
			ctx,
			trace,
			"question-direct",
			directURL,
			directRuntimeJWT(t, scenario.runtimeSecret, sessionID, scenario.sandboxID, "stream:read"),
			observer,
		)
		streamDone <- directRuntimeSSEResult{events: events, err: err}
	}()

	questionEvent := waitForQuestionRequested(t, trace, questionCh)
	questionRequestID := assertQuestionRequestedPayload(t, questionEvent, sessionID)
	answerBody := questionAnswerBody(t)
	postQuestionAnswerWithRuntimeSecret(t, trace, ctx, scenario.baseURL, scenario.runtimeSecret, sessionID, questionRequestID, answerBody, http.StatusOK)
	postQuestionAnswerWithRuntimeSecret(t, trace, ctx, scenario.baseURL, scenario.runtimeSecret, sessionID, questionRequestID, answerBody, http.StatusConflict)

	result := <-streamDone
	if result.err != nil {
		t.Fatalf("question direct stream failed: %v", result.err)
	}
	assertToolCalls(t, result.events, "request_user_input")
	assertQuestionRuntimeEvents(t, trace, result.events)
	assertRuntimeSessionFinal(t, trace, result.events, []string{"QUESTION_E2E_PASS", "SELECTED_SHIP_IT"})
	assertAgentRuntimePostRunAPIs(t, trace, ctx, scenario.baseURL, scenario.runtimeSecret, sessionID, response.TraceID, scenario.workspaceRoot)
}

func agentRuntimeQuestionDefinition(t *testing.T, trace *agentRuntimeE2ETrace, proxyURL, controlPlaneURL, sandboxID string) map[string]any {
	t.Helper()
	modelID := strings.TrimSpace(os.Getenv("HIVY_AGENT_RUNTIME_E2E_MODEL"))
	if modelID == "" {
		modelID = "openai/gpt-4o-mini"
	}
	trace.Logf("definition", "question model_id=%s proxy_url=%s control_plane_url=%s", modelID, proxyURL, controlPlaneURL)
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
			"X-Title":      "Hivy question runtime E2E",
		},
	}
	return map[string]any{
		"agent": map[string]any{
			"name":        "Runtime E2E Question Agent",
			"description": "Verifies blocking user input in the runtime.",
		},
		"system_prompt": map[string]any{
			"cacheable_segments": []any{map[string]any{"type": "static_text", "config": map[string]any{
				"title": "Question Contract",
				"content": strings.Join([]string{
					"You are verifying the Hivy runtime request_user_input tool.",
					"When the user asks for a question, call request_user_input exactly once with the requested fields.",
					"Use the tool result to decide the final marker. Do not finalize before the tool returns.",
				}, "\n"),
			}}},
			"dynamic_segments": []any{},
		},
		"model":            model,
		"multimodal_model": nil,
		"limits":           map[string]any{"max_turns_per_session": 20, "input_token_budget": 30000, "output_token_budget": 3000, "tool_call_timeout_seconds": 900},
		"context":          map[string]any{"max_history_events": 20},
		"tools":            []any{map[string]any{"type": "builtin.request_user_input"}},
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

func waitForQuestionRequested(t *testing.T, trace *agentRuntimeE2ETrace, ch <-chan runtimeSSEEvent) runtimeSSEEvent {
	t.Helper()
	select {
	case event := <-ch:
		trace.Logf("question", "observed question_requested payload=%s", event.RawData)
		return event
	case <-time.After(3 * time.Minute):
		t.Fatalf("timed out waiting for question_requested")
		return runtimeSSEEvent{}
	}
}

func assertQuestionRequestedPayload(t *testing.T, event runtimeSSEEvent, sessionID string) string {
	t.Helper()
	if got, _ := event.Payload["session_id"].(string); got != sessionID {
		t.Fatalf("question_requested session_id=%q want=%q payload=%v", got, sessionID, event.Payload)
	}
	questionRequestID, _ := event.Payload["question_request_id"].(string)
	if strings.TrimSpace(questionRequestID) == "" {
		t.Fatalf("question_requested missing question_request_id payload=%v", event.Payload)
	}
	questions, ok := event.Payload["questions"].([]any)
	if !ok || len(questions) != 1 {
		t.Fatalf("question_requested questions=%v want exactly one", event.Payload["questions"])
	}
	question, ok := questions[0].(map[string]any)
	if !ok {
		t.Fatalf("question_requested question shape=%T", questions[0])
	}
	if id, _ := question["id"].(string); id != "deployment_path" {
		t.Fatalf("question id=%q want deployment_path payload=%v", id, question)
	}
	if header, _ := question["header"].(string); header != "Deploy" {
		t.Fatalf("question header=%q want Deploy payload=%v", header, question)
	}
	return questionRequestID
}

func questionAnswerBody(t *testing.T) []byte {
	t.Helper()
	return mustJSON(t, map[string]any{
		"answers": map[string]any{
			"deployment_path": map[string]any{"answers": []string{"Ship It"}},
		},
		"user":              "agent-runtime-e2e",
		"user_display_name": "Runtime E2E",
	})
}

func postQuestionAnswerWithRuntimeSecret(t *testing.T, trace *agentRuntimeE2ETrace, ctx context.Context, baseURL, runtimeSecret, sessionID, questionRequestID string, body []byte, wantStatus int) {
	t.Helper()
	endpoint := fmt.Sprintf("%s/sessions/%s/questions/%s/answer", strings.TrimRight(baseURL, "/"), neturl.PathEscape(sessionID), neturl.PathEscape(questionRequestID))
	parsed, err := neturl.Parse(endpoint)
	if err != nil {
		t.Fatalf("parse question answer endpoint: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new question answer request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+runtimeSecret)
	trace.Body("question", fmt.Sprintf("POST %s request", parsed.String()), body)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post question answer failed: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	trace.Body("question", fmt.Sprintf("POST %s response status=%d", parsed.String(), resp.StatusCode), respBody)
	if resp.StatusCode != wantStatus {
		t.Fatalf("question answer status=%d want=%d body=%s", resp.StatusCode, wantStatus, respBody)
	}
}

func assertQuestionRuntimeEvents(t *testing.T, trace *agentRuntimeE2ETrace, events []runtimeSSEEvent) {
	t.Helper()
	counts := map[string]int{}
	for _, event := range events {
		counts[event.Name]++
		if event.Name == "tool_result" && !strings.Contains(event.RawData, "Ship It") {
			t.Fatalf("request_user_input tool_result did not include submitted answer: %s", event.RawData)
		}
	}
	trace.Logf("assert", "question stream event counts=%v", counts)
	for _, want := range []string{"question_requested", "question_answered", "tool_call", "tool_result", "final", "turn_completed"} {
		if counts[want] == 0 {
			t.Fatalf("question stream missing %s; events=%s", want, summarizeEvents(events))
		}
	}
}
