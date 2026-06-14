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

type agentSessionsStreamAccess struct {
	SessionID      string `json:"session_id"`
	SessionEventID string `json:"session_event_id"`
	SequenceNumber int64  `json:"sequence_number"`
	StreamID       string `json:"stream_id"`
	StreamURL      string `json:"stream_url"`
	DirectURL      string `json:"direct_url"`
	StreamToken    string `json:"stream_token"`
	TraceID        string `json:"trace_id"`
	TurnID         string `json:"turn_id"`
}

func waitForAgentSessionsStreamAccess(t *testing.T, ctx context.Context, baseURL, token, orgID, sessionID, eventID string) agentSessionsStreamAccess {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	var lastStatus int
	var lastBody string
	for time.Now().Before(deadline) {
		out, status, body := agentSessionsTryStreamAccess(t, ctx, baseURL, token, orgID, sessionID, eventID)
		if status == http.StatusOK {
			if out.DirectURL == "" || out.StreamToken == "" || out.StreamID == "" {
				t.Fatalf("stream access missing direct fields: %+v", out)
			}
			return out
		}
		lastStatus = status
		lastBody = body
		if status != http.StatusNotFound {
			t.Fatalf("stream access status=%d body=%s", status, body)
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for stream access status=%d body=%s", lastStatus, lastBody)
	return agentSessionsStreamAccess{}
}

func agentSessionsStreamAccessStatus(t *testing.T, ctx context.Context, baseURL, token, orgID, sessionID, eventID string, want int) {
	t.Helper()
	_, status, body := agentSessionsTryStreamAccess(t, ctx, baseURL, token, orgID, sessionID, eventID)
	if status != want {
		t.Fatalf("stream access status=%d want=%d body=%s", status, want, body)
	}
}

func agentSessionsTryStreamAccess(t *testing.T, ctx context.Context, baseURL, token, orgID, sessionID, eventID string) (agentSessionsStreamAccess, int, string) {
	t.Helper()
	endpoint := baseURL + "/v1/sessions/" + sessionID + "/stream-access?event_id=" + url.QueryEscape(eventID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("new stream access request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-ID", orgID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream access request failed: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out agentSessionsStreamAccess
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("decode stream access: %v\n%s", err, raw)
		}
	}
	return out, resp.StatusCode, string(raw)
}

func agentSessionsReadDirectStream(t *testing.T, ctx context.Context, directURL, streamToken string) []runtimeSSEEvent {
	t.Helper()
	parsed, err := url.Parse(directURL)
	if err != nil {
		t.Fatalf("parse direct stream url: %v", err)
	}
	query := parsed.Query()
	query.Set("stream_token", streamToken)
	parsed.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		t.Fatalf("new direct stream request: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open direct sandbox stream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("direct sandbox stream status=%d body=%s", resp.StatusCode, raw)
	}

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
		events = append(events, runtimeSSEEvent{Name: name, Payload: payload, RawData: raw})
		done := name == "done"
		name = ""
		dataLines = nil
		return done
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if flush() {
				return events
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
		t.Fatalf("read direct sandbox stream: %v", err)
	}
	flush()
	return events
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
