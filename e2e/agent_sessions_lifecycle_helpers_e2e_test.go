package e2e

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	dockerclient "github.com/docker/docker/client"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/mcpserver"
	"github.com/usehivy/hivy/internal/model"
)

func agentSessionsCreateCronViaMCP(t *testing.T, ctx context.Context, orgIDRaw, agentIDRaw, sessionID string, intervalSeconds int64, description, taskPrompt string) model.AgentSchedule {
	t.Helper()
	db := agentSessionsOpenDB(t)
	orgID := uuid.MustParse(orgIDRaw)
	agentID := uuid.MustParse(agentIDRaw)
	jobID := "lifecycle-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	token := &model.Token{
		OrgID: orgID,
		Meta: model.JSON{
			model.TokenMetaType:    model.TokenTypeAgentProxy,
			model.TokenMetaAgentID: agentID.String(),
		},
	}
	server, err := mcpserver.BuildServer(ctx, token, db, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("build Hivy MCP server: %v", err)
	}
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("connect Hivy MCP server: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "agent-sessions-lifecycle-e2e", Version: "v1"}, nil)
	mcpSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect Hivy MCP client: %v", err)
	}
	result, err := mcpSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "cron",
		Arguments: map[string]any{
			"action":             "create",
			"job_id":             jobID,
			"description":        description,
			"task_prompt":        taskPrompt,
			"interval_seconds":   intervalSeconds,
			"repeat_count":       int64(1),
			"_hivy_session_id":   sessionID,
			"cron_expression":    nil,
			"schedule_is_system": false,
		},
	})
	if err != nil {
		t.Fatalf("call Hivy MCP cron create: %v", err)
	}
	if result.IsError {
		t.Fatalf("Hivy MCP cron create failed: %s", agentSessionsMCPText(result))
	}
	var schedule model.AgentSchedule
	if err := db.WithContext(ctx).
		Where("org_id = ? AND agent_id = ? AND runtime_job_id = ?", orgID, agentID, jobID).
		First(&schedule).Error; err != nil {
		t.Fatalf("load schedule created via MCP: %v; result=%s", err, agentSessionsMCPText(result))
	}
	return schedule
}

func agentSessionsMCPText(result *mcp.CallToolResult) string {
	if result == nil {
		return ""
	}
	var parts []string
	for _, item := range result.Content {
		if text, ok := item.(*mcp.TextContent); ok {
			parts = append(parts, text.Text)
		} else {
			raw, _ := json.Marshal(item)
			parts = append(parts, string(raw))
		}
	}
	return strings.Join(parts, "\n")
}

func agentSessionsWaitForScheduleRunSession(t *testing.T, ctx context.Context, scheduleID uuid.UUID) model.AgentScheduleRun {
	t.Helper()
	db := agentSessionsOpenDB(t)
	deadline := time.Now().Add(4 * time.Minute)
	var last string
	for time.Now().Before(deadline) {
		var run model.AgentScheduleRun
		err := db.WithContext(ctx).
			Where("schedule_id = ?", scheduleID).
			Order("created_at ASC").
			Last(&run).Error
		if err == nil {
			last = "status=" + run.Status + " run=" + run.ID.String()
			if run.SessionID != nil {
				return run
			}
		} else if err != gorm.ErrRecordNotFound {
			t.Fatalf("load schedule run: %v", err)
		}
		t.Logf("waiting for schedule run session schedule=%s last=%s", scheduleID, last)
		select {
		case <-ctx.Done():
			t.Fatalf("context expired waiting for schedule run: %v", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
	t.Fatalf("timed out waiting for schedule run session schedule=%s last=%s", scheduleID, last)
	return model.AgentScheduleRun{}
}

func agentSessionsWaitForSandboxDBStatus(t *testing.T, ctx context.Context, sandboxID uuid.UUID, status string) model.Sandbox {
	t.Helper()
	db := agentSessionsOpenDB(t)
	deadline := time.Now().Add(3 * time.Minute)
	var last string
	for time.Now().Before(deadline) {
		var sb model.Sandbox
		if err := db.WithContext(ctx).Where("id = ?", sandboxID).First(&sb).Error; err != nil {
			t.Fatalf("load sandbox %s: %v", sandboxID, err)
		}
		last = "status=" + sb.Status + " external_id=" + sb.ExternalID
		if sb.Status == status {
			return sb
		}
		t.Logf("waiting for sandbox status sandbox=%s want=%s last=%s", sandboxID, status, last)
		select {
		case <-ctx.Done():
			t.Fatalf("context expired waiting for sandbox status: %v", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
	t.Fatalf("timed out waiting for sandbox status sandbox=%s want=%s last=%s", sandboxID, status, last)
	return model.Sandbox{}
}

func agentSessionsWaitForDockerRunning(t *testing.T, ctx context.Context, externalID string, want bool) {
	t.Helper()
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatalf("create docker client: %v", err)
	}
	defer cli.Close()
	deadline := time.Now().Add(2 * time.Minute)
	var last string
	for time.Now().Before(deadline) {
		inspectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		info, err := cli.ContainerInspect(inspectCtx, externalID)
		cancel()
		if err != nil {
			t.Fatalf("inspect docker container %s: %v", externalID, err)
		}
		running := info.State != nil && info.State.Running
		if info.State != nil {
			last = info.State.Status
		}
		if running == want {
			return
		}
		t.Logf("waiting for docker running=%t container=%s last_status=%s", want, externalID, last)
		select {
		case <-ctx.Done():
			t.Fatalf("context expired waiting for docker state: %v", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
	t.Fatalf("timed out waiting for docker running=%t container=%s last_status=%s", want, externalID, last)
}
