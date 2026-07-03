package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type logsResponse struct {
	Stream  string    `json:"stream"`
	Lines   []string  `json:"lines"`
	Count   int       `json:"count"`
	App     appStatus `json:"app"`
	Journal []string  `json:"journal,omitempty"`
}

// handleLogs reads the size-rotated log files for the requested stream.
//
//	GET /logs?lines=&grep=&since=&stream=app|appd
//
// grep is a per-line regex applied before the line cap; since filters out
// lines whose parseable timestamp is older than the given RFC3339 time
// (lines without a recognizable timestamp are kept). The line count is
// hard-capped at 2000 after filtering.
func (s *server) handleLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	stream := q.Get("stream")
	if stream == "" {
		stream = "app"
	}
	if stream != "app" && stream != "appd" {
		writeError(w, http.StatusBadRequest, `stream must be "app" or "appd"`)
		return
	}

	limit := defaultLogLines
	if raw := q.Get("lines"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "lines must be a positive integer")
			return
		}
		limit = n
	}
	if limit > maxLogLines {
		limit = maxLogLines
	}

	var grep *regexp.Regexp
	if raw := q.Get("grep"); raw != "" {
		re, err := regexp.Compile(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid grep regex: "+err.Error())
			return
		}
		grep = re
	}

	var since time.Time
	if raw := q.Get("since"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "since must be RFC3339: "+err.Error())
			return
		}
		since = t
	}

	base := s.cfg.appLogPath()
	if stream == "appd" {
		base = s.cfg.appdLogPath()
	}
	lines, err := readLogTail(base, limit, grep, since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read logs: "+err.Error())
		return
	}

	resp := logsResponse{Stream: stream, Lines: lines, Count: len(lines), App: s.proc.Status(r.Context())}
	if stream == "app" {
		resp.Journal = s.proc.Journal(r.Context(), 50)
	}
	writeJSON(w, http.StatusOK, resp)
}

// readLogTail collects up to limit filtered lines across the rotated set
// (base.N .. base.1, base), returned in chronological order. Files are
// scanned newest-first and reading stops once enough lines matched.
func readLogTail(base string, limit int, grep *regexp.Regexp, since time.Time) ([]string, error) {
	paths := []string{base}
	for i := 1; i <= rotateKeep; i++ {
		paths = append(paths, fmt.Sprintf("%s.%d", base, i))
	}

	var collected []string // newest-last within each chunk, chunks prepended
	for _, path := range paths {
		if len(collected) >= limit {
			break
		}
		matched, err := filterFileLines(path, limit-len(collected), grep, since)
		if err != nil {
			return nil, err
		}
		collected = append(matched, collected...)
	}
	if collected == nil {
		collected = []string{}
	}
	return collected, nil
}

// filterFileLines returns the last keep lines of the file that pass the grep
// and since filters, in file order. A missing file yields no lines.
func filterFileLines(path string, keep int, grep *regexp.Regexp, since time.Time) ([]string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var matched []string
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		if grep != nil && !grep.MatchString(line) {
			continue
		}
		if !since.IsZero() {
			if ts, ok := lineTimestamp(line); ok && ts.Before(since) {
				continue
			}
		}
		matched = append(matched, line)
	}
	if len(matched) > keep {
		matched = matched[len(matched)-keep:]
	}
	return matched, nil
}

// lineTimestamp best-effort extracts a timestamp from a log line: JSON lines
// are checked for time/ts/timestamp fields, plain lines for a leading
// RFC3339 token.
func lineTimestamp(line string) (time.Time, bool) {
	if strings.HasPrefix(line, "{") {
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err == nil {
			for _, key := range []string{"time", "ts", "timestamp"} {
				raw, ok := obj[key].(string)
				if !ok {
					continue
				}
				if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
					return t, true
				}
			}
		}
		return time.Time{}, false
	}
	token, _, _ := strings.Cut(line, " ")
	if t, err := time.Parse(time.RFC3339Nano, token); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339, token); err == nil {
		return t, true
	}
	return time.Time{}, false
}
