package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type agentRuntimeE2EScenario struct {
	trace         *agentRuntimeE2ETrace
	ctx           context.Context
	repoRoot      string
	workspaceRoot string
	runtimeSecret string
	agentID       string
	sandboxID     string
	baseURL       string
	stopRuntime   func()
	proxy         *agentRuntimeModelProxy
	controlPlane  *agentRuntimeMockControlPlane
}

type agentRuntimeE2EScenarioOptions struct {
	name          string
	workspaceRoot string
	dbPath        string
	schedules     []any
}

func startAgentRuntimeE2EScenario(
	t *testing.T,
	trace *agentRuntimeE2ETrace,
	ctx context.Context,
	opts agentRuntimeE2EScenarioOptions,
	buildDefinition func(proxyURL, controlPlaneURL, sandboxID string) map[string]any,
) *agentRuntimeE2EScenario {
	t.Helper()
	loadEnv(t)
	systemModelKey := strings.TrimSpace(os.Getenv("HIVY_SYSTEM_OPENROUTER_API_KEY"))
	if systemModelKey == "" {
		t.Skip("HIVY_SYSTEM_OPENROUTER_API_KEY is not configured")
	}
	repoRoot := repoRootFromE2E(t)
	workspaceRoot := opts.workspaceRoot
	if workspaceRoot == "" {
		workspaceRoot = t.TempDir()
	}
	runtimeSecret := "agent-runtime-e2e-secret" // #nosec G101 -- fixed local E2E runtime secret.
	agentID := uuid.NewString()
	sandboxID := uuid.NewString()
	controlPlane := newAgentRuntimeMockControlPlane(t, trace, runtimeSecret, agentID, sandboxID)
	proxy := newAgentRuntimeModelProxy(t, trace, systemModelKey, agentRuntimeProxyToken)
	t.Cleanup(controlPlane.server.Close)
	t.Cleanup(proxy.server.Close)
	baseURL, stopRuntime := startAgentRuntimeContainerWithOptions(
		t,
		trace,
		ctx,
		repoRoot,
		workspaceRoot,
		runtimeSecret,
		agentRuntimeProxyToken,
		agentID,
		sandboxID,
		controlPlane.containerURL,
		agentRuntimeContainerOptions{dbPath: opts.dbPath},
	)
	definition := buildDefinition(containerURLForServer(proxy.server.URL), controlPlane.containerURL, sandboxID)
	payload := agentRuntimeConfigPayload(runtimeSecret, controlPlane.containerURL, agentID, sandboxID, definition, opts.schedules)
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal %s config: %v", opts.name, err)
	}
	trace.Body("config", opts.name+" runtime /config request", body)
	doRuntimeJSON(t, trace, ctx, http.MethodPut, baseURL+"/config", runtimeSecret, body, http.StatusOK)
	waitForRuntimeReady(t, trace, ctx, baseURL, runtimeSecret)
	return &agentRuntimeE2EScenario{
		trace:         trace,
		ctx:           ctx,
		repoRoot:      repoRoot,
		workspaceRoot: workspaceRoot,
		runtimeSecret: runtimeSecret,
		agentID:       agentID,
		sandboxID:     sandboxID,
		baseURL:       baseURL,
		stopRuntime:   stopRuntime,
		proxy:         proxy,
		controlPlane:  controlPlane,
	}
}

func agentRuntimeConfigPayload(runtimeSecret, controlPlaneURL, agentID, sandboxID string, definition map[string]any, schedules []any) map[string]any {
	if schedules == nil {
		schedules = []any{}
	}
	return map[string]any{
		"runtime_secret": runtimeSecret,
		"runtime_env": map[string]string{
			"HIVY_RUNTIME_SECRET":           runtimeSecret,
			"HIVY_CONTROL_PLANE_URL":        controlPlaneURL,
			"HIVY_DRIVE_UPLOAD_URL":         controlPlaneURL + "/internal/agents/" + agentID + "/drive",
			"HIVY_DRIVE_UPLOAD_BEARER":      runtimeSecret,
			"HIVY_AGENT_ID":                 agentID,
			"HIVY_ORG_ID":                   uuid.NewString(),
			"HIVY_SANDBOX_ID":               sandboxID,
			"HIVY_DB_SYNC_ENABLED":          "true",
			"HIVY_DB_SYNC_WRITE_THRESHOLD":  "1",
			"HIVY_DB_SYNC_INTERVAL_SECONDS": "1",
			"HIVY_PROXY_API_KEY":            agentRuntimeProxyToken,
			"HIVY_MCP_SECURE_TOKEN":         "secure-mcp-token",
		},
		"definition": definition,
		"schedules":  schedules,
	}
}

func agentRuntimeE2EContext(t *testing.T, timeout time.Duration) (context.Context, context.CancelFunc, *agentRuntimeE2ETrace) {
	t.Helper()
	if os.Getenv("HIVY_AGENT_RUNTIME_E2E") != "1" {
		t.Skip("set HIVY_AGENT_RUNTIME_E2E=1 to run the real runtime container E2E")
	}
	requireAgentRuntimeE2EVerbose(t)
	trace := newAgentRuntimeE2ETrace(t)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	return ctx, cancel, trace
}
