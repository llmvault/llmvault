package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// appsRealtimeRepoRoot walks up from the test's working directory (the e2e
// package dir) until it finds the repository root — the directory that holds
// both go.mod and the app template. Fails clearly if the layout is unexpected.
func appsRealtimeRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 8; i++ {
		if fileExists(filepath.Join(dir, "go.mod")) && dirExists(filepath.Join(dir, "global", "apps", "template")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate repo root (go.mod + global/apps/template) from working dir")
	return ""
}

func fileExists(p string) bool { info, err := os.Stat(p); return err == nil && !info.IsDir() }
func dirExists(p string) bool  { info, err := os.Stat(p); return err == nil && info.IsDir() }

// appsRealtimeRequireTooling fails with a clear diagnostic if the host lacks
// the toolchain the template build needs (go, node, npm, zip).
func appsRealtimeRequireTooling(t *testing.T) {
	t.Helper()
	for _, tool := range []string{"go", "node", "npm", "zip", "make"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatalf("required build tool %q is not on PATH — install it to run the realtime e2e: %v", tool, err)
		}
	}
}

// appsRealtimeBuildTemplate copies global/apps/template into a throwaway
// directory (excluding node_modules/dist/public/.git), then runs `make deps`
// and `make all` cross-compiled for the docker host's container arch
// (linux/<runtime.GOARCH>). It returns the paths to the built source.zip and
// bundle.zip. This is the harness building the template directly — no agent.
func appsRealtimeBuildTemplate(t *testing.T, ctx context.Context) (sourceZip, bundleZip string) {
	t.Helper()
	appsRealtimeRequireTooling(t)
	root := appsRealtimeRepoRoot(t)
	srcDir := filepath.Join(root, "global", "apps", "template")
	dstDir := t.TempDir()
	appsRealtimeCopyTemplate(t, srcDir, dstDir)

	goarch := runtime.GOARCH
	env := append(os.Environ(), "GOOS=linux", "GOARCH="+goarch, "CGO_ENABLED=0")
	t.Logf("building app template in %s for linux/%s", dstDir, goarch)

	depsStart := time.Now()
	appsRealtimeRunMake(t, ctx, dstDir, env, 8*time.Minute, "deps")
	t.Logf("make deps (npm ci) finished in %s", time.Since(depsStart).Round(time.Second))

	buildStart := time.Now()
	appsRealtimeRunMake(t, ctx, dstDir, env, 6*time.Minute, "all")
	t.Logf("make all (web+server+bundle+source) finished in %s", time.Since(buildStart).Round(time.Second))

	sourceZip = filepath.Join(dstDir, "dist", "source.zip")
	bundleZip = filepath.Join(dstDir, "dist", "bundle.zip")
	for _, p := range []string{sourceZip, bundleZip} {
		info, err := os.Stat(p)
		if err != nil || info.Size() <= 0 {
			t.Fatalf("expected build artifact %s (err=%v)", p, err)
		}
	}
	t.Logf("built artifacts: source.zip=%d bytes bundle.zip=%d bytes",
		fileSize(sourceZip), fileSize(bundleZip))
	return sourceZip, bundleZip
}

func fileSize(p string) int64 {
	info, err := os.Stat(p)
	if err != nil {
		return 0
	}
	return info.Size()
}

// appsRealtimeRunMake runs one make target in dir with the given env and a
// bounded timeout, failing with the captured output tail on error.
func appsRealtimeRunMake(t *testing.T, ctx context.Context, dir string, env []string, timeout time.Duration, target string) {
	t.Helper()
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, "make", target)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		tail := string(out)
		if len(tail) > 6000 {
			tail = tail[len(tail)-6000:]
		}
		t.Fatalf("make %s failed in %s: %v\n--- output tail ---\n%s", target, dir, err, tail)
	}
}

// appsRealtimeCopyTemplate copies the template tree, skipping build outputs and
// VCS/OS cruft so `make deps`/`make all` regenerate them from scratch.
func appsRealtimeCopyTemplate(t *testing.T, srcDir, dstDir string) {
	t.Helper()
	skip := map[string]bool{
		filepath.Join("web", "node_modules"): true,
		"dist":                               true,
		"public":                             true,
		".git":                               true,
	}
	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		for prefix := range skip {
			if rel == prefix || strings.HasPrefix(rel, prefix+string(filepath.Separator)) {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		if strings.HasSuffix(rel, ".DS_Store") {
			return nil
		}
		target := filepath.Join(dstDir, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target, info.Mode())
	})
	if err != nil {
		t.Fatalf("copy template tree: %v", err)
	}
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// appsRealtimePublishResult is the 201 body of POST /v1/apps/{appID}/versions.
type appsRealtimePublishResult struct {
	VersionID string `json:"version_id"`
	URL       string `json:"url"`
	Status    string `json:"status"`
}

// appsRealtimePublish uploads the built source+bundle via the REST publish
// endpoint (multipart), proving the server-side deploy path end to end. The
// deploy is synchronous, so the client tolerates a long round-trip.
func appsRealtimePublish(t *testing.T, ctx context.Context, apiBase, token, orgID, appID, sourceZip, bundleZip string) appsRealtimePublishResult {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for field, path := range map[string]string{"source": sourceZip, "bundle": bundleZip} {
		part, err := writer.CreateFormFile(field, filepath.Base(path))
		if err != nil {
			t.Fatalf("create form file %s: %v", field, err)
		}
		f, err := os.Open(path)
		if err != nil {
			t.Fatalf("open %s: %v", path, err)
		}
		if _, err := io.Copy(part, f); err != nil {
			f.Close()
			t.Fatalf("copy %s: %v", path, err)
		}
		f.Close()
	}
	if err := writer.WriteField("notes", "apps realtime e2e"); err != nil {
		t.Fatalf("write notes field: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+"/v1/apps/"+appID+"/versions", body)
	if err != nil {
		t.Fatalf("build publish request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-ID", orgID)
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("publish request failed: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("publish status=%d want 201; body=%s", resp.StatusCode, raw)
	}
	var out appsRealtimePublishResult
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode publish response: %v body=%s", err, raw)
	}
	if out.VersionID == "" || out.Status != "running" || strings.TrimSpace(out.URL) == "" {
		t.Fatalf("publish response incomplete: %+v (body=%s)", out, raw)
	}
	return out
}
