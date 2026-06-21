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
	agentRuntimeE2EToken      = "HIVY_AGENT_RUNTIME_E2E_TOKEN_7d4f1c" // #nosec G101 -- fixed local E2E sentinel token.
	agentRuntimeProxyToken    = "ptok_agent_runtime_e2e_proxy_7d4f1c" // #nosec G101 -- fixed local E2E sentinel token.
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

	runtimeSecret := "agent-runtime-e2e-secret" // #nosec G101 -- fixed local E2E runtime secret.
	agentID := uuid.NewString()
	sandboxID := uuid.NewString()
	controlPlane := newAgentRuntimeMockControlPlane(t, trace, runtimeSecret, agentID, sandboxID)
	defer controlPlane.server.Close()
	trace.Logf("ids", "agent_id=%s sandbox_id=%s", agentID, sandboxID)

	runtimeBaseURL, stopRuntime := startAgentRuntimeContainerWithOptions(
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
		agentRuntimeContainerOptions{developerImage: true},
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
	requiredFinalText := append([]string{"E2E_PASS", agentRuntimeE2EToken, "REAL_REPOS_CONFIRMED", "FFF_TOOLS_CONFIRMED", "APPLY_PATCH_CONFIRMED", "LSP_CONFIRMED", "ALL_LSP_SERVERS_CONFIRMED"}, agentRuntimeE2EAllSubagentMarkers()...)
	assertRuntimeSessionFinal(t, trace, directSessionEvents, requiredFinalText)
	assertAgentRuntimePostRunAPIs(t, trace, ctx, runtimeBaseURL, runtimeSecret, messageResponse.SessionID, messageResponse.TraceID, workspaceRoot)
	assertFixtureProjectCompleted(t, trace, workspaceRoot)
	if got := fixture.calls.Load(); got < 1 {
		t.Fatalf("fixture tool was not called")
	} else {
		trace.Logf("assert", "fixture tool calls=%d", got)
	}
	proxy.assertUsed(t)
	proxy.assertOpenWeightPayloads(t)
	controlPlane.waitForActivity(t)
	controlPlane.assertAgentActivity(t)
	controlPlane.waitForBatchActivity(t)
	trace.Logf("done", "flagship runtime E2E completed")
}

func agentRuntimeCodingRequest() map[string]any {
	codebaseMarker := agentRuntimeE2ESubagentMarkers["codebase-explorer"]
	librarianMarker := agentRuntimeE2ESubagentMarkers["librarian"]
	oracleMarker := agentRuntimeE2ESubagentMarkers["oracle"]
	return map[string]any{
		"session_id": "agent-runtime-coding-e2e",
		"user":       "agent-runtime-e2e",
		"text": strings.Join([]string{
			"You are running the flagship Hivy agent runtime E2E as Hakaree. This is a test session: every agent and subagent must minimize token use, avoid real exploratory work beyond these steps, and use the exact named tools.",
			"1. Call search_sessions with query agent-runtime-e2e.",
			"2. Call fixture_requirements to retrieve the exact token and requirement phrases.",
			"3. Use bash to clone two real public codebases into repos/: run mkdir -p repos && git clone --depth=1 https://github.com/tidwall/gjson repos/gjson && git clone --depth=1 https://github.com/pallets/itsdangerous repos/itsdangerous.",
			"4. Call file_search with query serializer.py and path repos/itsdangerous.",
			"5. Call glob with pattern **/*.go and path repos/gjson.",
			"6. Call grep with pattern func Get, path repos/gjson, and include **/*.go.",
			"7. Call multi_grep with patterns [\"func Get\", \"class Serializer\"], path repos, and include **/*.{go,py}.",
			"8. Call lsp with operation diagnostics separately for every file in this exact list to verify every built-in LSP server: lsp-fixtures/deno/mod.ts, lsp-fixtures/typescript/src/app.ts, lsp-fixtures/python/app.py, lsp-fixtures/go/main.go, lsp-fixtures/rust/src/lib.rs, lsp-fixtures/cpp/main.cpp, lsp-fixtures/json/package.json, lsp-fixtures/yaml/config.yaml, lsp-fixtures/html/index.html, lsp-fixtures/css/styles.css, lsp-fixtures/tailwind/src/App.tsx, lsp-fixtures/bash/script.sh, lsp-fixtures/docker/Dockerfile.",
			"9. Call lsp with operation documentSymbol for filePath repos/gjson/gjson.go.",
			"10. Call lsp with operation diagnostics for filePath calc.py.",
			"11. Read calc.py with read_file.",
			"12. Use apply_patch to add TOOLING_E2E.md containing the retrieved token and the phrases REAL_REPOS_CONFIRMED, FFF_TOOLS_CONFIRMED, APPLY_PATCH_CONFIRMED, LSP_CONFIRMED, and ALL_LSP_SERVERS_CONFIRMED. The patch argument must follow this exact shape: *** Begin Patch\\n*** Add File: TOOLING_E2E.md\\n+token: <retrieved token>\\n+REAL_REPOS_CONFIRMED\\n+FFF_TOOLS_CONFIRMED\\n+APPLY_PATCH_CONFIRMED\\n+LSP_CONFIRMED\\n+ALL_LSP_SERVERS_CONFIRMED\\n*** End Patch",
			"13. Use write_file to create e2e_notes.txt containing the retrieved token.",
			"14. Use edit_file on calc.py with two replacements: replace PLACEHOLDER_TOKEN with the retrieved token, and replace PLACEHOLDER_HELPER with " + codebaseMarker + ".",
			"15. Use bash to run a background command: python3 -c 'print(\"background-ok\")' with run_in_background=true, then call check_bash_status with its process_id. If check_bash_status returns next_cursor, include that cursor if you check again.",
			"16. Dispatch the three configured Hakaree subagents in parallel. In one assistant tool-call batch, call subagent_task three times and use the returned completed tool results directly. Use agent codebase-explorer with goal: FLAGSHIP_E2E_TEST_SESSION: do exactly 2-3 cheap local tool calls, specifically read_file calc.py, glob pattern **/*.py path ., and grep pattern runtime_token path . include **/*.py, then final exactly " + codebaseMarker + ". Use agent librarian with goal: FLAGSHIP_E2E_TEST_SESSION: do exactly 2-3 cheap local tool calls, specifically read_file test_calc.py, file_search query calc.py path ., and glob pattern lsp-fixtures/**/*.json path ., then final exactly " + librarianMarker + ". Use agent oracle with goal: FLAGSHIP_E2E_TEST_SESSION: do exactly 2-3 cheap local tool calls, specifically read_file calc.py, multi_grep patterns [\"runtime_token\", \"helper_phrase\"] path . include **/*.py, and lsp diagnostics filePath calc.py, then final exactly " + oracleMarker + ".",
			"17. Do not call any subagent status tool. Each subagent_task result must contain its marker before you continue.",
			"18. Use bash to run python3 -m unittest -v.",
			"19. After the tests pass and all three subagent_task results are available, final answer must include E2E_PASS, the exact token, REAL_REPOS_CONFIRMED, FFF_TOOLS_CONFIRMED, APPLY_PATCH_CONFIRMED, LSP_CONFIRMED, ALL_LSP_SERVERS_CONFIRMED, and " + codebaseMarker + ", " + librarianMarker + ", " + oracleMarker + ".",
		}, "\n"),
		"raw": map[string]any{"source": "session", "test": "agent-runtime-coding-e2e"},
	}
}
