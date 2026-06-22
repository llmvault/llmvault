package e2e

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func assertAgentSessionsCanonicalIngestion(t *testing.T, events []agentSessionsEvent, sessionID, streamID, thinkingMarker, firstMarker, secondMarker string) {
	t.Helper()
	counts := map[string]int{}
	turnIDs := map[string]bool{}
	traceIDs := map[string]bool{}
	streamIDs := map[string]bool{}
	for _, event := range events {
		counts[event.EventType]++
		if isRuntimeCanonicalEvent(event.EventType) {
			assertRuntimeCanonicalPayload(t, event, sessionID)
			if turnID := eventString(event.Payload, "turn_id"); turnID != "" {
				turnIDs[turnID] = true
			}
			if traceID := eventString(event.Payload, "trace_id"); traceID != "" {
				traceIDs[traceID] = true
			}
			if got := eventString(event.Payload, "stream_id"); got != "" {
				streamIDs[got] = true
			}
		}
	}

	for _, want := range []string{"turn_started", "token", "thinking", "tool_result", "final", "turn_completed", "model_usage"} {
		if counts[want] == 0 {
			t.Fatalf("canonical ingestion missing %s; counts=%v events=%s", want, counts, summarizeSessionEventTypes(events))
		}
	}
	if counts["turn_started"] < 2 || counts["final"] < 2 || counts["turn_completed"] < 2 || counts["model_usage"] < 2 {
		t.Fatalf("canonical ingestion did not persist both turns; counts=%v", counts)
	}
	if len(turnIDs) < 2 || len(traceIDs) < 2 {
		t.Fatalf("canonical ingestion did not preserve distinct turn/trace ids: turn_ids=%v trace_ids=%v", turnIDs, traceIDs)
	}
	if streamID != "" && !streamIDs[streamID] {
		t.Fatalf("canonical ingestion did not preserve stable stream_id=%s; stream_ids=%v", streamID, streamIDs)
	}
	assertCoalescedDeltas(t, events, "token", "")
	assertCoalescedDeltas(t, events, "thinking", thinkingMarker)
	assertPersistedToolResult(t, events)
	assertPersistedFinals(t, events, firstMarker, secondMarker)
	assertPersistedModelUsage(t, events)
	t.Logf("canonical ingestion counts=%v turn_ids=%d trace_ids=%d stream_ids=%v", counts, len(turnIDs), len(traceIDs), streamIDs)
}

func assertRuntimeCanonicalPayload(t *testing.T, event agentSessionsEvent, sessionID string) {
	t.Helper()
	if got := eventString(event.Payload, "event_name"); got != event.EventType {
		t.Fatalf("event %s payload event_name=%q", event.EventType, got)
	}
	if got := eventString(event.Payload, "session_id"); got != sessionID {
		t.Fatalf("event %s payload session_id=%q want %q", event.EventType, got, sessionID)
	}
	if got := eventString(event.Payload, "event_id"); got == "" || got != event.EventID {
		t.Fatalf("event %s event_id mismatch row=%q payload=%q", event.EventType, event.EventID, got)
	}
	if got := eventString(event.Payload, "scope"); got == "" {
		t.Fatalf("event %s missing scope payload=%v", event.EventType, event.Payload)
	}
	if got := eventString(event.Payload, "occurred_at"); got == "" {
		t.Fatalf("event %s missing occurred_at payload=%v", event.EventType, event.Payload)
	}
	if event.RuntimeSeq == nil || *event.RuntimeSeq <= 0 {
		t.Fatalf("event %s missing positive runtime_seq row=%d runtime_seq=%v", event.EventType, event.SequenceNumber, event.RuntimeSeq)
	}
	if got := eventNumber(event.Payload, "runtime_seq"); got > 0 && int64(got) != *event.RuntimeSeq {
		t.Fatalf("event %s payload runtime_seq mismatch row_runtime_seq=%d payload=%v", event.EventType, *event.RuntimeSeq, got)
	}
	if _, ok := event.Payload["sequence"]; !ok {
		t.Fatalf("event %s missing payload sequence payload=%v", event.EventType, event.Payload)
	}
}

func assertCoalescedDeltas(t *testing.T, events []agentSessionsEvent, eventType, marker string) {
	t.Helper()
	found := false
	markerFound := marker == ""
	for _, event := range events {
		if event.EventType != eventType {
			continue
		}
		found = true
		text := eventString(event.Payload, "text")
		if text == "" {
			t.Fatalf("%s event missing accumulated text payload=%v", eventType, event.Payload)
		}
		if !eventBool(event.Payload, "coalesced") || eventNumber(event.Payload, "delta_count") < 1 {
			t.Fatalf("%s event is not coalesced payload=%v", eventType, event.Payload)
		}
		start := eventNumber(event.Payload, "sequence_start")
		end := eventNumber(event.Payload, "sequence_end")
		if start < 1 || end < start {
			t.Fatalf("%s event has invalid sequence range start=%v end=%v payload=%v", eventType, start, end, event.Payload)
		}
		if marker != "" && strings.Contains(text, marker) {
			markerFound = true
		}
	}
	if !found {
		t.Fatalf("missing %s event", eventType)
	}
	if !markerFound {
		t.Fatalf("%s events did not include marker %q", eventType, marker)
	}
}

func assertPersistedToolResult(t *testing.T, events []agentSessionsEvent) {
	t.Helper()
	for _, event := range events {
		if event.EventType != "tool_result" || eventString(event.Payload, "tool") != "bash" {
			continue
		}
		if got := eventString(event.Payload, "status"); got != "completed" {
			t.Fatalf("bash tool_result status=%q payload=%v", got, event.Payload)
		}
		if summary := eventString(event.Payload, "result_summary"); strings.Contains(summary, "first-turn-tool-done") {
			return
		}
	}
	t.Fatalf("missing persisted bash tool_result with first-turn-tool-done; events=%s", summarizeSessionEventTypes(events))
}

func assertPersistedFinals(t *testing.T, events []agentSessionsEvent, markers ...string) {
	t.Helper()
	var text strings.Builder
	for _, event := range events {
		if event.EventType == "final" {
			text.WriteString(eventString(event.Payload, "text"))
			text.WriteString("\n")
		}
	}
	for _, marker := range markers {
		if !strings.Contains(text.String(), marker) {
			t.Fatalf("persisted final events missing marker %s; final_text=%s", marker, text.String())
		}
	}
}

func assertAgentSessionsHistoryMatchesLiveMarkers(t *testing.T, events []agentSessionsEvent, markers ...string) {
	t.Helper()
	assertPersistedFinals(t, events, markers...)
}

func assertPersistedModelUsage(t *testing.T, events []agentSessionsEvent) {
	t.Helper()
	for _, event := range events {
		if event.EventType != "model_usage" {
			continue
		}
		usage, _ := event.Payload["usage"].(map[string]any)
		if eventNumber(usage, "prompt_tokens")+eventNumber(usage, "completion_tokens")+eventNumber(usage, "total_tokens") > 0 {
			return
		}
	}
	t.Fatalf("missing persisted model_usage with token counts; events=%s", summarizeSessionEventTypes(events))
}

func isRuntimeCanonicalEvent(eventType string) bool {
	switch eventType {
	case "turn_started", "token", "thinking", "tool_result", "final", "turn_completed", "turn_failed", "model_usage", "error", "session_waiting":
		return true
	default:
		return strings.HasPrefix(eventType, "question_") || strings.HasPrefix(eventType, "subagent_") || eventType == "plan_updated"
	}
}

func eventString(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	switch value := payload[key].(type) {
	case string:
		return strings.TrimSpace(value)
	case fmt.Stringer:
		return strings.TrimSpace(value.String())
	default:
		return ""
	}
}

func eventBool(payload map[string]any, key string) bool {
	if payload == nil {
		return false
	}
	value, _ := payload[key].(bool)
	return value
}

func eventNumber(payload map[string]any, key string) float64 {
	if payload == nil {
		return 0
	}
	switch value := payload[key].(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case json.Number:
		out, _ := value.Float64()
		return out
	default:
		return 0
	}
}

func summarizeSessionEventTypes(events []agentSessionsEvent) string {
	types := make([]string, 0, len(events))
	for _, event := range events {
		types = append(types, event.EventType)
	}
	if len(types) > 120 {
		types = types[:120]
	}
	return strings.Join(types, ",")
}
