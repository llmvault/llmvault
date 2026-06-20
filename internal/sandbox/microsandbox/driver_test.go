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
			_ = json.NewEncoder(w).Encode(map[string]string{"url": "https://7080-sbx_test.preview.test"})
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
		TemplateRef: "ghcr.io/usehivy/hivy-sandboxes-runtime:latest",
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
	if _, ok := createReq["snapshot_id"]; ok {
		t.Fatalf("snapshot_id must not be sent: %#v", createReq)
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
	init, ok := createReq["init"].(map[string]any)
	if !ok {
		t.Fatalf("init = %T, want object", createReq["init"])
	}
	if init["cmd"] != "/usr/local/bin/hivy-runtime-entrypoint" {
		t.Fatalf("init cmd = %v", init["cmd"])
	}
	args, ok := init["args"].([]any)
	if !ok {
		t.Fatalf("init args = %T, want array", init["args"])
	}
	if len(args) != 1 || args[0] != "/usr/local/bin/hivy-sandboxes-runtime" {
		t.Fatalf("init args = %v", args)
	}

	endpoint, err := driver.GetEndpoint(context.Background(), "sbx_test", 0)
	if err != nil {
		t.Fatalf("GetEndpoint: %v", err)
	}
	if endpoint != "https://7080-sbx_test.preview.test" {
		t.Fatalf("endpoint = %q", endpoint)
	}
	if runtimeReq["port"] != float64(sandbox.AgentSandboxPort) {
		t.Fatalf("runtime port = %v, want %d", runtimeReq["port"], sandbox.AgentSandboxPort)
	}
	if _, ok := runtimeReq["ttl_seconds"]; ok {
		t.Fatalf("runtime endpoint request must not send ttl_seconds: %#v", runtimeReq)
	}
}

func TestDriverCreateSandboxUsesTemplateIDForMicrosandboxTemplate(t *testing.T) {
	var createReq map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sandboxes" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&createReq); err != nil {
			t.Fatalf("decode create request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ID": "sbx_template", "Status": "running"})
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
	if _, err := driver.CreateSandbox(context.Background(), sandbox.CreateSandboxOpts{
		Name:        "agent-template",
		TemplateRef: "tpl_abc12345",
		Labels:      map[string]string{"org_id": "org_123"},
	}); err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	if createReq["template_id"] != "tpl_abc12345" {
		t.Fatalf("template_id = %v, want tpl_abc12345", createReq["template_id"])
	}
	if _, ok := createReq["snapshot_id"]; ok {
		t.Fatalf("snapshot_id must not be sent: %#v", createReq)
	}
	if createReq["image_ref"] != "" {
		t.Fatalf("image_ref = %v, want empty", createReq["image_ref"])
	}
}

func TestDriverCreateSandboxUsesConfiguredPreviewPortsWithRuntimePort(t *testing.T) {
	var createReq map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sandboxes" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&createReq); err != nil {
			t.Fatalf("decode create request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ID": "sbx_ports", "Status": "running"})
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
	if _, err := driver.CreateSandbox(context.Background(), sandbox.CreateSandboxOpts{
		Name:         "agent-ports",
		TemplateRef:  "ghcr.io/usehivy/hivy-sandboxes-runtime:latest",
		Labels:       map[string]string{"org_id": "org_123"},
		ExposedPorts: []int{5173, 3000, 5173},
	}); err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	got := jsonNumberArray(t, createReq["preview_ports"])
	want := []int{5173, 3000, sandbox.AgentSandboxPort}
	if len(got) != len(want) {
		t.Fatalf("preview_ports = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("preview_ports = %v, want %v", got, want)
		}
	}
}

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
		case "/v1/templates":
			w.Header().Set("Content-Type", "application/x-ndjson")
			_, _ = w.Write([]byte(`{"type":"log","id":"tpl_test","message":"built"}` + "\n"))
			_, _ = w.Write([]byte(`{"type":"result","id":"tpl_test","status":"ready"}` + "\n"))
		case "/v1/templates/tpl_test":
			_ = json.NewEncoder(w).Encode(map[string]any{"ID": "tpl_test", "Status": "ready", "Logs": "built\n"})
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
	if templateID != "tpl_test" {
		t.Fatalf("template id = %q, want tpl_test", templateID)
	}
	logs, err := driver.GetTemplateLogs(ctx, "tpl_test")
	if err != nil {
		t.Fatalf("GetTemplateLogs: %v", err)
	}
	if logs != "built\n" {
		t.Fatalf("logs = %q, want built", logs)
	}
}

func jsonNumberArray(t *testing.T, value any) []int {
	t.Helper()
	raw, ok := value.([]any)
	if !ok {
		t.Fatalf("value = %T, want array", value)
	}
	out := make([]int, 0, len(raw))
	for _, item := range raw {
		number, ok := item.(float64)
		if !ok {
			t.Fatalf("array item = %T, want number", item)
		}
		out = append(out, int(number))
	}
	return out
}
