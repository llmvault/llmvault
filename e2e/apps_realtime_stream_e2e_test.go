package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/sheets"
)

// appsRealtimeFrame is one parsed SSE frame from the app's /api/_live stream.
type appsRealtimeFrame struct {
	Event string
	Data  string
}

// appsRealtimeStream is a raw SSE client connected to {appBase}/api/_live with
// the app session cookie. It reads text/event-stream frames off a real HTTP
// connection into a buffered channel; comment (heartbeat) lines are skipped so
// only real events surface.
type appsRealtimeStream struct {
	frames chan appsRealtimeFrame
	cancel context.CancelFunc
	resp   *http.Response
}

// appsRealtimeDialLive opens an authenticated SSE stream to the deployed app.
func appsRealtimeDialLive(t *testing.T, ctx context.Context, appBase, cookie, label string) *appsRealtimeStream {
	t.Helper()
	reqCtx, cancel := context.WithCancel(ctx)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, appBase+"/api/_live", nil)
	if err != nil {
		cancel()
		t.Fatalf("[%s] build live request: %v", label, err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cookie", "hivy_app_session="+cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("[%s] dial /api/_live: %v", label, err)
	}
	if resp.StatusCode != http.StatusOK {
		cancel()
		resp.Body.Close()
		t.Fatalf("[%s] /api/_live status=%d want 200", label, resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		cancel()
		resp.Body.Close()
		t.Fatalf("[%s] /api/_live content-type=%q want text/event-stream", label, ct)
	}
	s := &appsRealtimeStream{frames: make(chan appsRealtimeFrame, 256), cancel: cancel, resp: resp}
	go s.read()
	t.Cleanup(func() { cancel(); resp.Body.Close() })
	return s
}

func (s *appsRealtimeStream) read() {
	defer close(s.frames)
	defer s.resp.Body.Close()
	reader := bufio.NewReader(s.resp.Body)
	var event string
	var data []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if len(data) > 0 {
				s.frames <- appsRealtimeFrame{Event: event, Data: strings.Join(data, "\n")}
			}
			event = ""
			data = data[:0]
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue // SSE comment / heartbeat
		}
		field, value := splitSSE(line)
		switch field {
		case "event":
			event = value
		case "data":
			data = append(data, value)
		}
	}
}

func splitSSE(line string) (string, string) {
	idx := strings.IndexByte(line, ':')
	if idx < 0 {
		return line, ""
	}
	value := line[idx+1:]
	return line[:idx], strings.TrimPrefix(value, " ")
}

// next returns the next frame or fails after timeout.
func (s *appsRealtimeStream) next(t *testing.T, timeout time.Duration, what string) appsRealtimeFrame {
	t.Helper()
	select {
	case f, ok := <-s.frames:
		if !ok {
			t.Fatalf("live stream closed while waiting for %s", what)
		}
		return f
	case <-time.After(timeout):
		t.Fatalf("timed out after %s waiting for %s", timeout, what)
		return appsRealtimeFrame{}
	}
}

// waitRowsChanged reads frames until it finds a rows_changed event whose parsed
// payload satisfies match, returning the decoded event and observed latency. If
// forbidSheetID is non-empty and ANY rows_changed for that sheet arrives first,
// it fails (cross-sheet isolation). Fails on timeout.
func (s *appsRealtimeStream) waitRowsChanged(t *testing.T, timeout time.Duration, what, forbidSheetID string, match func(sheets.Event) bool) sheets.Event {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf("timed out after %s waiting for rows_changed: %s", timeout, what)
		}
		select {
		case f, ok := <-s.frames:
			if !ok {
				t.Fatalf("live stream closed while waiting for %s", what)
			}
			if f.Event != sheets.EventRowsChanged {
				continue
			}
			ev := appsRealtimeDecode(t, f)
			if forbidSheetID != "" && ev.SheetID == forbidSheetID {
				t.Fatalf("CROSS-SHEET LEAK: app stream delivered a rows_changed for unbound sheet %s: %+v", forbidSheetID, ev)
			}
			if match(ev) {
				return ev
			}
		case <-time.After(remaining):
			t.Fatalf("timed out after %s waiting for rows_changed: %s", timeout, what)
		}
	}
}

// tryRowsChanged waits up to timeout for a rows_changed matching match, without
// failing on timeout. Used by the warm-up probe. Returns (event, found).
func (s *appsRealtimeStream) tryRowsChanged(timeout time.Duration, match func(sheets.Event) bool) (sheets.Event, bool) {
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return sheets.Event{}, false
		}
		select {
		case f, ok := <-s.frames:
			if !ok {
				return sheets.Event{}, false
			}
			if f.Event != sheets.EventRowsChanged {
				continue
			}
			var ev sheets.Event
			if json.Unmarshal([]byte(f.Data), &ev) != nil {
				continue
			}
			if match(ev) {
				return ev, true
			}
		case <-time.After(remaining):
			return sheets.Event{}, false
		}
	}
}

func appsRealtimeDecode(t *testing.T, f appsRealtimeFrame) sheets.Event {
	t.Helper()
	var ev sheets.Event
	if err := json.Unmarshal([]byte(f.Data), &ev); err != nil {
		t.Fatalf("decode rows_changed payload %q: %v", f.Data, err)
	}
	return ev
}

func eventHasRow(ev sheets.Event, rowID string) bool {
	for _, id := range ev.RowIDs {
		if id == rowID {
			return true
		}
	}
	return false
}

func eventSnapshot(ev sheets.Event, rowID string) (sheets.EventRow, bool) {
	for _, r := range ev.Rows {
		if r.ID == rowID {
			return r, true
		}
	}
	return sheets.EventRow{}, false
}
