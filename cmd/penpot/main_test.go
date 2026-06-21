package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceURLBuildsPenpotWorkspaceURL(t *testing.T) {
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
	want := "https://canvas.usehivy.com/mcp?userToken=abc"
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
	want := cliState{CanvasFileID: "canvas-file", PenpotFileID: "penpot-file"}
	if err := saveState(want); err != nil {
		t.Fatalf("saveState: %v", err)
	}
	got, err := loadState()
	if err != nil {
		t.Fatalf("loadState: %v", err)
	}
	if got.CanvasFileID != want.CanvasFileID || got.PenpotFileID != want.PenpotFileID || got.UpdatedAt == "" {
		t.Fatalf("state = %#v, want %#v with UpdatedAt", got, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "state.json")); err != nil {
		t.Fatalf("state file missing: %v", err)
	}
}
