package microsandbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/sandbox"
)

func TestDriverCreateWarmSlotCreatesRunningRuntimeEndpoint(t *testing.T) {
	var createReq map[string]any
	var runtimeReq map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sandboxes":
			if err := json.NewDecoder(r.Body).Decode(&createReq); err != nil {
				t.Fatalf("decode create request: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ID": "sbx_warm", "Status": "running"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sandboxes/sbx_warm/runtime-endpoints":
			if err := json.NewDecoder(r.Body).Decode(&runtimeReq); err != nil {
				t.Fatalf("decode runtime request: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"url": "https://7080-sbx_warm.preview.test"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	driver, err := NewDriver(Config{
		ControlURL:   server.URL,
		APIToken:     "test-token",
		RuntimeImage: "fallback:latest",
	})
	if err != nil {
		t.Fatalf("NewDriver: %v", err)
	}
	if !driver.UsesWarmPool() {
		t.Fatalf("UsesWarmPool = false, want true")
	}
	info, err := driver.CreateWarmSlot(context.Background(), sandbox.WarmSlotCreateOpts{
		Name:          "hivy-agent-default-small-warm-test",
		Mode:          "agent",
		ImageKind:     "default",
		SandboxSize:   "small",
		RuntimeImage:  "ghcr.io/usehivy/hivy-sandboxes-runtime:v3.4.0-amd64",
		RuntimePort:   sandbox.AgentSandboxPort,
		CPU:           1,
		Memory:        2,
		Disk:          10,
		RuntimeSecret: "runtime-secret",
		EnvVars:       map[string]string{"SENTRY_DSN": "dsn"},
	})
	if err != nil {
		t.Fatalf("CreateWarmSlot: %v", err)
	}
	if info.ExternalID != "sbx_warm" || info.EndpointURL != "https://7080-sbx_warm.preview.test" {
		t.Fatalf("warm slot info = %+v", info)
	}
	if createReq["org_id"] != "warm-pool" {
		t.Fatalf("org_id = %v, want warm-pool", createReq["org_id"])
	}
	if createReq["image_ref"] != "ghcr.io/usehivy/hivy-sandboxes-runtime:v3.4.0-amd64" {
		t.Fatalf("image_ref = %v", createReq["image_ref"])
	}
	assertRuntimeHealthCheck(t, createReq, sandbox.AgentSandboxPort)
	if createReq["cpu"] != float64(1) || createReq["memory_mb"] != float64(2048) || createReq["disk_gb"] != float64(10) {
		t.Fatalf("resources = cpu:%v memory:%v disk:%v, want 1/2048/10", createReq["cpu"], createReq["memory_mb"], createReq["disk_gb"])
	}
	env, ok := createReq["env"].(map[string]any)
	if !ok {
		t.Fatalf("env = %T, want object", createReq["env"])
	}
	if env["HIVY_RUNTIME_SECRET"] != "runtime-secret" || env["HIVY_RUNTIME_BIND_ADDR"] != "0.0.0.0:7080" {
		t.Fatalf("runtime env = %#v", env)
	}
	if env[agentruntime.AgentEnvDBPath] != agentruntime.AgentRuntimeDBPath {
		t.Fatalf("runtime db path = %v, want %s", env[agentruntime.AgentEnvDBPath], agentruntime.AgentRuntimeDBPath)
	}
	metadata, ok := createReq["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata = %T, want object", createReq["metadata"])
	}
	if metadata["harness"] != "agent-sandbox-warm-pool" || metadata["sandbox_image"] != "default" || metadata["sandbox_size"] != "small" {
		t.Fatalf("metadata = %#v", metadata)
	}
	if runtimeReq["port"] != float64(sandbox.AgentSandboxPort) {
		t.Fatalf("runtime port = %v, want %d", runtimeReq["port"], sandbox.AgentSandboxPort)
	}
}
