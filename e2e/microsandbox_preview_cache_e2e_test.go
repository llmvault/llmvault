package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
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

func TestMicrosandboxPreviewCacheWakeE2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tempDir := t.TempDir()
	scriptPath := filepath.Join(tempDir, "microsandbox-preview-cache.py")
	rawScript, err := os.ReadFile("../ansible/roles/microsandbox-preview-cache/templates/microsandbox-preview-cache.py.j2")
	if err != nil {
		t.Fatalf("read preview-cache template: %v", err)
	}
	if err := os.WriteFile(scriptPath, rawScript, 0o755); err != nil {
		t.Fatalf("write preview-cache script: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "redis.py"), []byte(fakePreviewCacheRedisModule), 0o644); err != nil {
		t.Fatalf("write fake redis module: %v", err)
	}

	cachePort := freeTCPPort(t)
	cacheBaseURL := fmt.Sprintf("http://127.0.0.1:%d", cachePort)
	const (
		sandboxID           = "sbx-preview-e2e"
		concurrentSandboxID = "sbx-preview-e2e-concurrent"
		cacheToken          = "preview-cache-token"
		activityToken       = "preview-activity-token"
		upstream            = "127.0.0.1:49123"
	)

	var wakeCalls atomic.Int64
	var activityCalls atomic.Int64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		routePath := strings.TrimPrefix(r.URL.Path, "/internal/preview/sandboxes/")
		requestSandboxID, ok := strings.CutSuffix(routePath, "/activity")
		if !ok || requestSandboxID == routePath || requestSandboxID == "" {
			http.NotFound(w, r)
			return
		}
		if got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); got != activityToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		activityCalls.Add(1)
		if r.URL.Query().Get("wake") == "true" {
			wakeCalls.Add(1)
			if err := postPreviewCacheRouteE(ctx, cacheBaseURL, cacheToken, requestSandboxID, "running", upstream); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"running"}`))
	}))
	defer backend.Close()

	cmd := exec.CommandContext(ctx, "python3", scriptPath)
	cmd.Env = append(os.Environ(),
		"PYTHONPATH="+tempDir,
		"HIVY_MICROSANDBOX_PREVIEW_CACHE_ADDR=127.0.0.1",
		fmt.Sprintf("HIVY_MICROSANDBOX_PREVIEW_CACHE_PORT=%d", cachePort),
		"HIVY_MICROSANDBOX_PREVIEW_CACHE_REDIS_URL=memory://preview-cache-e2e",
		"HIVY_MICROSANDBOX_PREVIEW_CACHE_TOKEN="+cacheToken,
		"HIVY_MICROSANDBOX_PREVIEW_BASE_DOMAIN=preview.test",
		"HIVY_PREVIEW_ACTIVITY_API_URL="+backend.URL,
		"HIVY_PREVIEW_ACTIVITY_TOKEN="+activityToken,
		"HIVY_MICROSANDBOX_PREVIEW_WAKE_TIMEOUT=5s",
	)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatalf("start preview-cache: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if t.Failed() {
			t.Logf("preview-cache output:\n%s", output.String())
		}
	})
	waitPreviewCacheHealth(t, ctx, cacheBaseURL)

	postPreviewCacheRoute(t, ctx, cacheBaseURL, cacheToken, sandboxID, "stopped", upstream)
	if got := lookupPreviewCache(t, ctx, cacheBaseURL, sandboxID); got != upstream {
		t.Fatalf("lookup upstream = %q, want %q", got, upstream)
	}
	if got := wakeCalls.Load(); got != 1 {
		t.Fatalf("wake calls after stopped lookup = %d, want 1", got)
	}

	if got := lookupPreviewCache(t, ctx, cacheBaseURL, sandboxID); got != upstream {
		t.Fatalf("running lookup upstream = %q, want %q", got, upstream)
	}
	if got := wakeCalls.Load(); got != 1 {
		t.Fatalf("wake calls after running lookup = %d, want still 1", got)
	}

	postPreviewCacheRoute(t, ctx, cacheBaseURL, cacheToken, concurrentSandboxID, "stopped", upstream)
	wakeCalls.Store(0)
	const concurrentLookups = 5
	var wg sync.WaitGroup
	errCh := make(chan error, concurrentLookups)
	for i := 0; i < concurrentLookups; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := lookupPreviewCacheE(ctx, cacheBaseURL, concurrentSandboxID)
			if err != nil {
				errCh <- err
				return
			}
			if got != upstream {
				errCh <- fmt.Errorf("lookup upstream = %q, want %q", got, upstream)
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := wakeCalls.Load(); got != 1 {
		t.Fatalf("concurrent stopped lookups triggered %d wake calls, want 1", got)
	}
	if got := activityCalls.Load(); got == 0 {
		t.Fatal("preview activity endpoint was not called")
	}
}

func waitPreviewCacheHealth(t *testing.T, ctx context.Context, baseURL string) {
	t.Helper()
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("preview-cache health did not become ready")
}

func postPreviewCacheRoute(t *testing.T, ctx context.Context, baseURL, token, sandboxID, status, upstream string) {
	t.Helper()
	if err := postPreviewCacheRouteE(ctx, baseURL, token, sandboxID, status, upstream); err != nil {
		t.Fatal(err)
	}
}

func postPreviewCacheRouteE(ctx context.Context, baseURL, token, sandboxID, status, upstream string) error {
	body, _ := json.Marshal(map[string]any{
		"sandbox_id": sandboxID,
		"status":     status,
		"upstreams": map[string]string{
			"3000": "http://" + upstream,
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/routes/"+sandboxID, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build route request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("post preview route: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("post preview route status=%d", resp.StatusCode)
	}
	return nil
}

func lookupPreviewCache(t *testing.T, ctx context.Context, baseURL, sandboxID string) string {
	t.Helper()
	upstream, err := lookupPreviewCacheE(ctx, baseURL, sandboxID)
	if err != nil {
		t.Fatal(err)
	}
	return upstream
}

func lookupPreviewCacheE(ctx context.Context, baseURL, sandboxID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/lookup", nil)
	if err != nil {
		return "", fmt.Errorf("build lookup request: %w", err)
	}
	req.Host = "3000-" + sandboxID + ".preview.test"
	req.Header.Set("X-Forwarded-Host", req.Host)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("lookup preview route: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return "", fmt.Errorf("lookup status=%d", resp.StatusCode)
	}
	return resp.Header.Get("X-Microsandbox-Upstream"), nil
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate tcp port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

const fakePreviewCacheRedisModule = `
import threading
import time

_lock = threading.Lock()
_store = {}

class Redis:
    @classmethod
    def from_url(cls, url, decode_responses=True):
        return cls()

    def ping(self):
        return True

    def _expired(self, key, now=None):
        now = now or time.time()
        item = _store.get(key)
        if not item:
            return True
        _, expires_at = item
        if expires_at and expires_at <= now:
            _store.pop(key, None)
            return True
        return False

    def set(self, key, value, ex=None, nx=False):
        with _lock:
            if nx and not self._expired(key):
                return False
            expires_at = time.time() + ex if ex else None
            _store[key] = (str(value), expires_at)
            return True

    def get(self, key):
        with _lock:
            if self._expired(key):
                return None
            return _store[key][0]

    def delete(self, key):
        with _lock:
            existed = not self._expired(key)
            _store.pop(key, None)
            return 1 if existed else 0
`
