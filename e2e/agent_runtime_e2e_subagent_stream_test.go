package e2e

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
)

type subagentStreamMonitor struct {
	trace   *agentRuntimeE2ETrace
	ctx     context.Context
	baseURL string
	token   string
	wg      sync.WaitGroup
	mu      sync.Mutex
	streams map[string][]runtimeSSEEvent
	errors  []string
}

func newSubagentStreamMonitor(trace *agentRuntimeE2ETrace, ctx context.Context, baseURL, token string) *subagentStreamMonitor {
	return &subagentStreamMonitor{trace: trace, ctx: ctx, baseURL: baseURL, token: token, streams: map[string][]runtimeSSEEvent{}}
}

func (m *subagentStreamMonitor) observeParentEvent(event runtimeSSEEvent) {
	if event.Name != "subagent_started" {
		return
	}
	agent, _ := event.Payload["agent_name"].(string)
	streamURL, _ := event.Payload["stream_url"].(string)
	if agent == "" || streamURL == "" {
		m.recordError("subagent_started missing agent_name or stream_url: %s", event.RawData)
		return
	}
	m.mu.Lock()
	if _, exists := m.streams[agent]; exists {
		m.mu.Unlock()
		return
	}
	m.streams[agent] = nil
	m.mu.Unlock()

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		events, err := readRuntimeSSEClient(m.ctx, m.trace, "subagent-"+agent, m.baseURL+streamURL, m.token, nil)
		m.mu.Lock()
		defer m.mu.Unlock()
		if err != nil {
			m.errors = append(m.errors, fmt.Sprintf("%s: %v", agent, err))
			return
		}
		m.streams[agent] = events
	}()
}

func (m *subagentStreamMonitor) assert(t *testing.T) {
	t.Helper()
	m.wg.Wait()
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.errors) > 0 {
		t.Fatalf("subagent stream subscriptions failed: %s", strings.Join(m.errors, "; "))
	}
	for _, agent := range []string{"planner", "qa", "reviewer"} {
		events, ok := m.streams[agent]
		if !ok {
			t.Fatalf("subagent stream was not discovered for %s; streams=%v", agent, streamKeys(m.streams))
		}
		if len(events) == 0 {
			t.Fatalf("subagent stream for %s produced no events", agent)
		}
		if countEvents(events, "done") == 0 {
			t.Fatalf("subagent stream for %s did not complete with done; events=%s", agent, summarizeEvents(events))
		}
	}
	m.trace.Logf("assert", "subagent stream subscriptions passed streams=%v", streamKeys(m.streams))
}

func (m *subagentStreamMonitor) recordError(format string, args ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors = append(m.errors, fmt.Sprintf(format, args...))
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

func streamKeys(streams map[string][]runtimeSSEEvent) []string {
	keys := make([]string, 0, len(streams))
	for key := range streams {
		keys = append(keys, key)
	}
	return keys
}
