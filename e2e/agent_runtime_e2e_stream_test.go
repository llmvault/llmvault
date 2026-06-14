package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type runtimeSSEEvent struct {
	Name    string
	Payload map[string]any
	RawData string
}

type runtimeSSEObserver func(runtimeSSEEvent)

func readRuntimeSSE(t *testing.T, trace *agentRuntimeE2ETrace, ctx context.Context, url, token string, observer runtimeSSEObserver) []runtimeSSEEvent {
	t.Helper()
	events, err := readRuntimeSSEClient(ctx, trace, "parent", url, token, observer)
	if err != nil {
		t.Fatalf("read parent stream: %v", err)
	}
	return events
}

func readRuntimeSSEClient(ctx context.Context, trace *agentRuntimeE2ETrace, label, url, token string, observer runtimeSSEObserver) ([]runtimeSSEEvent, error) {
	trace.Logf("sse", "%s opening stream url=%s", label, url)
	streamCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(streamCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("new stream request: %w", err)
	}
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("open stream: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("stream status=%d body=%s", resp.StatusCode, body)
	}
	if strings.TrimSpace(token) == "" && resp.Header.Get("Access-Control-Allow-Origin") == "" {
		return nil, fmt.Errorf("direct stream missing access-control-allow-origin header")
	}
	trace.Logf("sse", "%s stream opened status=%d", label, resp.StatusCode)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024), 2*1024*1024)
	var events []runtimeSSEEvent
	var name string
	var dataLines []string
	flush := func() bool {
		if name == "" && len(dataLines) == 0 {
			return false
		}
		raw := strings.Join(dataLines, "\n")
		payload := map[string]any{}
		if raw != "" {
			if err := json.Unmarshal([]byte(raw), &payload); err != nil {
				payload = map[string]any{"_raw": raw}
			}
		}
		event := runtimeSSEEvent{Name: name, Payload: payload, RawData: raw}
		events = append(events, event)
		traceRuntimeSSEEvent(trace, label, len(events), event)
		if observer != nil {
			observer(event)
		}
		done := name == "done"
		name = ""
		dataLines = nil
		return done
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if flush() {
				trace.Logf("sse", "%s stream completed events=%d", label, len(events))
				return events, nil
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			name = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read stream: %w", err)
	}
	flush()
	trace.Logf("sse", "%s stream ended without explicit done events=%d", label, len(events))
	return events, nil
}

func assertRuntimeE2EEvents(t *testing.T, trace *agentRuntimeE2ETrace, events []runtimeSSEEvent) {
	t.Helper()
	if len(events) == 0 {
		t.Fatalf("no runtime stream events")
	}
	eventNames := map[string]int{}
	tools := map[string]int{}
	agents := map[string]int{}
	var finalText strings.Builder
	for _, event := range events {
		eventNames[event.Name]++
		if event.Name == "tool_call" {
			if tool, _ := event.Payload["tool"].(string); tool != "" {
				tools[tool]++
			}
		}
		if event.Name == "subagent_started" || event.Name == "subagent_completed" {
			if agent, _ := event.Payload["agent_name"].(string); agent != "" {
				agents[agent]++
			}
		}
		if event.Name == "final" {
			if text, _ := event.Payload["text"].(string); text != "" {
				finalText.WriteString(text)
				finalText.WriteString("\n")
			}
		}
	}
	trace.Logf("assert", "stream event counts=%v", eventNames)
	trace.Logf("assert", "stream tool counts=%v", tools)
	trace.Logf("assert", "stream subagent lifecycle counts=%v", agents)
	for _, want := range []string{"tool_call", "tool_result", "subagent_started", "subagent_completed", "final", "turn_completed"} {
		if eventNames[want] == 0 {
			t.Fatalf("stream did not contain %s; names=%v events=%s", want, eventNames, summarizeEvents(events))
		}
	}
	for _, tool := range []string{"search_sessions", "fixture_requirements", "read_file", "write_file", "edit_file", "bash", "check_bash_status", "subagent_task", "check_subagent_task_status"} {
		if tools[tool] == 0 {
			t.Fatalf("missing required tool call %s; tools=%v events=%s", tool, tools, summarizeEvents(events))
		}
	}
	if tools["subagent_task"] < 3 || tools["check_subagent_task_status"] < 3 {
		t.Fatalf("subagent tool counts too low: tools=%v events=%s", tools, summarizeEvents(events))
	}
	for _, agent := range []string{"planner", "qa", "reviewer"} {
		if agents[agent] == 0 {
			t.Fatalf("missing subagent lifecycle event for %s; agents=%v events=%s", agent, agents, summarizeEvents(events))
		}
	}
	text := finalText.String()
	for _, want := range []string{agentRuntimeE2EToken, "E2E_PASS", "PLANNER_SUBAGENT_CONFIRMED", "QA_SUBAGENT_CONFIRMED", "REVIEW_SUBAGENT_CONFIRMED"} {
		if !strings.Contains(text, want) {
			t.Fatalf("final text missing %q; events=%s\nfinals:\n%s", want, summarizeEvents(events), text)
		}
	}
	trace.Body("assert", "final text", []byte(text))
	trace.Logf("assert", "stream assertions passed")
}

func assertRuntimeSessionFinal(t *testing.T, trace *agentRuntimeE2ETrace, events []runtimeSSEEvent, requiredText []string) {
	t.Helper()
	if len(events) == 0 {
		t.Fatalf("session stream produced no events")
	}
	var finalText strings.Builder
	eventNames := map[string]int{}
	for _, event := range events {
		eventNames[event.Name]++
		if event.Name == "final" {
			if text, _ := event.Payload["text"].(string); text != "" {
				finalText.WriteString(text)
				finalText.WriteString("\n")
			}
		}
	}
	trace.Logf("assert", "session stream event counts=%v", eventNames)
	for _, want := range []string{"final", "turn_completed"} {
		if eventNames[want] == 0 {
			t.Fatalf("session stream missing %s; events=%s", want, summarizeEvents(events))
		}
	}
	text := finalText.String()
	for _, want := range requiredText {
		if !strings.Contains(text, want) {
			t.Fatalf("session stream final text missing %q; events=%s\nfinals:\n%s", want, summarizeEvents(events), text)
		}
	}
	trace.Body("assert", "session stream final text", []byte(text))
}

func traceRuntimeSSEEvent(trace *agentRuntimeE2ETrace, label string, index int, event runtimeSSEEvent) {
	tool, _ := event.Payload["tool"].(string)
	agent, _ := event.Payload["agent_name"].(string)
	scope, _ := event.Payload["scope"].(string)
	if agent == "" {
		if subagent := payloadMap(event.Payload["subagent"]); subagent != nil {
			agent, _ = subagent["agent_name"].(string)
		}
	}
	trace.Logf("sse", "%s event #%d name=%s scope=%s tool=%s agent=%s", label, index, event.Name, scope, tool, agent)
	if event.RawData != "" {
		trace.Body("sse", fmt.Sprintf("%s event #%d %s payload", label, index, event.Name), []byte(event.RawData))
	}
}

func summarizeEvents(events []runtimeSSEEvent) string {
	parts := make([]string, 0, len(events))
	for _, event := range events {
		if event.Name == "tool_call" {
			parts = append(parts, fmt.Sprintf("%s:%v", event.Name, event.Payload["tool"]))
		} else {
			parts = append(parts, event.Name)
		}
	}
	if len(parts) > 80 {
		parts = parts[len(parts)-80:]
	}
	return strings.Join(parts, ",")
}
