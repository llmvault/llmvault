package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func agentSessionsReadSandboxStream(t *testing.T, ctx context.Context, apiBase, token, orgID, sessionID string) []runtimeSSEEvent {
	t.Helper()
	stream := agentSessionsOpenSandboxStream(t, ctx, apiBase, token, orgID, sessionID, true)
	return stream.waitDone(t, ctx)
}

type agentSessionsLiveSandboxStream struct {
	events chan runtimeSSEEvent
	done   chan []runtimeSSEEvent
	errs   chan error
}

func agentSessionsStartSandboxStream(t *testing.T, ctx context.Context, apiBase, token, orgID, sessionID string) *agentSessionsLiveSandboxStream {
	t.Helper()
	return agentSessionsOpenSandboxStream(t, ctx, apiBase, token, orgID, sessionID, false)
}

func agentSessionsStartSandboxStreamWithAccess(t *testing.T, ctx context.Context, sessionID string, access agentSessionsSandboxAccess) *agentSessionsLiveSandboxStream {
	t.Helper()
	return agentSessionsOpenSandboxStreamWithAccess(t, ctx, sessionID, access, false)
}

func agentSessionsOpenSandboxStream(t *testing.T, ctx context.Context, apiBase, token, orgID, sessionID string, stopOnTerminal bool) *agentSessionsLiveSandboxStream {
	t.Helper()
	access := fetchAgentSessionsSandboxAccess(t, ctx, apiBase, token, orgID, sessionID)
	return agentSessionsOpenSandboxStreamWithAccess(t, ctx, sessionID, access, stopOnTerminal)
}

func agentSessionsOpenSandboxStreamWithAccess(t *testing.T, ctx context.Context, sessionID string, access agentSessionsSandboxAccess, stopOnTerminal bool) *agentSessionsLiveSandboxStream {
	t.Helper()
	requireAgentSessionsSandboxStreamAccess(t, access, sessionID)
	resp := openSandboxSessionStreamWithRetry(t, ctx, sessionID, access) //nolint:bodyclose // Closed by the stream reader goroutine.

	stream := &agentSessionsLiveSandboxStream{
		events: make(chan runtimeSSEEvent, 256),
		done:   make(chan []runtimeSSEEvent, 1),
		errs:   make(chan error, 1),
	}
	go func() {
		defer resp.Body.Close()
		events, err := readAgentSessionsSandboxStreamEvents(resp.Body, stream.events, stopOnTerminal)
		if err != nil {
			stream.errs <- err
			return
		}
		stream.done <- events
	}()
	return stream
}

func openSandboxSessionStreamWithRetry(t *testing.T, ctx context.Context, sessionID string, access agentSessionsSandboxAccess) *http.Response {
	t.Helper()
	resp, err := openSandboxSessionStreamClient(ctx, sessionID, access)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func openSandboxSessionStreamClient(ctx context.Context, sessionID string, access agentSessionsSandboxAccess) (*http.Response, error) {
	endpoint, err := agentSessionsSandboxStreamURL(access, sessionID, url.Values{"after_seq": []string{"0"}})
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(2 * time.Minute)
	var lastStatus int
	var lastBody []byte
	var lastErr error
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("new direct sandbox stream request: %w", err)
		}
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Authorization", "Bearer "+access.Token)
		resp, err := http.DefaultClient.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			return resp, nil
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
				return nil, fmt.Errorf("open direct sandbox stream: %w", lastErr)
			}
			return nil, fmt.Errorf("direct sandbox stream status=%d body=%s", lastStatus, lastBody)
		}
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return nil, fmt.Errorf("direct sandbox stream wait canceled: %w", lastErr)
			}
			return nil, fmt.Errorf("direct sandbox stream wait canceled after status=%d body=%s: %w", lastStatus, lastBody, ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
}

func (s *agentSessionsLiveSandboxStream) waitForEvent(t *testing.T, ctx context.Context, timeout time.Duration, want func(runtimeSSEEvent) bool) runtimeSSEEvent {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	observed := make([]runtimeSSEEvent, 0, 32)
	for {
		select {
		case event := <-s.events:
			observed = append(observed, event)
			if want(event) {
				return event
			}
		case events := <-s.done:
			t.Fatalf("direct sandbox stream completed before expected event; events=%s", summarizeRuntimeSSEEvents(events))
		case err := <-s.errs:
			t.Fatalf("direct sandbox stream failed before expected event: %v", err)
		case <-ctx.Done():
			t.Fatalf("direct sandbox stream context ended before expected event: %v", ctx.Err())
		case <-timer.C:
			t.Fatalf("timed out waiting for direct sandbox stream event; observed=%s", summarizeRuntimeSSEEvents(observed))
		}
	}
}

func (s *agentSessionsLiveSandboxStream) waitDone(t *testing.T, ctx context.Context) []runtimeSSEEvent {
	t.Helper()
	for {
		select {
		case events := <-s.done:
			return events
		case err := <-s.errs:
			t.Fatalf("read direct sandbox stream: %v", err)
		case <-ctx.Done():
			t.Fatalf("direct sandbox stream context ended before done: %v", ctx.Err())
		}
	}
}

func (s *agentSessionsLiveSandboxStream) assertNoBufferedEvent(t *testing.T, want func(runtimeSSEEvent) bool) {
	t.Helper()
	for {
		select {
		case event := <-s.events:
			if want(event) {
				t.Fatalf("direct sandbox stream had unexpected buffered event: %s", event.RawData)
			}
		default:
			return
		}
	}
}

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
