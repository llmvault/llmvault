package handler_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/sheets"
)

// TestAppsLiveRejectsBadBearer: a wrong app secret never opens the stream.
func TestAppsLiveRejectsBadBearer(t *testing.T) {
	h := newAppsHarness(t)
	if rr := h.do(t, http.MethodGet, h.appPath("/live"), "wrong-secret", "", nil); rr.Code != http.StatusUnauthorized {
		t.Fatalf("bad bearer status = %d body=%s", rr.Code, rr.Body.String())
	}
	if rr := h.do(t, http.MethodGet, h.appPath("/live"), "", "", nil); rr.Code != http.StatusUnauthorized {
		t.Fatalf("missing bearer status = %d body=%s", rr.Code, rr.Body.String())
	}
}

// TestAppsLiveStreamsBoundSheetEvents drives the full SSE relay against real
// Redis: it asserts the event framing, a heartbeat, and that events from a
// second sheet in the same org NEVER reach the app's stream.
func TestAppsLiveStreamsBoundSheetEvents(t *testing.T) {
	h := newAppsHarness(t)
	rc := connectTestRedis(t)
	if err := rc.Ping(context.Background()).Err(); err != nil {
		t.Skipf("Redis is not available: %v", err)
	}
	restore := handler.SetAppLiveHeartbeatForTest(300 * time.Millisecond)
	defer restore()
	h.handler.WithRedis(rc)
	h.svc.WithPublisher(sheets.NewRedisEventPublisher(rc))

	server := httptest.NewServer(h.router)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+h.appPath("/live"), nil)
	if err != nil {
		t.Fatalf("build live request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+h.secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open live stream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("live status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type = %q", ct)
	}

	reader := bufio.NewReader(resp.Body)
	// Stream opens with the ": ok" comment.
	open, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read stream open: %v", err)
	}
	if !strings.HasPrefix(open, ": ok") {
		t.Fatalf("stream open line = %q", open)
	}

	// Publish once the subscriber is attached: a foreign sheet's event (same
	// org, same channel) that must never arrive, then the bound sheet's event.
	go func() {
		time.Sleep(300 * time.Millisecond)
		_, _ = h.svc.InsertRows(context.Background(), h.org.ID, h.unboundPage.Page.ID,
			[]sheets.RowInsert{{Data: map[string]any{h.unboundPage.Fields[0].ID: "SECRET"}}},
			sheets.MaxRowsPerWriteREST, sheets.Actor{UserID: &h.user.ID})
		time.Sleep(200 * time.Millisecond)
		_, _ = h.svc.InsertRows(sheets.WithMutationID(context.Background(), "live-1"), h.org.ID, h.page.Page.ID,
			[]sheets.RowInsert{{Data: map[string]any{h.fields["Title"]: "Streamed"}}},
			sheets.MaxRowsPerWriteREST, sheets.Actor{UserID: &h.user.ID})
	}()

	var (
		sawEventName bool
		sawHeartbeat bool
		sawSnapshot  bool
	)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && !(sawEventName && sawHeartbeat && sawSnapshot) {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read stream: %v", err)
		}
		line = strings.TrimRight(line, "\n")
		switch {
		case line == ": ping":
			sawHeartbeat = true
		case strings.HasPrefix(line, "event: "):
			if line != "event: rows_changed" {
				t.Fatalf("unexpected event name: %q", line)
			}
			sawEventName = true
		case strings.HasPrefix(line, "data: "):
			payload := strings.TrimPrefix(line, "data: ")
			if strings.Contains(payload, h.unboundPage.Page.ID.String()) {
				t.Fatalf("foreign sheet event leaked into the app stream: %s", payload)
			}
			var ev sheets.Event
			if err := json.Unmarshal([]byte(payload), &ev); err != nil {
				t.Fatalf("decode event payload %q: %v", payload, err)
			}
			if ev.Type != sheets.EventRowsChanged || ev.PageID != h.page.Page.ID.String() {
				t.Fatalf("event routing = %+v", ev)
			}
			if len(ev.Rows) == 1 && ev.Rows[0].Data[h.fields["Title"]] == "Streamed" {
				sawSnapshot = true
			}
		}
	}
	if !sawEventName {
		t.Fatal("never saw an event: rows_changed frame")
	}
	if !sawSnapshot {
		t.Fatal("event frame carried no row snapshot")
	}
	if !sawHeartbeat {
		t.Fatal("never saw a heartbeat comment")
	}
}
