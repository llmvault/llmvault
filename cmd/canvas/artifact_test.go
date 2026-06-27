package main

import (
	"encoding/json"
	"io"
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
		if !strings.Contains(file.Content, `data-canvas-id="page"`) {
			t.Errorf("file content missing data-canvas-id: %s", file.Content)
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

func TestArtifactValidateReturnsStructuredCorrectionFeedback(t *testing.T) {
	artifactDir := createTestArtifact(t)
	htmlPath := filepath.Join(artifactDir, "index.html")
	badHTML := `<!doctype html>
<html><body>
  <main>
    <section><h1>Landing Page</h1></section>
  </main>
</body></html>`
	if err := os.WriteFile(htmlPath, []byte(badHTML), 0o644); err != nil {
		t.Fatalf("write bad html: %v", err)
	}

	out, err := captureStdoutWithError(t, func() error {
		return run([]string{"artifact", "validate", artifactDir})
	})
	if err == nil {
		t.Fatalf("validate unexpectedly succeeded: %s", out)
	}
	var result artifactValidationResult
	if decodeErr := json.Unmarshal([]byte(out), &result); decodeErr != nil {
		t.Fatalf("decode validation output: %v\n%s", decodeErr, out)
	}
	if result.Valid {
		t.Fatalf("validation result unexpectedly valid: %+v", result)
	}
	if !validationIssuesContain(result.Issues, "missing_canvas_id") {
		t.Fatalf("expected missing_canvas_id issue: %+v", result.Issues)
	}
	if len(result.Guidance) == 0 {
		t.Fatalf("expected correction guidance: %+v", result)
	}
}

func TestArtifactValidateTreatsHeadingIssuesAsWarnings(t *testing.T) {
	artifactDir := createTestArtifact(t)
	htmlPath := filepath.Join(artifactDir, "index.html")
	html := `<!doctype html>
<html><body>
  <main data-canvas-id="page">
    <section data-canvas-id="hero"><p>No heading here.</p></section>
  </main>
</body></html>`
	if err := os.WriteFile(htmlPath, []byte(html), 0o644); err != nil {
		t.Fatalf("write html: %v", err)
	}

	out := captureStdout(t, func() error {
		return run([]string{"artifact", "validate", artifactDir})
	})
	var result artifactValidationResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("decode validation output: %v\n%s", err, out)
	}
	if !result.Valid {
		t.Fatalf("expected heading warning to allow valid artifact: %+v", result)
	}
	if !validationIssuesContain(result.Issues, "missing_heading") {
		t.Fatalf("expected missing_heading warning: %+v", result.Issues)
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

func captureStdoutWithError(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = writer
	runErr := fn()
	_ = writer.Close()
	os.Stdout = old
	data, readErr := io.ReadAll(reader)
	_ = reader.Close()
	if readErr != nil {
		t.Fatalf("read stdout: %v", readErr)
	}
	return string(data), runErr
}

func validationIssuesContain(issues []artifactValidationIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
