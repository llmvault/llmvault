package e2e

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

type agentRuntimeContainerOptions struct {
	dbPath string
}

func startAgentRuntimeContainer(t *testing.T, trace *agentRuntimeE2ETrace, ctx context.Context, repoRoot, workspaceRoot, runtimeSecret, proxyToken, agentID, sandboxID, controlPlaneURL string) (string, func()) {
	return startAgentRuntimeContainerWithOptions(t, trace, ctx, repoRoot, workspaceRoot, runtimeSecret, proxyToken, agentID, sandboxID, controlPlaneURL, agentRuntimeContainerOptions{})
}

func startAgentRuntimeContainerWithOptions(t *testing.T, trace *agentRuntimeE2ETrace, ctx context.Context, repoRoot, workspaceRoot, runtimeSecret, proxyToken, agentID, sandboxID, controlPlaneURL string, opts agentRuntimeContainerOptions) (string, func()) {
	t.Helper()
	image, removeImage := agentRuntimeImage(t, trace, ctx, repoRoot)
	port := allocateLocalPort(t)
	trace.Logf("docker", "allocated host port=%s container_port=%s image=%s", port, agentRuntimeContainerPort, image)
	dbPath := opts.dbPath
	if strings.TrimSpace(dbPath) == "" {
		dbPath = "/app/data/runtime.db"
	}

	args := []string{
		"run", "-d", "--rm",
		"--add-host", "host.docker.internal:host-gateway",
		"-p", "127.0.0.1:" + port + ":" + agentRuntimeContainerPort,
		"-v", workspaceRoot + ":/workspace",
		"-e", "HIVY_RUNTIME_SECRET=" + runtimeSecret,
		"-e", "HIVY_RUNTIME_BIND_ADDR=0.0.0.0:" + agentRuntimeContainerPort,
		"-e", "HIVY_DB_PATH=" + dbPath,
		"-e", "HIVY_WORKSPACE_ROOT=/workspace",
		"-e", "HIVY_PROXY_API_KEY=" + proxyToken,
		"-e", "HIVY_CONTROL_PLANE_URL=" + controlPlaneURL,
		"-e", "HIVY_AGENT_ID=" + agentID,
		"-e", "HIVY_ORG_ID=" + uuid.NewString(),
		"-e", "HIVY_SANDBOX_ID=" + sandboxID,
		"-e", "HIVY_DB_SYNC_ENABLED=true",
		"-e", "HIVY_DB_SYNC_WRITE_THRESHOLD=1",
		"-e", "HIVY_DB_SYNC_INTERVAL_SECONDS=1",
		"-e", "SENTRY_DSN=",
		"-e", "SENTRY_SPOTLIGHT=false",
		"-e", "RUST_LOG=info",
		image,
	}
	out := runCommand(t, trace, ctx, repoRoot, "docker", args...)
	containerID := strings.TrimSpace(string(out))
	if containerID == "" {
		t.Fatalf("docker run returned empty container id")
	}
	trace.Logf("docker", "container started id=%s", containerID)
	stopLogStream := streamDockerLogs(t, trace, containerID)

	var stopped atomic.Bool
	stop := func() {
		if stopped.Swap(true) {
			return
		}
		trace.Logf("docker", "stopping container id=%s", containerID)
		_ = exec.Command("docker", "rm", "-f", containerID).Run()
		stopLogStream()
		if removeImage {
			trace.Logf("docker", "removing e2e image=%s", image)
			_ = exec.Command("docker", "rmi", "-f", image).Run()
		}
	}
	t.Cleanup(stop)

	baseURL := "http://127.0.0.1:" + port
	deadline := time.Now().Add(90 * time.Second)
	lastLog := time.Time{}
	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/healthz", nil)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				trace.Logf("runtime", "healthz ok base_url=%s", baseURL)
				return baseURL, stop
			}
		}
		if time.Since(lastLog) > 2*time.Second {
			trace.Logf("runtime", "waiting for healthz base_url=%s err=%v", baseURL, err)
			lastLog = time.Now()
		}
		time.Sleep(250 * time.Millisecond)
	}
	logs := dockerLogs(containerID)
	stop()
	t.Fatalf("runtime container did not become healthy:\n%s", tailString(logs, 12000))
	return "", nil
}

var agentRuntimeImageCache struct {
	sync.Mutex
	image string
}

func agentRuntimeImage(t *testing.T, trace *agentRuntimeE2ETrace, ctx context.Context, repoRoot string) (string, bool) {
	t.Helper()
	if image := strings.TrimSpace(os.Getenv("HIVY_AGENT_RUNTIME_E2E_IMAGE")); image != "" {
		trace.Logf("docker", "using prebuilt image from HIVY_AGENT_RUNTIME_E2E_IMAGE=%s", image)
		return image, false
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("docker is required for this E2E: %v", err)
	}
	agentRuntimeImageCache.Lock()
	defer agentRuntimeImageCache.Unlock()
	if agentRuntimeImageCache.image != "" {
		trace.Logf("docker", "reusing runtime image=%s", agentRuntimeImageCache.image)
		return agentRuntimeImageCache.image, false
	}
	image := "hivy-agent-runtime-e2e:" + strings.ToLower(strings.ReplaceAll(uuid.NewString(), "-", ""))
	trace.Logf("docker", "building runtime image=%s", image)
	runCommand(t, trace, ctx, repoRoot, "make", "sandbox-runtime-image", "SANDBOX_RUNTIME_IMAGE="+image)
	agentRuntimeImageCache.image = image
	return image, false
}

func allocateLocalPort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate runtime port: %v", err)
	}
	defer listener.Close()
	return fmt.Sprintf("%d", listener.Addr().(*net.TCPAddr).Port)
}

func runCommand(t *testing.T, trace *agentRuntimeE2ETrace, ctx context.Context, dir, name string, args ...string) []byte {
	t.Helper()
	trace.Logf("command", "start dir=%s cmd=%s %s", dir, name, strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("%s stdout pipe: %v", name, err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("%s stderr pipe: %v", name, err)
	}
	var out bytes.Buffer
	var outMu sync.Mutex
	if err := cmd.Start(); err != nil {
		t.Fatalf("%s %s start failed: %v", name, strings.Join(args, " "), err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go scanCommandOutput(trace, &wg, &outMu, &out, name, "stdout", stdout)
	go scanCommandOutput(trace, &wg, &outMu, &out, name, "stderr", stderr)
	err = cmd.Wait()
	wg.Wait()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, tailString(out.String(), 12000))
	}
	trace.Logf("command", "complete cmd=%s %s bytes=%d", name, strings.Join(args, " "), out.Len())
	return out.Bytes()
}

func scanCommandOutput(trace *agentRuntimeE2ETrace, wg *sync.WaitGroup, outMu *sync.Mutex, out *bytes.Buffer, command, stream string, reader io.Reader) {
	defer wg.Done()
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		outMu.Lock()
		out.WriteString(line)
		out.WriteByte('\n')
		outMu.Unlock()
		trace.Logf("command", "%s %s %s", command, stream, line)
	}
	if err := scanner.Err(); err != nil {
		trace.Logf("command", "%s %s scanner error: %v", command, stream, err)
	}
}

func dockerLogs(containerID string) string {
	cmd := exec.Command("docker", "logs", containerID)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out) + "\n" + err.Error()
	}
	return string(out)
}

func repoRootFromE2E(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if filepath.Base(wd) == "e2e" {
		return filepath.Dir(wd)
	}
	return wd
}

func doRuntimeJSON(t *testing.T, trace *agentRuntimeE2ETrace, ctx context.Context, method, url, token string, body []byte, wantStatus int) []byte {
	t.Helper()
	trace.Body("runtime-http", method+" "+url+" request", body)
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new runtime request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s failed: %v", method, url, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	trace.Body("runtime-http", fmt.Sprintf("%s %s response status=%d", method, url, resp.StatusCode), respBody)
	if resp.StatusCode != wantStatus {
		t.Fatalf("%s %s status=%d want=%d body=%s", method, url, resp.StatusCode, wantStatus, respBody)
	}
	return respBody
}

func waitForRuntimeReady(t *testing.T, trace *agentRuntimeE2ETrace, ctx context.Context, baseURL, token string) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	lastLog := time.Time{}
	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/readyz", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				trace.Logf("runtime", "readyz ok base_url=%s", baseURL)
				return
			}
		}
		if time.Since(lastLog) > 2*time.Second {
			trace.Logf("runtime", "waiting for readyz base_url=%s err=%v", baseURL, err)
			lastLog = time.Now()
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("runtime did not become ready")
}
