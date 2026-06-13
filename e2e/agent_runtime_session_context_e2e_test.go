package e2e

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAgentRuntimeSessionContextE2E(t *testing.T) {
	ctx, cancel, trace := agentRuntimeE2EContext(t, 25*time.Minute)
	defer cancel()
	trace.Logf("start", "session context runtime E2E starting")

	attachmentServer := newPublicHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trace.Logf("attachment", "incoming method=%s path=%s", r.Method, r.URL.Path)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ATTACHMENT_CONTENT_OK\n"))
	}))
	defer attachmentServer.Close()

	scenario := startAgentRuntimeE2EScenario(
		t,
		trace,
		ctx,
		agentRuntimeE2EScenarioOptions{name: "session-context", workspaceRoot: t.TempDir()},
		func(proxyURL, controlPlaneURL, sandboxID string) map[string]any {
			return agentRuntimeFeatureDefinition(t, proxyURL, controlPlaneURL, sandboxID, "Runtime E2E Session Agent", strings.Join([]string{
				"You are executing the Hivy runtime session context E2E.",
				"Use the user-visible message, dynamic context, and attached text file content.",
				"Final answers must include every requested marker.",
			}, "\n"), agentRuntimeFileTools(), []any{}, []any{})
		},
	)

	response, _, responseEvents := runAgentRuntimeMessage(t, trace, ctx, scenario.baseURL, scenario.runtimeSecret, sessionContextE2ERequest(containerURLForServer(attachmentServer.URL)+"/fixture.txt"))
	assertRuntimeSessionFinal(t, trace, responseEvents, []string{"SESSION_CONTEXT_E2E_PASS", "DYNAMIC_CONTEXT_OK", "ATTACHMENT_CONTENT_OK"})
	assertAgentRuntimePostRunAPIs(t, trace, ctx, scenario.baseURL, scenario.runtimeSecret, response.SessionID, response.TraceID, scenario.workspaceRoot)
	scenario.proxy.assertUsed(t)
	scenario.controlPlane.waitForActivity(t)
	trace.Logf("done", "session context runtime E2E completed")
}

func sessionContextE2ERequest(attachmentURL string) map[string]any {
	return map[string]any{
		"session_id":      "agent-runtime-session-context-e2e",
		"user":            "agent-runtime-e2e",
		"text":            "Reply with SESSION_CONTEXT_E2E_PASS plus the dynamic context marker and attached file marker.",
		"dynamic_context": []string{"Runtime session dynamic context marker: DYNAMIC_CONTEXT_OK"},
		"attachments": []map[string]any{{
			"url":        attachmentURL,
			"mime_type":  "text/plain",
			"name":       "session-context.txt",
			"size_bytes": 22,
		}},
		"raw": map[string]any{"source": "session", "test": "agent-runtime-session-context-e2e"},
	}
}
