package e2e

import (
	"fmt"
	"strings"
	"testing"
)

func assertRuntimeSharedSubagentStream(t *testing.T, trace *agentRuntimeE2ETrace, label string, events []runtimeSSEEvent) {
	t.Helper()
	started := map[string]int{}
	startOrder := map[string]int{}
	completed := map[string]int{}
	scopedFinals := map[string]int{}
	scopedEvents := map[string]int{}
	doneCount := 0
	firstCompletedIndex := -1

	for index, event := range events {
		if event.Name == "done" {
			doneCount++
		}
		scope, _ := event.Payload["scope"].(string)
		subagent := payloadMap(event.Payload["subagent"])
		agent, _ := subagent["agent_name"].(string)
		if event.Name == "subagent_started" {
			if _, exists := event.Payload["stream_url"]; exists {
				t.Fatalf("%s subagent_started exposed child stream_url: %s", label, event.RawData)
			}
			assertSubagentMarker(t, label, event, agent)
			started[agent]++
			if _, exists := startOrder[agent]; !exists {
				startOrder[agent] = index
			}
		}
		if event.Name == "subagent_completed" {
			assertSubagentMarker(t, label, event, agent)
			completed[agent]++
			if firstCompletedIndex == -1 {
				firstCompletedIndex = index
			}
		}
		if scope == "subagent" {
			assertSubagentMarker(t, label, event, agent)
			if event.Name == "done" {
				t.Fatalf("%s subagent emitted terminal done on shared stream: %s", label, event.RawData)
			}
			scopedEvents[agent]++
			if event.Name == "final" {
				scopedFinals[agent]++
			}
		}
	}

	if doneCount != 1 {
		t.Fatalf("%s shared stream done count=%d want=1 events=%s", label, doneCount, summarizeEvents(events))
	}
	for _, agent := range agentRuntimeE2EHakareeSubagents {
		if started[agent] == 0 {
			t.Fatalf("%s missing subagent_started for %s; started=%v events=%s", label, agent, started, summarizeEvents(events))
		}
		if completed[agent] == 0 {
			t.Fatalf("%s missing subagent_completed for %s; completed=%v events=%s", label, agent, completed, summarizeEvents(events))
		}
		if scopedEvents[agent] == 0 {
			t.Fatalf("%s missing scoped live events for %s; scoped=%v events=%s", label, agent, scopedEvents, summarizeEvents(events))
		}
		if scopedFinals[agent] == 0 {
			t.Fatalf("%s missing scoped final event for %s; finals=%v events=%s", label, agent, scopedFinals, summarizeEvents(events))
		}
	}
	assertSubagentsStartedBeforeFirstCompletion(t, label, startOrder, firstCompletedIndex, events)
	trace.Logf("assert", "%s shared subagent stream passed started=%v completed=%v scoped=%v finals=%v", label, started, completed, scopedEvents, scopedFinals)
}

func assertSubagentsStartedBeforeFirstCompletion(t *testing.T, label string, startOrder map[string]int, firstCompletedIndex int, events []runtimeSSEEvent) {
	t.Helper()
	if firstCompletedIndex < 0 {
		t.Fatalf("%s missing first subagent completion; events=%s", label, summarizeEvents(events))
	}
	for _, agent := range agentRuntimeE2EHakareeSubagents {
		startIndex, ok := startOrder[agent]
		if !ok {
			t.Fatalf("%s missing start order for %s; starts=%v events=%s", label, agent, startOrder, summarizeEvents(events))
		}
		if startIndex > firstCompletedIndex {
			t.Fatalf("%s subagent %s started after first completion; starts=%v first_completed_index=%d events=%s", label, agent, startOrder, firstCompletedIndex, summarizeEvents(events))
		}
	}
}

func assertSubagentMarker(t *testing.T, label string, event runtimeSSEEvent, agent string) {
	t.Helper()
	if event.Payload["scope"] != "subagent" {
		t.Fatalf("%s %s missing subagent scope marker: %s", label, event.Name, event.RawData)
	}
	subagent := payloadMap(event.Payload["subagent"])
	if agent == "" {
		t.Fatalf("%s %s missing subagent.agent_name: %s", label, event.Name, event.RawData)
	}
	for _, key := range []string{"job_id", "parent_session_id", "child_session_id"} {
		if value, _ := subagent[key].(string); strings.TrimSpace(value) == "" {
			t.Fatalf("%s %s missing subagent.%s: %s", label, event.Name, key, event.RawData)
		}
	}
	child, _ := subagent["child_session_id"].(string)
	if !strings.HasPrefix(child, "subagent-subagent-task-") {
		t.Fatalf("%s %s child_session_id=%q does not look like a subagent task session", label, event.Name, child)
	}
}

func payloadMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	if mapped, ok := value.(map[string]any); ok {
		return mapped
	}
	return map[string]any{"_invalid": fmt.Sprint(value)}
}

func countEvents(events []runtimeSSEEvent, name string) int {
	count := 0
	for _, event := range events {
		if event.Name == name {
			count++
		}
	}
	return count
}
