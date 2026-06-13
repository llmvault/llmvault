package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return body
}

func tailString(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[len(value)-max:]
}

func waitForWorkspaceFileContaining(t *testing.T, trace *agentRuntimeE2ETrace, workspaceRoot, relPath, marker string, timeout time.Duration) {
	t.Helper()
	path := filepath.Join(workspaceRoot, relPath)
	deadline := time.Now().Add(timeout)
	lastLog := time.Time{}
	for time.Now().Before(deadline) {
		body, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(body), marker) {
			trace.Body("assert-files", path, body)
			return
		}
		if time.Since(lastLog) > 2*time.Second {
			trace.Logf("assert-files", "waiting for %s to contain %q", path, marker)
			lastLog = time.Now()
		}
		time.Sleep(250 * time.Millisecond)
	}
	body, _ := os.ReadFile(path)
	t.Fatalf("file %s did not contain %q within %s; body=%s", path, marker, timeout, body)
}
