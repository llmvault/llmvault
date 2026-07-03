package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// longRunningScript is a stand-in app server: logs a marker (and an env var)
// then blocks until terminated.
const longRunningScript = "#!/bin/sh\necho \"fake app started GREETING=$GREETING\"\ntrap 'exit 0' TERM\nwhile true; do sleep 1; done\n"

func TestDeployHappyPathEndToEnd(t *testing.T) {
	srv, ts := newTestServer(t)

	bundle, sha := makeBundle(t, longRunningScript, map[string]string{"public/index.html": "<html>v1</html>"})
	bundleURL := serveBundle(t, bundle)

	status, body := doJSON(t, ts, http.MethodPost, "/deploy", testSecret, map[string]any{
		"bundle_url": bundleURL,
		"sha256":     sha,
		"version_id": "ver-0001",
		"env":        map[string]string{"GREETING": "hello-from-env"},
	})
	if status != http.StatusOK {
		t.Fatalf("deploy status = %d, body = %v", status, body)
	}
	if body["old_sha"] != "" || body["new_sha"] != sha {
		t.Errorf("unexpected shas: old=%v new=%v want new=%s", body["old_sha"], body["new_sha"], sha)
	}

	// Release extracted, symlink swapped, env file written 0600.
	releaseDir := filepath.Join(srv.cfg.releasesDir(), sha)
	if _, err := os.Stat(filepath.Join(releaseDir, "server")); err != nil {
		t.Fatalf("release server binary missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(releaseDir, "public", "index.html")); err != nil {
		t.Errorf("release asset missing: %v", err)
	}
	target, err := os.Readlink(srv.cfg.currentLink())
	if err != nil || filepath.Base(target) != sha {
		t.Fatalf("current symlink = %q (err %v), want -> %s", target, err, sha)
	}
	envInfo, err := os.Stat(srv.cfg.envFile())
	if err != nil {
		t.Fatalf("env file missing: %v", err)
	}
	if envInfo.Mode().Perm() != 0o600 {
		t.Errorf("env file perms = %v, want 0600", envInfo.Mode().Perm())
	}

	// The direct-supervision manager actually started the fake server with
	// the env file applied, capturing its stdout in app.log.
	waitFor(t, 5*time.Second, func() bool {
		data, err := os.ReadFile(srv.cfg.appLogPath())
		return err == nil && strings.Contains(string(data), "fake app started GREETING=hello-from-env")
	}, "app log to contain startup marker with env var")

	procStatus := srv.proc.Status(t.Context())
	if procStatus.State != stateRunning || procStatus.PID == 0 {
		t.Fatalf("app status = %+v, want running with pid", procStatus)
	}

	// Health reflects the active release and version marker.
	status, health := doJSON(t, ts, http.MethodGet, "/health", testSecret, nil)
	if status != http.StatusOK {
		t.Fatalf("health status = %d", status)
	}
	if health["active_sha"] != sha || health["version_id"] != "ver-0001" {
		t.Errorf("health = %v, want active_sha=%s version_id=ver-0001", health, sha)
	}

	// Second deploy: old_sha is the previous release, both stay on disk.
	bundle2, sha2 := makeBundle(t, longRunningScript, map[string]string{"public/index.html": "<html>v2</html>"})
	status, body = doJSON(t, ts, http.MethodPost, "/deploy", testSecret, map[string]any{
		"bundle_url": serveBundle(t, bundle2),
		"sha256":     sha2,
		"version_id": "ver-0002",
	})
	if status != http.StatusOK {
		t.Fatalf("second deploy status = %d, body = %v", status, body)
	}
	if body["old_sha"] != sha || body["new_sha"] != sha2 {
		t.Errorf("second deploy shas: old=%v new=%v, want old=%s new=%s", body["old_sha"], body["new_sha"], sha, sha2)
	}
	if _, err := os.Stat(filepath.Join(srv.cfg.releasesDir(), sha, "server")); err != nil {
		t.Errorf("previous release was removed: %v", err)
	}

	// Rollback to the first release.
	status, body = doJSON(t, ts, http.MethodPost, "/rollback", testSecret, map[string]any{"sha256": sha})
	if status != http.StatusOK {
		t.Fatalf("rollback status = %d, body = %v", status, body)
	}
	if body["old_sha"] != sha2 || body["new_sha"] != sha {
		t.Errorf("rollback shas: old=%v new=%v, want old=%s new=%s", body["old_sha"], body["new_sha"], sha2, sha)
	}
	target, err = os.Readlink(srv.cfg.currentLink())
	if err != nil || filepath.Base(target) != sha {
		t.Errorf("current symlink after rollback = %q (err %v), want -> %s", target, err, sha)
	}
}

func TestDeployRejectsSHAMismatch(t *testing.T) {
	srv, ts := newTestServer(t)
	bundle, _ := makeBundle(t, longRunningScript, nil)
	wrongSHA := strings.Repeat("ab", 32)

	status, body := doJSON(t, ts, http.MethodPost, "/deploy", testSecret, map[string]any{
		"bundle_url": serveBundle(t, bundle),
		"sha256":     wrongSHA,
	})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", status)
	}
	if !strings.Contains(body["error"].(string), "sha256 mismatch") {
		t.Errorf("error = %v, want sha256 mismatch", body["error"])
	}
	// Nothing extracted, temp download cleaned up, no symlink created.
	if entries, _ := os.ReadDir(srv.cfg.releasesDir()); len(entries) != 0 {
		t.Errorf("releases dir not empty after rejected deploy: %v", entries)
	}
	if entries, _ := os.ReadDir(srv.cfg.tmpDir()); len(entries) != 0 {
		t.Errorf("tmp dir not cleaned after rejected deploy: %v", entries)
	}
	if _, err := os.Readlink(srv.cfg.currentLink()); err == nil {
		t.Error("current symlink exists after rejected deploy")
	}
}

func TestDeployRejectsInvalidSHAFormat(t *testing.T) {
	_, ts := newTestServer(t)
	for _, sha := range []string{"", "short", strings.Repeat("Z", 64), strings.Repeat("ab", 31)} {
		status, _ := doJSON(t, ts, http.MethodPost, "/deploy", testSecret, map[string]any{
			"bundle_url": "http://127.0.0.1:1/bundle.zip",
			"sha256":     sha,
		})
		if status != http.StatusBadRequest {
			t.Errorf("sha %q: status = %d, want 400", sha, status)
		}
	}
}

func TestRollbackUnknownReleaseIs404(t *testing.T) {
	_, ts := newTestServer(t)
	status, _ := doJSON(t, ts, http.MethodPost, "/rollback", testSecret, map[string]any{
		"sha256": strings.Repeat("cd", 32),
	})
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}

func TestSwapCurrentSymlinkIsAtomicRepoint(t *testing.T) {
	root := t.TempDir()
	cfg := config{AppRoot: filepath.Join(root, "app"), LogsDir: filepath.Join(root, "logs")}
	if err := ensureDirs(cfg); err != nil {
		t.Fatalf("ensureDirs: %v", err)
	}
	shaA := strings.Repeat("aa", 32)
	shaB := strings.Repeat("bb", 32)
	for _, sha := range []string{shaA, shaB} {
		if err := os.MkdirAll(filepath.Join(cfg.releasesDir(), sha), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if err := swapCurrentSymlink(cfg, shaA); err != nil {
		t.Fatalf("first swap: %v", err)
	}
	target, err := os.Readlink(cfg.currentLink())
	if err != nil || target != filepath.Join("releases", shaA) {
		t.Fatalf("current -> %q (err %v), want releases/%s", target, err, shaA)
	}

	// Repointing over an existing symlink must succeed (rename semantics)
	// and leave no temp links behind.
	if err := swapCurrentSymlink(cfg, shaB); err != nil {
		t.Fatalf("second swap: %v", err)
	}
	target, err = os.Readlink(cfg.currentLink())
	if err != nil || target != filepath.Join("releases", shaB) {
		t.Fatalf("current -> %q (err %v), want releases/%s", target, err, shaB)
	}
	entries, err := os.ReadDir(cfg.AppRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".current-") {
			t.Errorf("stale temp symlink left behind: %s", entry.Name())
		}
	}
}
