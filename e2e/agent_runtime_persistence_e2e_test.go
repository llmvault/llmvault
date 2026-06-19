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

func TestAgentRuntimePersistenceE2E(t *testing.T) {
	ctx, cancel, trace := agentRuntimeE2EContext(t, 35*time.Minute)
	defer cancel()
	trace.Logf("start", "persistence runtime E2E starting")
	workspaceRoot := t.TempDir()

	first := startAgentRuntimeE2EScenario(
		t,
		trace,
		ctx,
		agentRuntimeE2EScenarioOptions{name: "persistence-first", workspaceRoot: workspaceRoot, dbPath: "/workspace/runtime.db"},
		func(proxyURL, controlPlaneURL, sandboxID string) map[string]any {
			return persistenceDefinition(t, proxyURL, controlPlaneURL, sandboxID)
		},
	)
	firstResponse, firstEvents, firstResponseEvents := runAgentRuntimeMessage(t, trace, ctx, first.baseURL, first.runtimeSecret, persistenceSeedRequest())
	assertToolCalls(t, firstEvents, "write_file")
	assertRuntimeSessionFinal(t, trace, firstResponseEvents, []string{"PERSISTENCE_SEED_DONE", "PERSISTENCE_E2E_UNIQUE"})
	waitForWorkspaceFileContaining(t, trace, workspaceRoot, ".skills/persisted-skill/SKILL.md", "PERSISTED_SKILL_OK", time.Minute)
	first.proxy.assertUsed(t)
	first.controlPlane.waitForActivity(t)
	first.stopRuntime()

	second := startPersistedRuntimeContainer(t, trace, ctx, workspaceRoot)
	defer second.stopRuntime()
	assertPersistedConfigBeforeApply(t, trace, ctx, second.baseURL, second.runtimeSecret)
	applyPersistedRuntimeConfig(t, trace, ctx, second)
	waitForWorkspaceFileContaining(t, trace, workspaceRoot, ".skills/persisted-skill/SKILL.md", "PERSISTED_SKILL_OK", time.Minute)
	assertPersistedSessionAPI(t, trace, ctx, second.baseURL, second.runtimeSecret, firstResponse.SessionID)

	_, secondEvents, secondResponseEvents := runAgentRuntimeMessage(t, trace, ctx, second.baseURL, second.runtimeSecret, persistenceResumeRequest())
	assertToolCalls(t, secondEvents, "search_sessions", "read_file")
	assertRuntimeSessionFinal(t, trace, secondResponseEvents, []string{"PERSISTENCE_E2E_PASS", "PERSISTENCE_E2E_UNIQUE"})
	second.proxy.assertUsed(t)
	second.controlPlane.waitForActivity(t)
	trace.Logf("done", "persistence runtime E2E completed")
}

func persistenceDefinition(t *testing.T, proxyURL, controlPlaneURL, sandboxID string) map[string]any {
	return agentRuntimeFeatureDefinition(t, proxyURL, controlPlaneURL, sandboxID, "Runtime E2E Persistence Agent", strings.Join([]string{
		"You are executing the Hivy runtime persistence E2E.",
		"Use file and session-search tools exactly when requested.",
		"Final answers must include the requested persistence markers.",
	}, "\n"), agentRuntimeFileTools(), []any{}, []any{map[string]any{
		"name":         "persisted-skill",
		"description":  "Persistence E2E skill.",
		"category":     "runtime-e2e",
		"trigger":      map[string]any{"type": "keyword", "patterns": []string{"persistence e2e"}},
		"instructions": "PERSISTED_SKILL_OK",
	}})
}

func persistenceSeedRequest() map[string]any {
	return map[string]any{
		"session_id": "agent-runtime-persistence-e2e",
		"user":       "agent-runtime-e2e",
		"text": strings.Join([]string{
			"Seed persistence state.",
			"1. Use write_file to create persistence_seed.txt containing PERSISTENCE_E2E_UNIQUE.",
			"2. Final answer must include PERSISTENCE_SEED_DONE and PERSISTENCE_E2E_UNIQUE.",
		}, "\n"),
		"raw": map[string]any{"source": "session", "test": "agent-runtime-persistence-e2e"},
	}
}

func persistenceResumeRequest() map[string]any {
	return map[string]any{
		"session_id": "agent-runtime-persistence-resume-e2e",
		"user":       "agent-runtime-e2e",
		"text": strings.Join([]string{
			"Prove persistence after restart.",
			"1. Call search_sessions with query PERSISTENCE_E2E_UNIQUE.",
			"2. Read persistence_seed.txt with read_file.",
			"3. Final answer must include PERSISTENCE_E2E_PASS and PERSISTENCE_E2E_UNIQUE.",
		}, "\n"),
		"raw": map[string]any{"source": "session", "test": "agent-runtime-persistence-resume-e2e"},
	}
}

func startPersistedRuntimeContainer(t *testing.T, trace *agentRuntimeE2ETrace, ctx context.Context, workspaceRoot string) *agentRuntimeE2EScenario {
	t.Helper()
	loadEnv(t)
	systemModelKey := strings.TrimSpace(os.Getenv("HIVY_SYSTEM_OPENROUTER_API_KEY"))
	if systemModelKey == "" {
		t.Skip("HIVY_SYSTEM_OPENROUTER_API_KEY is not configured")
	}
	runtimeSecret := "agent-runtime-e2e-secret" // #nosec G101 -- fixed local E2E runtime secret.
	agentID := uuid.NewString()
	sandboxID := uuid.NewString()
	controlPlane := newAgentRuntimeMockControlPlane(t, trace, runtimeSecret, agentID, sandboxID)
	proxy := newAgentRuntimeModelProxy(t, trace, systemModelKey, agentRuntimeProxyToken)
	t.Cleanup(controlPlane.server.Close)
	t.Cleanup(proxy.server.Close)
	baseURL, stopRuntime := startAgentRuntimeContainerWithOptions(t, trace, ctx, repoRootFromE2E(t), workspaceRoot, runtimeSecret, agentRuntimeProxyToken, agentID, sandboxID, controlPlane.containerURL, agentRuntimeContainerOptions{dbPath: "/workspace/runtime.db"})
	return &agentRuntimeE2EScenario{trace: trace, ctx: ctx, workspaceRoot: workspaceRoot, runtimeSecret: runtimeSecret, agentID: agentID, sandboxID: sandboxID, baseURL: baseURL, stopRuntime: stopRuntime, proxy: proxy, controlPlane: controlPlane}
}

func assertPersistedConfigBeforeApply(t *testing.T, trace *agentRuntimeE2ETrace, ctx context.Context, baseURL, token string) {
	t.Helper()
	body := doRuntimeJSON(t, trace, ctx, http.MethodGet, baseURL+"/config", token, nil, http.StatusOK)
	var config map[string]any
	if err := json.Unmarshal(body, &config); err != nil {
		t.Fatalf("decode persisted /config: %v\n%s", err, body)
	}
	agent, _ := config["agent"].(map[string]any)
	if got, _ := agent["name"].(string); got != "Runtime E2E Persistence Agent" {
		t.Fatalf("persisted config agent name=%q body=%s", got, body)
	}
	trace.Logf("assert", "persisted /config loaded before second config push")
}

func applyPersistedRuntimeConfig(t *testing.T, trace *agentRuntimeE2ETrace, ctx context.Context, s *agentRuntimeE2EScenario) {
	t.Helper()
	definition := persistenceDefinition(t, containerURLForServer(s.proxy.server.URL), s.controlPlane.containerURL, s.sandboxID)
	payload := agentRuntimeConfigPayload(s.runtimeSecret, s.controlPlane.containerURL, s.agentID, s.sandboxID, definition, nil)
	doRuntimeJSON(t, trace, ctx, http.MethodPut, s.baseURL+"/config", s.runtimeSecret, mustJSON(t, payload), http.StatusOK)
	waitForRuntimeReady(t, trace, ctx, s.baseURL, s.runtimeSecret)
}

func assertPersistedSessionAPI(t *testing.T, trace *agentRuntimeE2ETrace, ctx context.Context, baseURL, token, sessionID string) {
	t.Helper()
	body := doRuntimeJSON(t, trace, ctx, http.MethodGet, baseURL+"/sessions?session_id="+sessionID, token, nil, http.StatusOK)
	if !strings.Contains(string(body), sessionID) {
		t.Fatalf("persisted /sessions did not include %s: %s", sessionID, body)
	}
	detail := doRuntimeJSON(t, trace, ctx, http.MethodGet, baseURL+"/sessions/"+sessionID, token, nil, http.StatusOK)
	if !strings.Contains(string(detail), "PERSISTENCE_E2E_UNIQUE") {
		t.Fatalf("persisted session detail missing marker: %s", detail)
	}
	trace.Logf("assert", "persisted session API returned previous session=%s", sessionID)
}
