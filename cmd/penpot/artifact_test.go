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

func TestArtifactCreateAndValidateHappyPath(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv(envCanvasWorkspace, workspace)

	createOut := captureStdout(t, func() error {
		return run([]string{"artifact", "create", "--project", "launch", "--type", artifactTypeWebPage, "--name", "Landing Page"})
	})
	if !strings.Contains(createOut, `"slug": "landing-page"`) {
		t.Fatalf("create output missing slug: %s", createOut)
	}
	artifactDir := filepath.Join(workspace, "projects", "launch", "artifacts", "landing-page")
	if _, err := os.Stat(filepath.Join(workspace, "projects", "launch", "project.json")); err != nil {
		t.Fatalf("project manifest missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(artifactDir, "artifact.json")); err != nil {
		t.Fatalf("artifact manifest missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(artifactDir, "index.html")); err != nil {
		t.Fatalf("artifact html missing: %v", err)
	}

	validateOut := captureStdout(t, func() error {
		return run([]string{"artifact", "validate", artifactDir})
	})
	if !strings.Contains(validateOut, `"valid": true`) {
		t.Fatalf("validate output = %s", validateOut)
	}
}

func TestArtifactVerifyHappyPath(t *testing.T) {
	artifactDir := createTestArtifact(t)
	out := captureStdout(t, func() error {
		return run([]string{"artifact", "verify", filepath.Join(artifactDir, "artifact.json")})
	})
	if !strings.Contains(out, `"verified": true`) {
		t.Fatalf("verify output = %s", out)
	}
}

func TestArtifactSyncPostsDocumentedPayload(t *testing.T) {
	artifactDir := createTestArtifact(t)
	const runtimeSecret = "runtime-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if r.URL.Path != "/internal/agents/agent-1/canvas/artifacts/sync" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+runtimeSecret {
			t.Errorf("authorization = %q", got)
		}
		var payload artifactSyncPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload.SessionID != "session-1" {
			t.Errorf("session_id = %q", payload.SessionID)
		}
		if payload.Project.Ref != "launch" || payload.Project.Slug != "launch" {
			t.Errorf("project = %#v", payload.Project)
		}
		if payload.Artifact.Name != "Landing Page" || payload.Artifact.Type != artifactTypeWebPage {
			t.Errorf("artifact = %#v", payload.Artifact)
		}
		if len(payload.Files) != 1 {
			t.Fatalf("files = %#v", payload.Files)
		}
		file := payload.Files[0]
		if file.Path != "index.html" || file.ContentType != "text/html" || file.SHA256 == "" || file.SizeBytes == 0 {
			t.Errorf("file payload = %#v", file)
		}
		if !strings.Contains(file.Content, `data-hivy-id="page-root"`) {
			t.Errorf("file content missing data-hivy-id: %s", file.Content)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"synced":true}`))
	}))
	defer server.Close()

	t.Setenv(envControlPlaneURL, server.URL)
	t.Setenv(envRuntimeSecret, runtimeSecret)
	t.Setenv(envAgentID, "agent-1")
	t.Setenv("HIVY_SESSION_ID", "session-1")

	out := captureStdout(t, func() error {
		return run([]string{"artifact", "sync", artifactDir})
	})
	if !strings.Contains(out, `"synced": true`) {
		t.Fatalf("sync output = %s", out)
	}
}

func createTestArtifact(t *testing.T) string {
	t.Helper()
	workspace := t.TempDir()
	t.Setenv(envCanvasWorkspace, workspace)
	captureStdout(t, func() error {
		return run([]string{"artifact", "create", "--project", "launch", "--type", artifactTypeWebPage, "--name", "Landing Page"})
	})
	return filepath.Join(workspace, "projects", "launch", "artifacts", "landing-page")
}
