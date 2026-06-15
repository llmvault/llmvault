package microsandbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	msbapi "github.com/usehivy/hivy/internal/microsandbox/api"
	"github.com/usehivy/hivy/internal/sandbox"
)

func TestDriverCreateSandboxAndRuntimeEndpoint(t *testing.T) {
	var createReq map[string]any
	var runtimeReq map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization = %q, want bearer token", got)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sandboxes":
			if err := json.NewDecoder(r.Body).Decode(&createReq); err != nil {
				t.Fatalf("decode create request: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ID": "sbx_test", "Status": "running"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sandboxes/sbx_test/runtime-endpoints":
			if err := json.NewDecoder(r.Body).Decode(&runtimeReq); err != nil {
				t.Fatalf("decode runtime request: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"url": "https://7080-sbx_test.preview.test?rt=signed"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	driver, err := NewDriver(Config{
		ControlURL:   server.URL,
		APIToken:     "test-token",
		RuntimeImage: "ghcr.io/usehivy/hivy-sandboxes-runtime:latest",
	})
	if err != nil {
		t.Fatalf("NewDriver: %v", err)
	}

	info, err := driver.CreateSandbox(context.Background(), sandbox.CreateSandboxOpts{
		Name:        "agent-test",
		TemplateRef: "snp_template",
		Labels:      map[string]string{"org_id": "org_123", "agent_id": "emp_123"},
		EnvVars:     map[string]string{"HIVY_RUNTIME_SECRET": "secret"},
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	if info.ExternalID != "sbx_test" || info.Status != sandbox.StatusRunning {
		t.Fatalf("CreateSandbox returned %+v", info)
	}
	if createReq["org_id"] != "org_123" {
		t.Fatalf("org_id = %v, want org_123", createReq["org_id"])
	}
	if createReq["snapshot_id"] != "snp_template" {
		t.Fatalf("snapshot_id = %v, want snp_template", createReq["snapshot_id"])
	}
	if createReq["image_ref"] != "ghcr.io/usehivy/hivy-sandboxes-runtime:latest" {
		t.Fatalf("image_ref = %v", createReq["image_ref"])
	}
	if createReq["size"] != msbapi.DefaultSize {
		t.Fatalf("size = %v, want %s", createReq["size"], msbapi.DefaultSize)
	}
	previewPorts, ok := createReq["preview_ports"].([]any)
	if !ok {
		t.Fatalf("preview_ports = %T, want array", createReq["preview_ports"])
	}
	if len(previewPorts) != len(msbapi.DefaultPreviewPorts()) {
		t.Fatalf("preview_ports length = %d, want %d", len(previewPorts), len(msbapi.DefaultPreviewPorts()))
	}

	endpoint, err := driver.GetEndpoint(context.Background(), "sbx_test", 0)
	if err != nil {
		t.Fatalf("GetEndpoint: %v", err)
	}
	if endpoint != "https://7080-sbx_test.preview.test?rt=signed" {
		t.Fatalf("endpoint = %q", endpoint)
	}
	if runtimeReq["port"] != float64(sandbox.AgentSandboxPort) {
		t.Fatalf("runtime port = %v, want %d", runtimeReq["port"], sandbox.AgentSandboxPort)
	}
	if runtimeReq["ttl_seconds"] != float64(defaultRuntimeEndpointTTLSeconds) {
		t.Fatalf("runtime ttl = %v, want %d", runtimeReq["ttl_seconds"], defaultRuntimeEndpointTTLSeconds)
	}
}

func TestDriverLifecycleStatusExecAndTemplate(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/v1/sandboxes/sbx_test/start", "/v1/sandboxes/sbx_test/stop":
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		case "/v1/sandboxes/sbx_test":
			if r.Method != http.MethodGet {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ID": "sbx_test", "Status": "stopped"})
		case "/v1/sandboxes/sbx_test/exec":
			_ = json.NewEncoder(w).Encode(map[string]any{"stdout": "ok\n", "stderr": "", "exit_code": 0})
		case "/v1/snapshots":
			_ = json.NewEncoder(w).Encode(map[string]any{"ID": "snp_test", "Status": "ready"})
		case "/v1/snapshots/snp_test":
			_ = json.NewEncoder(w).Encode(map[string]any{"ID": "snp_test", "Status": "ready", "Logs": "built"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	driver, err := NewDriver(Config{ControlURL: server.URL, APIToken: "test-token", RuntimeImage: "runtime:latest"})
	if err != nil {
		t.Fatalf("NewDriver: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := driver.Validate(ctx); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if err := driver.StartSandbox(ctx, "sbx_test"); err != nil {
		t.Fatalf("StartSandbox: %v", err)
	}
	if err := driver.StopSandbox(ctx, "sbx_test"); err != nil {
		t.Fatalf("StopSandbox: %v", err)
	}
	status, err := driver.GetStatus(ctx, "sbx_test")
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if status != sandbox.StatusStopped {
		t.Fatalf("status = %q, want stopped", status)
	}
	out, err := driver.ExecuteCommand(ctx, "sbx_test", "echo ok")
	if err != nil {
		t.Fatalf("ExecuteCommand: %v", err)
	}
	if out != "ok\n" {
		t.Fatalf("command output = %q", out)
	}
	templateID, err := driver.BuildTemplate(ctx, sandbox.TemplateBuildRequest{Name: "react", BuildCommands: []string{"npm install"}})
	if err != nil {
		t.Fatalf("BuildTemplate: %v", err)
	}
	if templateID != "snp_test" {
		t.Fatalf("template id = %q, want snp_test", templateID)
	}
	logs, err := driver.GetTemplateLogs(ctx, "snp_test")
	if err != nil {
		t.Fatalf("GetTemplateLogs: %v", err)
	}
	if logs != "built" {
		t.Fatalf("logs = %q, want built", logs)
	}
}
