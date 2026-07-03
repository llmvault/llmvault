package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func writeLogFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func getLogs(t *testing.T, ts string, query string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, ts+"/logs"+query, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testSecret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode logs response: %v", err)
	}
	return resp.StatusCode, body
}

func TestLogsLinesCappedAt2000AfterGrep(t *testing.T) {
	srv, ts := newTestServer(t)
	var b strings.Builder
	for i := 0; i < 2500; i++ {
		fmt.Fprintf(&b, "match line %04d\n", i)
	}
	b.WriteString("odd one out\n")
	writeLogFile(t, srv.cfg.appLogPath(), b.String())

	status, body := getLogs(t, ts.URL, "?lines=5000&grep=match")
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	lines := body["lines"].([]any)
	if len(lines) != maxLogLines {
		t.Fatalf("got %d lines, want hard cap %d", len(lines), maxLogLines)
	}
	// Grep applied before the cap: newest matching lines survive, the
	// non-matching trailer does not.
	if last := lines[len(lines)-1].(string); last != "match line 2499" {
		t.Errorf("last line = %q, want newest matching line", last)
	}
	if first := lines[0].(string); first != "match line 0500" {
		t.Errorf("first line = %q, want match line 0500", first)
	}
}

func TestLogsGrepIsRegex(t *testing.T) {
	srv, ts := newTestServer(t)
	writeLogFile(t, srv.cfg.appLogPath(), "GET /api/items 200\nPOST /api/items 500\nGET /healthz 200\n")

	status, body := getLogs(t, ts.URL, "?grep="+`%5E%28GET%7CPOST%29.%2A500%24`) // ^(GET|POST).*500$
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	lines := body["lines"].([]any)
	if len(lines) != 1 || lines[0].(string) != "POST /api/items 500" {
		t.Errorf("lines = %v, want the single 500 line", lines)
	}

	status, _ = getLogs(t, ts.URL, "?grep=%5B") // invalid regex "["
	if status != http.StatusBadRequest {
		t.Errorf("invalid regex status = %d, want 400", status)
	}
}

func TestLogsSinceFiltersTimestampedLines(t *testing.T) {
	srv, ts := newTestServer(t)
	old := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	fresh := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	content := old.Format(time.RFC3339) + " old line\n" +
		fresh.Format(time.RFC3339) + " fresh line\n" +
		"no timestamp line\n" +
		`{"time":"` + old.Format(time.RFC3339) + `","msg":"old json"}` + "\n" +
		`{"time":"` + fresh.Format(time.RFC3339) + `","msg":"fresh json"}` + "\n"
	writeLogFile(t, srv.cfg.appLogPath(), content)

	status, body := getLogs(t, ts.URL, "?since=2026-07-02T00:00:00Z")
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	lines := body["lines"].([]any)
	joined := fmt.Sprint(lines)
	if len(lines) != 3 {
		t.Fatalf("got %d lines (%v), want 3", len(lines), joined)
	}
	if strings.Contains(joined, "old line") || strings.Contains(joined, "old json") {
		t.Errorf("since filter kept old lines: %v", joined)
	}
	if !strings.Contains(joined, "no timestamp line") {
		t.Errorf("lines without timestamps must be kept: %v", joined)
	}
}

func TestLogsReadAcrossRotatedFiles(t *testing.T) {
	srv, ts := newTestServer(t)
	writeLogFile(t, srv.cfg.appLogPath()+".2", "oldest a\noldest b\n")
	writeLogFile(t, srv.cfg.appLogPath()+".1", "middle a\nmiddle b\n")
	writeLogFile(t, srv.cfg.appLogPath(), "newest a\nnewest b\n")

	status, body := getLogs(t, ts.URL, "?lines=4")
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	var got []string
	for _, l := range body["lines"].([]any) {
		got = append(got, l.(string))
	}
	want := []string{"middle a", "middle b", "newest a", "newest b"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("lines = %v, want %v (chronological across rotations)", got, want)
	}
}

func TestLogsAppdStream(t *testing.T) {
	srv, ts := newTestServer(t)
	writeLogFile(t, srv.cfg.appdLogPath(), `{"time":"2026-07-03T10:00:00Z","msg":"deploy complete"}`+"\n")

	status, body := getLogs(t, ts.URL, "?stream=appd")
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if body["stream"] != "appd" || body["count"].(float64) != 1 {
		t.Errorf("body = %v, want appd stream with 1 line", body)
	}

	status, _ = getLogs(t, ts.URL, "?stream=bogus")
	if status != http.StatusBadRequest {
		t.Errorf("bogus stream status = %d, want 400", status)
	}
}
