package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return condition()
}

func TestArtifactWatchLoopSyncsOnChange(t *testing.T) {
	artifactDir := createTestArtifact(t)
	var syncCount atomic.Int64
	var lastBody atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r.Body)
		lastBody.Store(buf.String())
		syncCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"synced":true}`))
	}))
	defer server.Close()
	t.Setenv(envControlPlaneURL, server.URL)
	t.Setenv(envRuntimeSecret, "runtime-secret")
	t.Setenv(envAgentID, "agent-1")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logs := &safeBuffer{}
	done := make(chan error, 1)
	go func() {
		done <- runArtifactWatchLoop(ctx, artifactDir, 30*time.Millisecond, logs)
	}()

	if !waitFor(t, 3*time.Second, func() bool { return syncCount.Load() >= 1 }) {
		t.Fatalf("initial sync never happened, logs: %s", logs.String())
	}

	htmlPath := filepath.Join(artifactDir, "index.html")
	updated := `<!doctype html><html><head><meta charset="utf-8"><title>Landing Page</title></head><body><main data-canvas-id="page"><section data-canvas-id="hero"><h1>Updated Headline</h1></section></main></body></html>`
	if err := os.WriteFile(htmlPath, []byte(updated), 0o600); err != nil {
		t.Fatalf("write html: %v", err)
	}
	if !waitFor(t, 3*time.Second, func() bool { return syncCount.Load() >= 2 }) {
		t.Fatalf("change was not synced, logs: %s", logs.String())
	}
	if body, _ := lastBody.Load().(string); !strings.Contains(body, "Updated Headline") {
		t.Fatalf("synced payload missing updated content: %s", body)
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("watch loop returned error: %v", err)
	}
	if !strings.Contains(logs.String(), `"event":"synced"`) {
		t.Fatalf("logs missing synced event: %s", logs.String())
	}
}

func TestArtifactWatchLoopSkipsSyncWhenValidationFails(t *testing.T) {
	artifactDir := createTestArtifact(t)
	var syncCount atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		syncCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"synced":true}`))
	}))
	defer server.Close()
	t.Setenv(envControlPlaneURL, server.URL)
	t.Setenv(envRuntimeSecret, "runtime-secret")
	t.Setenv(envAgentID, "agent-1")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logs := &safeBuffer{}
	done := make(chan error, 1)
	go func() {
		done <- runArtifactWatchLoop(ctx, artifactDir, 30*time.Millisecond, logs)
	}()

	if !waitFor(t, 3*time.Second, func() bool { return syncCount.Load() >= 1 }) {
		t.Fatalf("initial sync never happened, logs: %s", logs.String())
	}
	countAfterInitial := syncCount.Load()

	if err := os.Remove(filepath.Join(artifactDir, "index.html")); err != nil {
		t.Fatalf("remove html: %v", err)
	}
	if !waitFor(t, 3*time.Second, func() bool {
		return strings.Contains(logs.String(), `"event":"validation_failed"`)
	}) {
		t.Fatalf("validation failure was not logged: %s", logs.String())
	}
	if syncCount.Load() != countAfterInitial {
		t.Fatalf("invalid artifact was synced anyway: %d -> %d", countAfterInitial, syncCount.Load())
	}

	restored := `<!doctype html><html><head><meta charset="utf-8"><title>Landing Page</title></head><body><main data-canvas-id="page"><section data-canvas-id="hero"><h1>Back Again</h1></section></main></body></html>`
	if err := os.WriteFile(filepath.Join(artifactDir, "index.html"), []byte(restored), 0o600); err != nil {
		t.Fatalf("restore html: %v", err)
	}
	if !waitFor(t, 3*time.Second, func() bool { return syncCount.Load() > countAfterInitial }) {
		t.Fatalf("fixed artifact was not synced, logs: %s", logs.String())
	}

	cancel()
	<-done
}

func TestArtifactWatchStartStopLifecycle(t *testing.T) {
	artifactDir := createTestArtifact(t)
	t.Setenv("CANVAS_CLI_STATE_DIR", t.TempDir())
	t.Setenv(envControlPlaneURL, "http://127.0.0.1:1")
	t.Setenv(envRuntimeSecret, "runtime-secret")
	t.Setenv(envAgentID, "agent-1")

	originalSpawn := watchSpawn
	watchSpawn = func(dir, logPath string) (int, error) {
		cmd := exec.Command("sleep", "60")
		if err := cmd.Start(); err != nil {
			return 0, err
		}
		go func() { _ = cmd.Wait() }()
		return cmd.Process.Pid, nil
	}
	defer func() { watchSpawn = originalSpawn }()

	startOut := captureStdout(t, func() error {
		return run([]string{"artifact", "watch", artifactDir})
	})
	if !strings.Contains(startOut, `"watching": true`) || !strings.Contains(startOut, `"already_watching": false`) {
		t.Fatalf("start output = %s", startOut)
	}
	if !strings.Contains(startOut, "artifact watch stop") {
		t.Fatalf("start output missing stop command: %s", startOut)
	}

	againOut := captureStdout(t, func() error {
		return run([]string{"artifact", "watch", artifactDir})
	})
	if !strings.Contains(againOut, `"already_watching": true`) {
		t.Fatalf("second start output = %s", againOut)
	}

	statusOut := captureStdout(t, func() error {
		return run([]string{"artifact", "watch", "status", artifactDir})
	})
	if !strings.Contains(statusOut, `"watching": true`) {
		t.Fatalf("status output = %s", statusOut)
	}

	stopOut := captureStdout(t, func() error {
		return run([]string{"artifact", "watch", "stop", artifactDir})
	})
	if !strings.Contains(stopOut, `"stopped": true`) {
		t.Fatalf("stop output = %s", stopOut)
	}

	stopAgainOut := captureStdout(t, func() error {
		return run([]string{"artifact", "watch", "stop", artifactDir})
	})
	if !strings.Contains(stopAgainOut, `"stopped": false`) {
		t.Fatalf("second stop output = %s", stopAgainOut)
	}

	statusAfterOut := captureStdout(t, func() error {
		return run([]string{"artifact", "watch", "status", artifactDir})
	})
	if !strings.Contains(statusAfterOut, `"watching": false`) {
		t.Fatalf("status after stop output = %s", statusAfterOut)
	}
}

func TestArtifactCreateAutoStartsWatcherWhenRuntimeConfigured(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv(envCanvasWorkspace, workspace)
	t.Setenv("CANVAS_CLI_STATE_DIR", t.TempDir())
	t.Setenv(envControlPlaneURL, "http://127.0.0.1:1")
	t.Setenv(envRuntimeSecret, "runtime-secret")
	t.Setenv(envAgentID, "agent-1")

	var spawnedDir atomic.Value
	originalSpawn := watchSpawn
	watchSpawn = func(dir, logPath string) (int, error) {
		spawnedDir.Store(dir)
		cmd := exec.Command("sleep", "60")
		if err := cmd.Start(); err != nil {
			return 0, err
		}
		go func() { _ = cmd.Wait() }()
		return cmd.Process.Pid, nil
	}
	defer func() { watchSpawn = originalSpawn }()

	createOut := captureStdout(t, func() error {
		return run([]string{"artifact", "create", "--project", "launch", "--type", artifactTypeWebPage, "--name", "Landing Page"})
	})
	if !strings.Contains(createOut, `"watching": true`) {
		t.Fatalf("create output missing watch status: %s", createOut)
	}
	artifactDir := filepath.Join(workspace, "projects", "launch", "artifacts", "landing-page")
	if got, _ := spawnedDir.Load().(string); got != artifactDir {
		t.Fatalf("watcher spawned for %q, want %q", got, artifactDir)
	}

	captureStdout(t, func() error {
		return run([]string{"artifact", "watch", "stop", artifactDir})
	})
}

func TestArtifactCreateSkipsWatcherWithoutRuntimeEnv(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv(envCanvasWorkspace, workspace)
	t.Setenv(envControlPlaneURL, "")
	t.Setenv(envRuntimeSecret, "")
	t.Setenv(envAgentID, "")

	createOut := captureStdout(t, func() error {
		return run([]string{"artifact", "create", "--project", "launch", "--type", artifactTypeWebPage, "--name", "Landing Page"})
	})
	if strings.Contains(createOut, `"watch"`) {
		t.Fatalf("create output should not include watch without runtime env: %s", createOut)
	}
}

func TestSnapshotsEqualDetectsChanges(t *testing.T) {
	a := watchSnapshot{"index.html": {ModTimeNanos: 1, Size: 10}}
	b := watchSnapshot{"index.html": {ModTimeNanos: 1, Size: 10}}
	if !snapshotsEqual(a, b) {
		t.Fatal("identical snapshots reported unequal")
	}
	b["index.html"] = watchFileStamp{ModTimeNanos: 2, Size: 10}
	if snapshotsEqual(a, b) {
		t.Fatal("modified snapshot reported equal")
	}
	b["index.html"] = watchFileStamp{ModTimeNanos: 1, Size: 10}
	b["extra.html"] = watchFileStamp{ModTimeNanos: 1, Size: 5}
	if snapshotsEqual(a, b) {
		t.Fatal("added file reported equal")
	}
}
