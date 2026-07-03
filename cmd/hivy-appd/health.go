package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type healthResponse struct {
	OK            bool         `json:"ok"`
	UptimeSeconds int64        `json:"uptime_seconds"`
	ActiveSHA     string       `json:"active_sha,omitempty"`
	VersionID     string       `json:"version_id,omitempty"`
	App           appStatus    `json:"app"`
	Probe         *probeResult `json:"probe,omitempty"`
}

type probeResult struct {
	OK         bool   `json:"ok"`
	StatusCode int    `json:"status_code,omitempty"`
	Error      string `json:"error,omitempty"`
	DurationMS int64  `json:"duration_ms"`
}

// handleHealth reports appd liveness, the app process state, the active
// release, and an HTTP probe of the app's /healthz on localhost.
//
//	GET /health
func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := healthResponse{
		OK:            true,
		UptimeSeconds: int64(time.Since(s.started).Seconds()),
		ActiveSHA:     s.currentReleaseSHA(),
		App:           s.proc.Status(r.Context()),
	}
	if resp.ActiveSHA != "" {
		resp.VersionID = readVersionMarker(filepath.Join(s.cfg.releasesDir(), resp.ActiveSHA))
		resp.Probe = s.probeApp(r)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *server) probeApp(r *http.Request) *probeResult {
	start := time.Now()
	url := fmt.Sprintf("http://127.0.0.1:%d/healthz", s.cfg.AppPort)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, url, nil)
	if err != nil {
		return &probeResult{Error: err.Error(), DurationMS: time.Since(start).Milliseconds()}
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return &probeResult{Error: err.Error(), DurationMS: time.Since(start).Milliseconds()}
	}
	defer resp.Body.Close()
	return &probeResult{
		OK:         resp.StatusCode >= 200 && resp.StatusCode < 300,
		StatusCode: resp.StatusCode,
		DurationMS: time.Since(start).Milliseconds(),
	}
}

func readVersionMarker(releaseDir string) string {
	data, err := os.ReadFile(filepath.Join(releaseDir, ".hivy_version_id"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
