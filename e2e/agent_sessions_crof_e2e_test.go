package e2e

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

const agentSessionsCrofDeepSeekFlashModel = "crof-deepseek-v4-flash"

func TestAgentSessionsCrofDeepSeekFlashE2E(t *testing.T) {
	if os.Getenv("HIVY_AGENT_SESSIONS_E2E") != "1" {
		t.Skip("set HIVY_AGENT_SESSIONS_E2E=1 to run against the live compose stack")
	}
	loadEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	apiBase := agentSessionsBaseURL("HIVY_API_BASE_URL", "HIVY_COMPOSE_API_PORT", "8080")
	workerBase := agentSessionsBaseURL("HIVY_WORKER_BASE_URL", "HIVY_COMPOSE_WORKER_HEALTH_PORT", "8090")
	requireAgentSessionsHealthy(t, ctx, apiBase, "api")
	requireAgentSessionsHealthy(t, ctx, workerBase, "worker")
	agentSessionsEnsureSystemCrofCredential(t)

	runID := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	password := "agent-sessions-crof-e2e-password"
	ownerEmail := "agent-sessions-crof-" + runID + "@example.com"
	finalMarker := "AGENT_SESSIONS_CROF_E2E_PASS_" + runID

	ownerAuth := agentSessionsRegister(t, ctx, apiBase, ownerEmail, password, "Agent Sessions Crof "+runID)
	if len(ownerAuth.Orgs) != 1 || ownerAuth.Orgs[0].ID == "" {
		t.Fatalf("owner register did not return one org: %+v", ownerAuth.Orgs)
	}
	orgID := ownerAuth.Orgs[0].ID
	ownerToken := ownerAuth.AccessToken

	agent := agentSessionsCreateCrofAgent(t, ctx, apiBase, ownerToken, orgID, runID)
	channel := agentSessionsCreateChannel(t, ctx, apiBase, ownerToken, orgID, "crof-e2e-"+runID, agent.ID)

	session := agentSessionsCreateSessionWithPayload(t, ctx, apiBase, ownerToken, orgID, map[string]any{
		"channel_id":       channel.ID,
		"reasoning_effort": "low",
		"text": strings.Join([]string{
			"This is the Crof provider agent-session E2E.",
			"Do not call tools.",
			"Visible final reply exactly " + finalMarker + " and no other text.",
		}, "\n"),
	})
	if session.Session.ID == "" {
		t.Fatalf("session was not created correctly: %+v", session)
	}
	assertAgentSessionsBackendOwnedMutationEvent(t, session.Event)

	sandbox := agentSessionsWaitForSessionSandbox(t, ctx, orgID, session.Session.ID)
	t.Cleanup(func() {
		agentSessionsDeleteSandbox(t, ctx, apiBase, ownerToken, orgID, sandbox.ID.String())
	})

	waitForAgentSessionsResponse(t, ctx, apiBase, ownerToken, orgID, session.Session.ID, finalMarker)
	events := agentSessionsListAllEvents(t, ctx, apiBase, ownerToken, orgID, session.Session.ID)
	assertAgentSessionsModelUsageModel(t, events, agentSessionsCrofDeepSeekFlashModel)
}

func agentSessionsCreateCrofAgent(t *testing.T, ctx context.Context, baseURL, token, orgID, runID string) agentSessionsAgentListItem {
	t.Helper()
	var out agentSessionsAgentMutation
	payload := map[string]any{
		"name":             "Crof E2E " + runID,
		"instructions":     "Answer directly and do not use tools unless explicitly required.",
		"model":            agentSessionsCrofDeepSeekFlashModel,
		"available_models": []string{agentSessionsCrofDeepSeekFlashModel},
		"sandbox_strategy": "per_session",
	}
	agentSessionsJSON(t, ctx, http.MethodPost, baseURL+"/v1/agents", token, orgID, payload, http.StatusCreated, &out)
	if out.Agent.ID == "" {
		t.Fatalf("Crof agent create returned empty agent: %+v", out)
	}
	if out.Agent.Model != agentSessionsCrofDeepSeekFlashModel {
		t.Fatalf("Crof agent model=%q want %q", out.Agent.Model, agentSessionsCrofDeepSeekFlashModel)
	}
	return out.Agent
}

func assertAgentSessionsModelUsageModel(t *testing.T, events []agentSessionsEvent, want string) {
	t.Helper()
	for _, event := range events {
		if event.EventType == "model_usage" && eventString(event.Payload, "model") == want {
			return
		}
	}
	t.Fatalf("missing model_usage for model %s; events=%s", want, summarizeSessionEventTypes(events))
}
