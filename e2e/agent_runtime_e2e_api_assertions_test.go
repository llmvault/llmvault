package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func assertAgentRuntimePostRunAPIs(t *testing.T, trace *agentRuntimeE2ETrace, ctx context.Context, baseURL, token, sessionID, traceID, workspaceRoot string) {
	t.Helper()
	configBody := doRuntimeJSON(t, trace, ctx, http.MethodGet, baseURL+"/config", token, nil, http.StatusOK)
	var config map[string]any
	if err := json.Unmarshal(configBody, &config); err != nil {
		t.Fatalf("decode /config: %v\n%s", err, configBody)
	}
	agent, _ := config["agent"].(map[string]any)
	if name, _ := agent["name"].(string); strings.TrimSpace(name) == "" {
		t.Fatalf("/config response missing agent.name: %s", configBody)
	}
	trace.Logf("assert", "/config returned agent=%v", agent["name"])

	sessionsBody := doRuntimeJSON(t, trace, ctx, http.MethodGet, baseURL+"/sessions?session_id="+sessionID, token, nil, http.StatusOK)
	var sessions struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(sessionsBody, &sessions); err != nil {
		t.Fatalf("decode /sessions: %v\n%s", err, sessionsBody)
	}
	if len(sessions.Items) == 0 {
		t.Fatalf("/sessions did not return session_id=%s: %s", sessionID, sessionsBody)
	}
	trace.Logf("assert", "/sessions returned items=%d", len(sessions.Items))

	detailBody := doRuntimeJSON(t, trace, ctx, http.MethodGet, baseURL+"/sessions/"+sessionID, token, nil, http.StatusOK)
	var detail struct {
		Session map[string]any   `json:"session"`
		Events  []map[string]any `json:"events"`
	}
	if err := json.Unmarshal(detailBody, &detail); err != nil {
		t.Fatalf("decode /sessions/{id}: %v\n%s", err, detailBody)
	}
	if len(detail.Events) == 0 {
		t.Fatalf("/sessions/%s returned no events: %s", sessionID, detailBody)
	}
	trace.Logf("assert", "/sessions/%s returned events=%d", sessionID, len(detail.Events))

	eventsBody := doRuntimeJSON(t, trace, ctx, http.MethodGet, baseURL+"/observability/traces/"+traceID+"/events", token, nil, http.StatusOK)
	var traceEvents []map[string]any
	if err := json.Unmarshal(eventsBody, &traceEvents); err != nil {
		t.Fatalf("decode trace events: %v\n%s", err, eventsBody)
	}
	if len(traceEvents) == 0 {
		t.Fatalf("trace events empty for %s", traceID)
	}
	trace.Logf("assert", "trace events returned count=%d", len(traceEvents))

	summaryBody := doRuntimeJSON(t, trace, ctx, http.MethodGet, baseURL+"/observability/traces/"+traceID+"/summary", token, nil, http.StatusOK)
	var summary map[string]any
	if err := json.Unmarshal(summaryBody, &summary); err != nil {
		t.Fatalf("decode trace summary: %v\n%s", err, summaryBody)
	}
	if got, _ := summary["trace_id"].(string); got != traceID {
		t.Fatalf("trace summary returned trace_id=%q want=%q body=%s", got, traceID, summaryBody)
	}
	trace.Logf("assert", "trace summary returned trace_id=%s", traceID)

	commandBody := mustJSON(t, map[string]any{
		"commands":      []string{"printf control-command-ok > control_command_marker.txt && pwd"},
		"workdir":       ".",
		"stop_on_error": true,
	})
	controlBody := doRuntimeJSON(t, trace, ctx, http.MethodPost, baseURL+"/control/commands", token, commandBody, http.StatusOK)
	var control struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(controlBody, &control); err != nil {
		t.Fatalf("decode /control/commands: %v\n%s", err, controlBody)
	}
	if !control.OK {
		t.Fatalf("/control/commands did not report ok: %s", controlBody)
	}
	marker, err := os.ReadFile(filepath.Join(workspaceRoot, "control_command_marker.txt"))
	if err != nil {
		t.Fatalf("control command marker missing: %v", err)
	}
	if strings.TrimSpace(string(marker)) != "control-command-ok" {
		t.Fatalf("control command marker = %q", marker)
	}
	trace.Logf("assert", "/control/commands wrote marker file")
}
