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

func agentSessionsReadGoStream(t *testing.T, ctx context.Context, apiBase, token, orgID, sessionID string) []runtimeSSEEvent {
	t.Helper()
	stream := agentSessionsOpenGoStream(t, ctx, apiBase, token, orgID, sessionID, true)
	return stream.waitDone(t, ctx)
}

type agentSessionsLiveGoStream struct {
	events chan runtimeSSEEvent
	done   chan []runtimeSSEEvent
	errs   chan error
}

func agentSessionsStartGoStream(t *testing.T, ctx context.Context, apiBase, token, orgID, sessionID string) *agentSessionsLiveGoStream {
	t.Helper()
	return agentSessionsOpenGoStream(t, ctx, apiBase, token, orgID, sessionID, false)
}

func agentSessionsOpenGoStream(t *testing.T, ctx context.Context, apiBase, token, orgID, sessionID string, stopOnTerminal bool) *agentSessionsLiveGoStream {
	t.Helper()
	resp := openGoSessionStreamWithRetry(t, ctx, apiBase, token, orgID, sessionID) //nolint:bodyclose // Closed by the stream reader goroutine.

	stream := &agentSessionsLiveGoStream{
		events: make(chan runtimeSSEEvent, 256),
		done:   make(chan []runtimeSSEEvent, 1),
		errs:   make(chan error, 1),
	}
	go func() {
		defer resp.Body.Close()
		events, err := readAgentSessionsGoStreamEvents(resp.Body, stream.events, stopOnTerminal)
		if err != nil {
			stream.errs <- err
			return
		}
		stream.done <- events
	}()
	return stream
}

func openGoSessionStreamWithRetry(t *testing.T, ctx context.Context, apiBase, token, orgID, sessionID string) *http.Response {
	t.Helper()
	endpoint := strings.TrimRight(apiBase, "/") + "/v1/sessions/" + sessionID + "/stream?after_seq=0"
	deadline := time.Now().Add(2 * time.Minute)
	var lastStatus int
	var lastBody []byte
	var lastErr error
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			t.Fatalf("new go session stream request: %v", err)
		}
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Org-ID", orgID)
		resp, err := http.DefaultClient.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			return resp
		}
		if err != nil {
			lastErr = err
		} else {
			lastStatus = resp.StatusCode
			lastBody, _ = io.ReadAll(resp.Body)
			resp.Body.Close()
		}
		if time.Now().After(deadline) {
			if lastErr != nil {
				t.Fatalf("open go session stream: %v", lastErr)
			}
			t.Fatalf("go session stream status=%d body=%s", lastStatus, lastBody)
		}
		select {
		case <-ctx.Done():
			if lastErr != nil {
				t.Fatalf("go session stream wait canceled: %v", lastErr)
			}
			t.Fatalf("go session stream wait canceled after status=%d body=%s: %v", lastStatus, lastBody, ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
}

func (s *agentSessionsLiveGoStream) waitForEvent(t *testing.T, ctx context.Context, timeout time.Duration, want func(runtimeSSEEvent) bool) runtimeSSEEvent {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case event := <-s.events:
			if want(event) {
				return event
			}
		case events := <-s.done:
			t.Fatalf("go session stream completed before expected event; events=%s", summarizeRuntimeSSEEvents(events))
		case err := <-s.errs:
			t.Fatalf("go session stream failed before expected event: %v", err)
		case <-ctx.Done():
			t.Fatalf("go session stream context ended before expected event: %v", ctx.Err())
		case <-timer.C:
			t.Fatalf("timed out waiting for go session stream event")
		}
	}
}

func (s *agentSessionsLiveGoStream) waitDone(t *testing.T, ctx context.Context) []runtimeSSEEvent {
	t.Helper()
	for {
		select {
		case events := <-s.done:
			return events
		case err := <-s.errs:
			t.Fatalf("read go session stream: %v", err)
		case <-ctx.Done():
			t.Fatalf("go session stream context ended before done: %v", ctx.Err())
		}
	}
}

func readAgentSessionsGoStreamEvents(body io.Reader, live chan<- runtimeSSEEvent, stopOnTerminal bool) ([]runtimeSSEEvent, error) {
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
		event := agentSessionsRuntimeEventFromGoSSE(name, raw)
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

func agentSessionsRuntimeEventFromGoSSE(name, raw string) runtimeSSEEvent {
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

func assertAgentSessionsGoStream(t *testing.T, events []runtimeSSEEvent, marker string) {
	t.Helper()
	if len(events) == 0 {
		t.Fatalf("go session stream returned no events")
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
		t.Fatalf("go session stream missing marker %s events=%s final=%s", marker, strings.Join(names, ","), finalText.String())
	}
	t.Logf("go session stream events=%s", fmt.Sprint(names))
}

func summarizeRuntimeSSEEvents(events []runtimeSSEEvent) string {
	names := make([]string, len(events))
	for i, event := range events {
		names[i] = event.Name
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
