package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceURLBuildsCanvasWorkspaceURL(t *testing.T) {
	t.Setenv(envCanvasURL, "https://canvas.usehivy.com/")
	t.Setenv(envCanvasTeamID, "team-1")
	got := workspaceURL("file-1", "page-1")
	if !strings.HasPrefix(got, "https://canvas.usehivy.com/#/workspace?") {
		t.Fatalf("workspace url = %q", got)
	}
	if !strings.Contains(got, "team-id=team-1") || !strings.Contains(got, "file-id=file-1") || !strings.Contains(got, "page-id=page-1") {
		t.Fatalf("workspace url missing expected query: %q", got)
	}
}

func TestNormalizeMCPURLUsesStreamableHTTPEndpoint(t *testing.T) {
	got := normalizeMCPURL("https://canvas.usehivy.com/mcp/stream?userToken=abc")
	want := "https://canvas.usehivy.com/mcp/stream?userToken=abc"
	if got != want {
		t.Fatalf("normalizeMCPURL = %q, want %q", got, want)
	}
}

func TestDecodeMCPBodyReadsEventStreamData(t *testing.T) {
	var out map[string]any
	data := []byte("event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"ok\":true}}\n\n")
	if err := decodeMCPBody("text/event-stream", data, &out); err != nil {
		t.Fatalf("decodeMCPBody: %v", err)
	}
	encoded, _ := json.Marshal(out["result"])
	if string(encoded) != `{"ok":true}` {
		t.Fatalf("decoded result = %s", encoded)
	}
}

func TestStateRoundTripUsesConfiguredStateDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PENPOT_CLI_STATE_DIR", dir)
	want := cliState{FileID: "canvas-file"}
	if err := saveState(want); err != nil {
		t.Fatalf("saveState: %v", err)
	}
	got, err := loadState()
	if err != nil {
		t.Fatalf("loadState: %v", err)
	}
	if got.FileID != want.FileID || got.UpdatedAt == "" {
		t.Fatalf("state = %#v, want %#v with UpdatedAt", got, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "state.json")); err != nil {
		t.Fatalf("state file missing: %v", err)
	}
}

func TestDoctorDoesNotExposeRuntimeVariableNames(t *testing.T) {
	dir := t.TempDir()
	browserPath := filepath.Join(dir, "browser")
	if err := os.WriteFile(browserPath, []byte("#!/bin/sh\nexit 0\n"), 0o600); err != nil {
		t.Fatalf("write fake browser: %v", err)
	}
	if err := os.Chmod(browserPath, 0o700); err != nil {
		t.Fatalf("chmod fake browser: %v", err)
	}
	t.Setenv("PATH", dir)
	for _, key := range []string{
		envCanvasURL,
		envCanvasTeamID,
		envCanvasProfileID,
		envCanvasSessionJWT,
		envCanvasMCPURL,
		envControlPlaneURL,
		envAgentID,
		envRuntimeSecret,
	} {
		t.Setenv(key, "")
	}
	err := doctor()
	if err == nil {
		t.Fatalf("doctor unexpectedly succeeded")
	}
	message := err.Error()
	if !strings.Contains(message, "canvas runtime configuration") {
		t.Fatalf("doctor error = %q", message)
	}
	if strings.Contains(message, "PENPOT_") || strings.Contains(message, "HIVY_") {
		t.Fatalf("doctor exposed runtime variable names: %q", message)
	}
}

func TestDoctorUsesFirstPartyRuntimeEnv(t *testing.T) {
	t.Setenv(envControlPlaneURL, "http://control-plane")
	t.Setenv(envAgentID, "agent-1")
	t.Setenv(envRuntimeSecret, "runtime-secret")
	t.Setenv("PATH", t.TempDir())
	if err := doctor(); err != nil {
		t.Fatalf("doctor: %v", err)
	}
}

func TestGetControlPlaneUsesRuntimeAuth(t *testing.T) {
	const runtimeSecret = "runtime-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s", r.Method)
		}
		if r.URL.Path != "/internal/agents/agent-1/canvas/projects" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+runtimeSecret {
			t.Fatalf("authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"projects":[{"project_id":"project-1","name":"Project"}]}`))
	}))
	defer server.Close()

	t.Setenv(envControlPlaneURL, server.URL)
	t.Setenv(envRuntimeSecret, runtimeSecret)

	var out map[string]any
	if err := getControlPlane("/internal/agents/agent-1/canvas/projects", &out); err != nil {
		t.Fatalf("getControlPlane: %v", err)
	}
	projects, ok := out["projects"].([]any)
	if !ok || len(projects) != 1 {
		t.Fatalf("projects = %#v", out["projects"])
	}
}
