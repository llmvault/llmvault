package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestStateRoundTripUsesConfiguredStateDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CANVAS_CLI_STATE_DIR", dir)
	want := cliState{ProjectID: "canvas-project"}
	if err := saveState(want); err != nil {
		t.Fatalf("saveState: %v", err)
	}
	got, err := loadState()
	if err != nil {
		t.Fatalf("loadState: %v", err)
	}
	if got.ProjectID != want.ProjectID || got.UpdatedAt == "" {
		t.Fatalf("state = %#v, want %#v with UpdatedAt", got, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "state.json")); err != nil {
		t.Fatalf("state file missing: %v", err)
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
