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

const (
	agentRuntimeE2EToken      = "HIVY_AGENT_RUNTIME_E2E_TOKEN_7d4f1c"
	agentRuntimeProxyToken    = "ptok_agent_runtime_e2e_proxy_7d4f1c"
	agentRuntimeContainerPort = "7080"
)

func TestAgentRuntimeCodingTaskE2E(t *testing.T) {
	if os.Getenv("HIVY_AGENT_RUNTIME_E2E") != "1" {
		t.Skip("set HIVY_AGENT_RUNTIME_E2E=1 to run the real runtime container E2E")
	}
	requireAgentRuntimeE2EVerbose(t)
	trace := newAgentRuntimeE2ETrace(t)
	trace.Logf("start", "flagship runtime E2E starting")
	loadEnv(t)
	trace.Logf("env", "loaded .env and process environment")

	systemModelKey := strings.TrimSpace(os.Getenv("HIVY_SYSTEM_OPENROUTER_API_KEY"))
	if systemModelKey == "" {
		t.Skip("HIVY_SYSTEM_OPENROUTER_API_KEY is not configured")
	}
	trace.Logf("env", "host model key is present and will only be used by the test proxy")

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Minute)
	defer cancel()

	repoRoot := repoRootFromE2E(t)
	workspaceRoot := t.TempDir()
	trace.Logf("paths", "repo_root=%s workspace_root=%s", repoRoot, workspaceRoot)
	writeFixtureProject(t, trace, workspaceRoot)

	fixture := newAgentRuntimeFixtureMCP(t, trace)
	defer fixture.server.Close()

	proxy := newAgentRuntimeModelProxy(t, trace, systemModelKey, agentRuntimeProxyToken)
	defer proxy.server.Close()

	runtimeSecret := "agent-runtime-e2e-secret"
	agentID := uuid.NewString()
	sandboxID := uuid.NewString()
	controlPlane := newAgentRuntimeMockControlPlane(t, trace, runtimeSecret, agentID, sandboxID)
	defer controlPlane.server.Close()
	trace.Logf("ids", "agent_id=%s sandbox_id=%s", agentID, sandboxID)

	runtimeBaseURL, stopRuntime := startAgentRuntimeContainer(
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
	)
	defer stopRuntime()
	trace.Logf("runtime", "container API is available at %s", runtimeBaseURL)
	assertDirectStreamDisabledBeforeConfig(t, trace, ctx, runtimeBaseURL)

	definition := agentRuntimeE2EDefinition(
		t,
		trace,
		containerURLForServer(fixture.server.URL),
		containerURLForServer(proxy.server.URL),
		controlPlane.containerURL,
		sandboxID,
	)
	configPayload := map[string]any{
		"runtime_secret": runtimeSecret,
		"runtime_env": map[string]string{
			"HIVY_RUNTIME_SECRET":           runtimeSecret,
			"HIVY_CONTROL_PLANE_URL":        controlPlane.containerURL,
			"HIVY_AGENT_ID":                 agentID,
			"HIVY_ORG_ID":                   uuid.NewString(),
			"HIVY_SANDBOX_ID":               sandboxID,
			"HIVY_DB_SYNC_ENABLED":          "true",
			"HIVY_DB_SYNC_WRITE_THRESHOLD":  "1",
			"HIVY_DB_SYNC_INTERVAL_SECONDS": "1",
			"HIVY_PROXY_API_KEY":            agentRuntimeProxyToken,
		},
		"definition": definition,
		"schedules":  []any{},
	}
	body, err := json.Marshal(configPayload)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	trace.Body("config", "runtime /config request", body)

	doRuntimeJSON(t, trace, ctx, http.MethodPut, runtimeBaseURL+"/config", runtimeSecret, body, http.StatusOK)
	waitForRuntimeReady(t, trace, ctx, runtimeBaseURL, runtimeSecret)

	messageResponse := sendAgentRuntimeMessage(t, trace, ctx, runtimeBaseURL, runtimeSecret, agentRuntimeCodingRequest())

	directSessionStream := readDirectRuntimeSSEAsync(
		trace,
		ctx,
		directRuntimeStreamURL(t, runtimeBaseURL, messageResponse.StreamURL),
		directRuntimeJWT(t, runtimeSecret, messageResponse.SessionID, sandboxID, "stream:read"),
	)
	events := readRuntimeSSE(t, trace, ctx, runtimeBaseURL+messageResponse.StreamURL, runtimeSecret, nil)
	directSessionEvents := directSessionStream.wait(t)
	assertRuntimeSharedSubagentStream(t, trace, "bearer parent stream", events)
	assertRuntimeSharedSubagentStream(t, trace, "direct browser stream", directSessionEvents)
	assertRuntimeE2EEvents(t, trace, events)
	assertRuntimeSessionFinal(t, trace, directSessionEvents, []string{"E2E_PASS", agentRuntimeE2EToken})
	assertAgentRuntimePostRunAPIs(t, trace, ctx, runtimeBaseURL, runtimeSecret, messageResponse.SessionID, messageResponse.TraceID, workspaceRoot)
	assertFixtureProjectCompleted(t, trace, workspaceRoot)
	if got := fixture.calls.Load(); got < 1 {
		t.Fatalf("fixture tool was not called")
	} else {
		trace.Logf("assert", "fixture tool calls=%d", got)
	}
	proxy.assertUsed(t)
	controlPlane.waitForActivity(t)
	controlPlane.assertAgentActivity(t)
	controlPlane.waitForBatchActivity(t)
	trace.Logf("done", "flagship runtime E2E completed")
}

func agentRuntimeCodingRequest() map[string]any {
	return map[string]any{
		"session_id": "agent-runtime-coding-e2e",
		"user":       "agent-runtime-e2e",
		"text": strings.Join([]string{
			"You are running the flagship Hivy agent runtime E2E. Complete every numbered step and use the exact named tools.",
			"1. Call search_sessions with query agent-runtime-e2e.",
			"2. Call fixture_requirements to retrieve the exact token and requirement phrases.",
			"3. Read calc.py with read_file.",
			"4. Use write_file to create e2e_notes.txt containing the retrieved token.",
			"5. Use edit_file on calc.py with two replacements: replace PLACEHOLDER_TOKEN with the retrieved token, and replace PLACEHOLDER_HELPER with PLANNER_SUBAGENT_CONFIRMED.",
			"6. Use bash to run a background command: python3 -c 'print(\"background-ok\")' with run_in_background=true, then call check_bash_status with its process_id.",
			"7. Use subagent_task once for each configured agent: planner, qa, and reviewer. Use each returned job_id with check_subagent_task_status exactly once.",
			"8. Use bash to run python3 -m unittest -v.",
			"9. After the tests pass and all three subagent completion notifications arrive, final answer must include E2E_PASS, the exact token, and PLANNER_SUBAGENT_CONFIRMED, QA_SUBAGENT_CONFIRMED, REVIEW_SUBAGENT_CONFIRMED.",
		}, "\n"),
		"raw": map[string]any{"source": "session", "test": "agent-runtime-coding-e2e"},
	}
}
