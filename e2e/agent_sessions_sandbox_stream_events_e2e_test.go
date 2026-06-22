package e2e

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
)

func readAgentSessionsSandboxStreamEvents(body io.Reader, live chan<- runtimeSSEEvent, stopOnTerminal bool) ([]runtimeSSEEvent, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 1024), 4*1024*1024)
	var events []runtimeSSEEvent
	var name string
	var dataLines []string
	flush := func() bool {
		if name == "" && len(dataLines) == 0 {
			return false
		}
		raw := strings.Join(dataLines, "\n")
		event := agentSessionsRuntimeEventFromSandboxSSE(name, raw)
		events = append(events, event)
		if live != nil {
			select {
			case live <- event:
			default:
			}
		}
		done := stopOnTerminal && (event.Name == "turn_completed" || event.Name == "turn_failed" || event.Name == "done")
		name = ""
		dataLines = nil
		return done
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if flush() {
				return events, nil
			}
			continue
		}
		if strings.HasPrefix(line, "event:") {
			name = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	flush()
	return events, nil
}

func agentSessionsRuntimeEventFromSandboxSSE(name, raw string) runtimeSSEEvent {
	payload := map[string]any{}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return runtimeSSEEvent{Name: name, Payload: map[string]any{"_raw": raw}, RawData: raw}
		}
	}
	if name != "session.preview" && name != "session.event" {
		if name == "session.control" {
			if controlType, _ := payload["type"].(string); controlType != "" {
				name = controlType
			}
		}
		return runtimeSSEEvent{Name: name, Payload: payload, RawData: raw}
	}
	eventType, _ := payload["event_type"].(string)
	if eventType == "" {
		eventType = name
	}
	eventPayload, _ := payload["payload"].(map[string]any)
	if eventPayload == nil {
		eventPayload = map[string]any{}
	}
	for _, key := range []string{"session_id", "event_id", "turn_id", "span_id", "durability", "runtime_seq", "sequence_number", "event_at", "occurred_at"} {
		if value, ok := payload[key]; ok {
			eventPayload[key] = value
		}
	}
	return runtimeSSEEvent{Name: eventType, Payload: eventPayload, RawData: raw}
}

func assertAgentSessionsSandboxStream(t *testing.T, events []runtimeSSEEvent, marker string) {
	t.Helper()
	if len(events) == 0 {
		t.Fatalf("direct sandbox stream returned no events")
	}
	var names []string
	var finalText strings.Builder
	for _, event := range events {
		names = append(names, event.Name)
		if text, _ := event.Payload["text"].(string); event.Name == "final" && text != "" {
			finalText.WriteString(text)
		}
		if strings.Contains(event.RawData, marker) {
			finalText.WriteString("\n")
			finalText.WriteString(event.RawData)
		}
	}
	if !strings.Contains(finalText.String(), marker) {
		t.Fatalf("direct sandbox stream missing marker %s events=%s final=%s", marker, strings.Join(names, ","), finalText.String())
	}
	t.Logf("direct sandbox stream events=%s", fmt.Sprint(names))
}

func summarizeRuntimeSSEEvents(events []runtimeSSEEvent) string {
	if len(events) == 0 {
		return ""
	}
	const maxEvents = 40
	start := 0
	if len(events) > maxEvents {
		start = len(events) - maxEvents
	}
	names := make([]string, 0, len(events)-start+1)
	if start > 0 {
		names = append(names, fmt.Sprintf("...%d earlier", start))
	}
	for _, event := range events[start:] {
		item := event.Name
		if text, _ := event.Payload["text"].(string); text != "" {
			item += ":" + text
		} else if output := agentSessionsToolResultOutput(event); output != "" {
			item += ":" + output
		}
		names = append(names, item)
	}
	return strings.Join(names, ",")
}

func agentSessionsToolResultOutput(event runtimeSSEEvent) string {
	if event.Name != "tool_result" {
		return ""
	}
	result, _ := event.Payload["result"].(map[string]any)
	output, _ := result["output"].(string)
	return output
}
