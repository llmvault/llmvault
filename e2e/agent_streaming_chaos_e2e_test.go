package e2e

import (
	"context"
	"os/exec"
	"testing"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func runStreamingWorkerResumeChaos(t *testing.T, ctx context.Context, apiBase, workerBase, token, orgID, channelID string, redisClient redis.UniversalClient, db *gorm.DB, runID string) {
	t.Helper()
	session := agentSessionsCreateSession(t, ctx, apiBase, token, orgID, channelID, streamingE2EPrompt(runID, 9, 1, "STREAMING_E2E_WORKER_CHAOS_"+runID))
	restartComposeService(t, ctx, "worker")
	requireAgentSessionsHealthy(t, ctx, workerBase, "worker")
	result, err := runAPISessionSubscriber(ctx, apiBase, token, orgID, session.Session.ID, "STREAMING_E2E_WORKER_CHAOS_"+runID, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.committed) == 0 {
		t.Fatalf("worker chaos subscriber saw no committed events")
	}
	assertRuntimeRedisAndPostgresConverged(t, ctx, db, redisClient, session.Session.ID)
}

func runStreamingAPIReconnectChaos(t *testing.T, ctx context.Context, apiBase, workerBase, token, orgID, channelID string, redisClient redis.UniversalClient, db *gorm.DB, runID string) {
	t.Helper()
	session := agentSessionsCreateSession(t, ctx, apiBase, token, orgID, channelID, streamingE2EPrompt(runID, 9, 2, "STREAMING_E2E_API_CHAOS_"+runID))
	restartComposeService(t, ctx, "api")
	requireAgentSessionsHealthy(t, ctx, apiBase, "api")
	requireAgentSessionsHealthy(t, ctx, workerBase, "worker")
	result, err := runAPISessionSubscriber(ctx, apiBase, token, orgID, session.Session.ID, "STREAMING_E2E_API_CHAOS_"+runID, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.committed) == 0 {
		t.Fatalf("api chaos subscriber saw no committed events")
	}
	assertRuntimeRedisAndPostgresConverged(t, ctx, db, redisClient, session.Session.ID)
}

func restartComposeService(t *testing.T, ctx context.Context, service string) {
	t.Helper()
	cmd := exec.CommandContext(ctx, "docker", "compose", "restart", service)
	cmd.Dir = ".."
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker compose restart %s: %v\n%s", service, err, output)
	}
}
