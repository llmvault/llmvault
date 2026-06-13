package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAgentRuntimeSkillsE2E(t *testing.T) {
	ctx, cancel, trace := agentRuntimeE2EContext(t, 30*time.Minute)
	defer cancel()
	trace.Logf("start", "skills runtime E2E starting")

	scenario := startAgentRuntimeE2EScenario(
		t,
		trace,
		ctx,
		agentRuntimeE2EScenarioOptions{name: "skills", workspaceRoot: t.TempDir()},
		func(proxyURL, controlPlaneURL, sandboxID string) map[string]any {
			return agentRuntimeFeatureDefinition(t, proxyURL, controlPlaneURL, sandboxID, "Runtime E2E Skills Agent", strings.Join([]string{
				"You are executing the Hivy runtime skills E2E.",
				"Use the exact tools requested by the user.",
				"Do not skip skill_manage operations after viewing the initial skill.",
				"Finish only after every requested skill operation succeeds.",
			}, "\n"), agentRuntimeSkillsTools(), []any{}, initialSkillsE2EConfig())
		},
	)

	response, events, responseEvents := runAgentRuntimeMessage(t, trace, ctx, scenario.baseURL, scenario.runtimeSecret, skillsE2ERequest())
	assertToolCalls(t, events, "skills_list", "skill_view", "skill_manage")
	assertRuntimeSessionFinal(t, trace, responseEvents, []string{"SKILL_E2E_PASS", "MANAGED_PATCHED", "INITIAL_GUIDE_OK"})
	assertAgentRuntimePostRunAPIs(t, trace, ctx, scenario.baseURL, scenario.runtimeSecret, response.SessionID, response.TraceID, scenario.workspaceRoot)
	assertSkillsE2EWorkspace(t, trace, scenario.workspaceRoot)
	scenario.controlPlane.waitForWebhookPayloadContaining(t, "skill.synced", "runtime-managed")
	scenario.controlPlane.waitForWebhookPayloadContaining(t, "skill.synced", "runtime-delete-me")
	scenario.proxy.assertUsed(t)
	scenario.controlPlane.waitForActivity(t)
	trace.Logf("done", "skills runtime E2E completed")
}

func skillsE2ERequest() map[string]any {
	return map[string]any{
		"session_id": "agent-runtime-skills-e2e",
		"user":       "agent-runtime-e2e",
		"text": strings.Join([]string{
			"Complete the skills E2E using the named tools.",
			"1. Call skills_list with category runtime-e2e.",
			"2. Call skill_view for runtime-initial.",
			"3. Call skill_view for runtime-initial with file_path references/guide.md.",
			"4. Call skill_manage create for runtime-managed. Content must include MANAGED_PLACEHOLDER.",
			"5. Call skill_manage write_file for runtime-managed file_path references/result.md with content RESULT_FILE_OK.",
			"6. Call skill_manage write_file for runtime-managed file_path references/remove.md with content REMOVE_FILE_OK.",
			"7. Call skill_manage remove_file for runtime-managed file_path references/remove.md.",
			"8. Call skill_manage patch for runtime-managed replacing MANAGED_PLACEHOLDER with MANAGED_PATCHED.",
			"9. Call skill_manage create for runtime-delete-me with simple content containing DELETE_ME_OK.",
			"10. Call skill_manage delete for runtime-delete-me with absorbed_into runtime-managed.",
			"11. Final answer must include SKILL_E2E_PASS, INITIAL_GUIDE_OK, and MANAGED_PATCHED.",
		}, "\n"),
		"raw": map[string]any{"source": "session", "test": "agent-runtime-skills-e2e"},
	}
}

func initialSkillsE2EConfig() []any {
	return []any{map[string]any{
		"name":         "runtime-initial",
		"description":  "Initial runtime E2E skill.",
		"category":     "runtime-e2e",
		"trigger":      map[string]any{"type": "keyword", "patterns": []string{"runtime skills e2e"}},
		"instructions": "This skill proves config-provisioned skills are readable. Marker: INITIAL_GUIDE_OK.",
		"files":        map[string]string{"references/guide.md": "INITIAL_GUIDE_OK\nUse this guide during the runtime skills E2E.\n"},
	}}
}

func assertSkillsE2EWorkspace(t *testing.T, trace *agentRuntimeE2ETrace, workspaceRoot string) {
	t.Helper()
	managed := filepath.Join(workspaceRoot, ".skills", "runtime-managed", "SKILL.md")
	body, err := os.ReadFile(managed)
	if err != nil {
		t.Fatalf("read managed skill: %v", err)
	}
	trace.Body("assert-files", managed, body)
	if !strings.Contains(string(body), "MANAGED_PATCHED") {
		t.Fatalf("managed skill missing patch marker:\n%s", body)
	}
	resultPath := filepath.Join(workspaceRoot, ".skills", "runtime-managed", "references", "result.md")
	result, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatalf("read managed skill linked file: %v", err)
	}
	trace.Body("assert-files", resultPath, result)
	if !strings.Contains(string(result), "RESULT_FILE_OK") {
		t.Fatalf("managed skill linked file missing marker:\n%s", result)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, ".skills", "runtime-managed", "references", "remove.md")); !os.IsNotExist(err) {
		t.Fatalf("runtime-managed remove.md still exists or stat failed unexpectedly: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, ".skills", "runtime-delete-me")); !os.IsNotExist(err) {
		t.Fatalf("runtime-delete-me skill directory still exists or stat failed unexpectedly: %v", err)
	}
}
