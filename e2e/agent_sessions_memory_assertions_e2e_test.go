package e2e

import (
	"encoding/json"
	"strings"
	"testing"
)

func assertAgentSessionsMemoryRetainEvents(t *testing.T, events []runtimeSSEEvent, facts []agentSessionsMemoryFact) {
	t.Helper()
	if count := agentSessionsToolCallCount(events, agentSessionsRetainTool); count < len(facts) {
		t.Fatalf("memory_retain tool calls=%d want at least %d events=%s", count, len(facts), agentSessionsMemoryEventDebug(events))
	}
	for _, fact := range facts {
		content := agentSessionsMemoryFactContent(fact)
		if !agentSessionsStreamToolCallContains(events, agentSessionsRetainTool, content) {
			t.Fatalf("memory_retain call did not include content %q events=%s", content, agentSessionsMemoryEventDebug(events))
		}
	}
	assertAgentSessionsMemoryToolResults(t, events, agentSessionsRetainTool, len(facts), nil)
}

func assertAgentSessionsMemoryRecallEvents(t *testing.T, events []runtimeSSEEvent, facts []agentSessionsMemoryFact) {
	t.Helper()
	if count := agentSessionsToolCallCount(events, agentSessionsRecallTool); count == 0 {
		t.Fatalf("memory_recall tool call missing events=%s", agentSessionsMemoryEventDebug(events))
	}
	assertAgentSessionsMemoryToolResults(t, events, agentSessionsRecallTool, 1, facts)
	assertAgentSessionsMemoryFinalEvents(t, events, facts)
}

func assertAgentSessionsMemoryToolResults(t *testing.T, events []runtimeSSEEvent, tool string, min int, facts []agentSessionsMemoryFact) {
	t.Helper()
	toolByID := agentSessionsToolCallsByID(events)
	count := 0
	var resultText strings.Builder
	for _, event := range events {
		if event.Name != "tool_result" || toolByID[agentSessionsEventID(event)] != tool {
			continue
		}
		count++
		text := agentSessionsMemoryToolResultText(event)
		resultText.WriteString(text)
		resultText.WriteString("\n")
		if agentSessionsToolResultError(event) != "" {
			t.Fatalf("%s tool result errored: %s event=%s", tool, agentSessionsToolResultError(event), event.RawData)
		}
	}
	if count < min {
		t.Fatalf("%s tool results=%d want at least %d events=%s", tool, count, min, agentSessionsMemoryEventDebug(events))
	}
	for _, fact := range facts {
		if !strings.Contains(resultText.String(), fact.Key) || !strings.Contains(resultText.String(), fact.Value) {
			t.Fatalf("%s tool results missing recalled fact %s=%s result=%s events=%s", tool, fact.Key, fact.Value, resultText.String(), agentSessionsMemoryEventDebug(events))
		}
	}
}

func assertAgentSessionsMemoryFinalEvents(t *testing.T, events []runtimeSSEEvent, facts []agentSessionsMemoryFact) {
	t.Helper()
	var finalText strings.Builder
	for _, event := range events {
		if event.Name == "final" {
			finalText.WriteString(eventString(event.Payload, "text"))
			finalText.WriteString(event.RawData)
			finalText.WriteString("\n")
		}
	}
	assertAgentSessionsMemoryTextContainsFacts(t, finalText.String(), facts, "recall stream final")
}

func assertAgentSessionsMemoryFinalPayload(t *testing.T, payload map[string]any, facts []agentSessionsMemoryFact) {
	t.Helper()
	raw, _ := json.Marshal(payload)
	assertAgentSessionsMemoryTextContainsFacts(t, string(raw), facts, "persisted recall final")
}

func assertAgentSessionsMemoryTextContainsFacts(t *testing.T, text string, facts []agentSessionsMemoryFact, label string) {
	t.Helper()
	for _, fact := range facts {
		if !strings.Contains(text, fact.Key) || !strings.Contains(text, fact.Value) {
			t.Fatalf("%s missing recalled fact %s=%s text=%s", label, fact.Key, fact.Value, text)
		}
	}
}

func agentSessionsStreamToolCallContains(events []runtimeSSEEvent, tool, text string) bool {
	for _, event := range events {
		if event.Name == "tool_call" && agentSessionsEventTool(event) == tool && strings.Contains(event.RawData, text) {
			return true
		}
	}
	return false
}

func agentSessionsToolCallCount(events []runtimeSSEEvent, tool string) int {
	count := 0
	for _, event := range events {
		if event.Name == "tool_call" && agentSessionsEventTool(event) == tool {
			count++
		}
	}
	return count
}

func agentSessionsToolCallsByID(events []runtimeSSEEvent) map[string]string {
	out := map[string]string{}
	for _, event := range events {
		if event.Name != "tool_call" {
			continue
		}
		id := agentSessionsEventID(event)
		if id != "" {
			out[id] = agentSessionsEventTool(event)
		}
	}
	return out
}

func agentSessionsEventID(event runtimeSSEEvent) string {
	return eventString(event.Payload, "id")
}

func agentSessionsEventTool(event runtimeSSEEvent) string {
	return eventString(event.Payload, "tool")
}

func agentSessionsMemoryToolResultText(event runtimeSSEEvent) string {
	if output := agentSessionsToolResultOutput(event); output != "" {
		return output
	}
	raw, err := json.Marshal(event.Payload["result"])
	if err != nil {
		return event.RawData
	}
	return string(raw)
}

func agentSessionsToolResultError(event runtimeSSEEvent) string {
	result, _ := event.Payload["result"].(map[string]any)
	if result == nil {
		return ""
	}
	for _, key := range []string{"isError", "is_error"} {
		if failed, _ := result[key].(bool); failed {
			return agentSessionsMemoryToolResultText(event)
		}
	}
	for _, key := range []string{"error", "safe_error"} {
		if errText := eventString(result, key); errText != "" {
			return errText
		}
	}
	return ""
}

func agentSessionsMemoryEventDebug(events []runtimeSSEEvent) string {
	const max = 8000
	var b strings.Builder
	for _, event := range events {
		if event.Name != "tool_call" && event.Name != "tool_result" && event.Name != "final" && event.Name != "error" && event.Name != "turn_failed" {
			continue
		}
		b.WriteString(event.Name)
		b.WriteString(":")
		b.WriteString(event.RawData)
		b.WriteString("\n")
		if b.Len() > max {
			return b.String()[:max] + "...truncated"
		}
	}
	return b.String()
}
