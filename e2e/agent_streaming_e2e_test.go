package e2e

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const streamingE2EModel = "deepseek-v4-flash"

type streamingSessionCase struct {
	agent   agentSessionsAgentListItem
	channel agentSessionsChannel
	session agentSessionsMutation
	marker  string
	prompt  string
}

func TestAgentRuntimeRedisSequencingE2E(t *testing.T) {
	if os.Getenv("HIVY_AGENT_STREAMING_E2E") != "1" {
		t.Skip("set HIVY_AGENT_STREAMING_E2E=1 to run against the live compose stack")
	}
	loadEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Minute)
	defer cancel()

	apiBase := agentSessionsBaseURL("HIVY_API_BASE_URL", "HIVY_COMPOSE_API_PORT", "8080")
	workerBase := agentSessionsBaseURL("HIVY_WORKER_BASE_URL", "HIVY_COMPOSE_WORKER_HEALTH_PORT", "8090")
	requireAgentSessionsHealthy(t, ctx, apiBase, "api")
	requireAgentSessionsHealthy(t, ctx, workerBase, "worker")
	agentSessionsEnsureSystemOpenRouterCredential(t)

	db := agentSessionsOpenDB(t)
	redisClient := redis.NewClient(&redis.Options{Addr: testRedisAddrOrEnv()})
	t.Cleanup(func() { _ = redisClient.Close() })

	runID := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	password := "agent-streaming-e2e-password"
	ownerEmail := "agent-streaming-owner-" + runID + "@example.com"
	ownerAuth := agentSessionsRegister(t, ctx, apiBase, ownerEmail, password, "Agent Streaming Owner "+runID)
	orgID := ownerAuth.Orgs[0].ID
	token := ownerAuth.AccessToken

	agents := make([]agentSessionsAgentListItem, 0, 2)
	for i := 0; i < 2; i++ {
		agent := agentStreamingCreateAgent(t, ctx, apiBase, token, orgID, fmt.Sprintf("Streaming %s %d", runID, i+1))
		agents = append(agents, agent)
	}

	var cases []streamingSessionCase
	for agentIndex, agent := range agents {
		channel := agentSessionsCreateChannel(t, ctx, apiBase, token, orgID, fmt.Sprintf("streaming-%s-%d", runID, agentIndex+1), agent.ID)
		for sessionIndex := 0; sessionIndex < 3; sessionIndex++ {
			marker := fmt.Sprintf("STREAMING_E2E_FINAL_%s_A%d_S%d", runID, agentIndex+1, sessionIndex+1)
			prompt := streamingE2EPrompt(runID, agentIndex+1, sessionIndex+1, marker)
			session := agentSessionsCreateSessionWithPayload(t, ctx, apiBase, token, orgID, map[string]any{
				"channel_id": channel.ID,
			})
			if session.Session.ID == "" {
				t.Fatalf("session create returned empty id: %+v", session)
			}
			if session.Queued {
				t.Fatalf("empty session create queued a message: %+v", session)
			}
			if session.Event != nil {
				t.Fatalf("empty session create returned event: %+v", session.Event)
			}
			cases = append(cases, streamingSessionCase{agent: agent, channel: channel, session: session, marker: marker, prompt: prompt})
		}
	}

	ready := make(chan subscriberReady, len(cases)*3)
	results := make(chan subscriberResult, len(cases)*3)
	errs := make(chan error, len(cases)*3)
	var wg sync.WaitGroup
	for _, tc := range cases {
		for subscriberIndex := 0; subscriberIndex < 3; subscriberIndex++ {
			wg.Add(1)
			go func(tc streamingSessionCase, subscriberIndex int) {
				defer wg.Done()
				result, err := runSandboxSessionSubscriber(ctx, apiBase, token, orgID, tc.session.Session.ID, tc.marker, subscriberIndex, ready)
				if err != nil {
					errs <- err
					return
				}
				results <- result
			}(tc, subscriberIndex)
		}
	}
	waitForStreamingSubscribersReady(t, ctx, ready, errs, len(cases)*3)
	sendStreamingPrompts(t, ctx, apiBase, token, orgID, cases)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	bySession := map[string][]subscriberResult{}
	for result := range results {
		bySession[result.sessionID] = append(bySession[result.sessionID], result)
	}
	for _, tc := range cases {
		sessionID := tc.session.Session.ID
		sessionResults := bySession[sessionID]
		if len(sessionResults) != 3 {
			t.Fatalf("session %s subscriber results=%d, want 3", sessionID, len(sessionResults))
		}
		assertStreamingSubscribersConverged(t, sessionID, sessionResults)
		assertRuntimeRedisAndPostgresConverged(t, ctx, db, redisClient, sessionID)
	}

	t.Run("worker_resume", func(t *testing.T) {
		if os.Getenv("HIVY_AGENT_STREAMING_E2E_CHAOS") != "1" {
			t.Skip("set HIVY_AGENT_STREAMING_E2E_CHAOS=1 to run docker compose restart chaos checks")
		}
		runStreamingWorkerResumeChaos(t, ctx, apiBase, workerBase, token, orgID, cases[0].channel.ID, redisClient, db, runID)
	})

	t.Run("api_reconnect", func(t *testing.T) {
		if os.Getenv("HIVY_AGENT_STREAMING_E2E_CHAOS") != "1" {
			t.Skip("set HIVY_AGENT_STREAMING_E2E_CHAOS=1 to run docker compose restart chaos checks")
		}
		runStreamingAPIReconnectChaos(t, ctx, apiBase, workerBase, token, orgID, cases[1].channel.ID, redisClient, db, runID)
	})
}

func agentStreamingCreateAgent(t *testing.T, ctx context.Context, baseURL, token, orgID, name string) agentSessionsAgentListItem {
	t.Helper()
	var out agentSessionsAgentMutation
	agentSessionsJSON(t, ctx, http.MethodPost, baseURL+"/v1/agents", token, orgID, map[string]any{
		"name":                name,
		"description":         "Runtime Redis sequencing E2E agent",
		"instructions":        "Use tools exactly when requested. Keep responses deterministic and include requested markers exactly.",
		"model":               streamingE2EModel,
		"sandbox_tools":       []string{"bash"},
		"permissions":         map[string]any{"bash": true},
		"tools":               map[string]any{"bash": true},
		"resources":           map[string]any{},
		"mcp_servers":         []any{},
		"skills":              map[string]any{},
		"sandbox_template_id": nil,
	}, http.StatusCreated, &out)
	if out.Agent.ID == "" {
		t.Fatalf("create streaming agent returned empty id: %+v", out)
	}
	return out.Agent
}

func streamingE2EPrompt(runID string, agentIndex, sessionIndex int, marker string) string {
	toolMarker := fmt.Sprintf("STREAMING_E2E_TOOL_%s_A%d_S%d", runID, agentIndex, sessionIndex)
	lines := make([]string, 0, 30)
	lines = append(lines,
		"This is the runtime Redis sequencing flagship E2E.",
		fmt.Sprintf("Before replying, call bash exactly once with this command: python3 -c 'print(%q)'.", toolMarker),
		"After the bash result, produce a numbered response with 30 short lines.",
		"Every line must contain the exact session marker "+marker+".",
		"Do not use markdown tables.",
	)
	return strings.Join(lines, "\n")
}
