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

func agentSessionsDirectStreamURL(access agentSessionsSandboxAccess, sessionID string) string {
	return strings.TrimRight(access.SandboxBaseURL, "/") + "/sessions/" + sessionID + "/stream"
}

func agentSessionsReadDirectStream(t *testing.T, ctx context.Context, directURL, token string) []runtimeSSEEvent {
	t.Helper()
	stream := agentSessionsOpenDirectStream(t, ctx, directURL, token, true)
	return stream.waitDone(t, ctx)
}

type agentSessionsLiveDirectStream struct {
	events chan runtimeSSEEvent
	done   chan []runtimeSSEEvent
	errs   chan error
}

func agentSessionsStartDirectStream(t *testing.T, ctx context.Context, directURL, token string) *agentSessionsLiveDirectStream {
	t.Helper()
	return agentSessionsOpenDirectStream(t, ctx, directURL, token, false)
}

func agentSessionsOpenDirectStream(t *testing.T, ctx context.Context, directURL, token string, stopOnDone bool) *agentSessionsLiveDirectStream {
	t.Helper()
	parsed, err := url.Parse(directURL)
	if err != nil {
		t.Fatalf("parse direct stream url: %v", err)
	}

	resp := openDirectStreamWithRetry(t, ctx, parsed.String(), token) //nolint:bodyclose // Closed by the stream reader goroutine.

	stream := &agentSessionsLiveDirectStream{
		events: make(chan runtimeSSEEvent, 256),
		done:   make(chan []runtimeSSEEvent, 1),
		errs:   make(chan error, 1),
	}
	go func() {
		defer resp.Body.Close()
		events, err := readAgentSessionsDirectStreamEvents(resp.Body, stream.events, stopOnDone)
		if err != nil {
			stream.errs <- err
			return
		}
		stream.done <- events
	}()
	return stream
}

func openDirectStreamWithRetry(t *testing.T, ctx context.Context, directURL, token string) *http.Response {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	var lastStatus int
	var lastBody []byte
	var lastErr error
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, directURL, nil)
		if err != nil {
			t.Fatalf("new direct stream request: %v", err)
		}
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Authorization", "Bearer "+token)
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
				t.Fatalf("open direct sandbox stream: %v", lastErr)
			}
			t.Fatalf("direct sandbox stream status=%d body=%s", lastStatus, lastBody)
		}
		select {
		case <-ctx.Done():
			if lastErr != nil {
				t.Fatalf("direct sandbox stream wait canceled: %v", lastErr)
			}
			t.Fatalf("direct sandbox stream wait canceled after status=%d body=%s: %v", lastStatus, lastBody, ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
}

func (s *agentSessionsLiveDirectStream) waitForEvent(t *testing.T, ctx context.Context, timeout time.Duration, want func(runtimeSSEEvent) bool) runtimeSSEEvent {
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
			t.Fatalf("direct stream completed before expected event; events=%s", summarizeRuntimeSSEEvents(events))
		case err := <-s.errs:
			t.Fatalf("direct stream failed before expected event: %v", err)
		case <-ctx.Done():
			t.Fatalf("direct stream context ended before expected event: %v", ctx.Err())
		case <-timer.C:
			t.Fatalf("timed out waiting for direct stream event")
		}
	}
}

func (s *agentSessionsLiveDirectStream) waitDone(t *testing.T, ctx context.Context) []runtimeSSEEvent {
	t.Helper()
	for {
		select {
		case events := <-s.done:
			return events
		case err := <-s.errs:
			t.Fatalf("read direct sandbox stream: %v", err)
		case <-ctx.Done():
			t.Fatalf("direct stream context ended before done: %v", ctx.Err())
		}
	}
}

func readAgentSessionsDirectStreamEvents(body io.Reader, live chan<- runtimeSSEEvent, stopOnDone bool) ([]runtimeSSEEvent, error) {
	scanner := bufio.NewScanner(body)
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
		events = append(events, runtimeSSEEvent{Name: name, Payload: payload, RawData: raw})
		if live != nil {
			select {
			case live <- events[len(events)-1]:
			default:
			}
		}
		done := stopOnDone && name == "done"
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

func assertAgentSessionsDirectStream(t *testing.T, events []runtimeSSEEvent, marker string) {
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
		t.Fatalf("direct stream missing marker %s events=%s final=%s", marker, strings.Join(names, ","), finalText.String())
	}
	t.Logf("direct sandbox stream events=%s", fmt.Sprint(names))
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
