package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/client"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func agentSessionsWaitForSessionSandbox(t *testing.T, ctx context.Context, orgIDRaw, sessionIDRaw string) model.Sandbox {
	t.Helper()
	db := agentSessionsOpenDB(t)
	orgID := uuid.MustParse(orgIDRaw)
	sessionID := uuid.MustParse(sessionIDRaw)
	deadline := time.Now().Add(4 * time.Minute)
	last := "not found"
	for time.Now().Before(deadline) {
		var sb model.Sandbox
		err := db.WithContext(ctx).Joins("JOIN sessions ON sessions.sandbox_id = sandboxes.id").Where("sessions.id = ? AND sessions.org_id = ?", sessionID, orgID).Take(&sb).Error
		if err == nil {
			last = fmt.Sprintf("sandbox_id=%s status=%s provider=%s external_id=%s", sb.ID, sb.Status, sb.ProviderID, sb.ExternalID)
			if sb.Status == "running" && sb.ExternalID != "" {
				return sb
			}
		} else if err != gorm.ErrRecordNotFound {
			t.Fatalf("load session sandbox: %v", err)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("context expired waiting for session sandbox: %v", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
	t.Fatalf("timed out waiting for session sandbox session=%s last=%s", sessionID, last)
	return model.Sandbox{}
}

func assertAgentSessionsDockerContainer(t *testing.T, ctx context.Context, label string, sb model.Sandbox) {
	t.Helper()
	if sb.ProviderID != "docker" {
		t.Fatalf("%s sandbox provider=%q want docker", label, sb.ProviderID)
	}
	if strings.TrimSpace(sb.ExternalID) == "" {
		t.Fatalf("%s sandbox external_id is empty", label)
	}
	inspectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatalf("%s create docker client: %v", label, err)
	}
	defer cli.Close()
	info, err := cli.ContainerInspect(inspectCtx, sb.ExternalID)
	if err != nil {
		t.Fatalf("%s docker inspect failed: %v", label, err)
	}
	if info.State == nil || !info.State.Running {
		t.Fatalf("%s docker container is not running", label)
	}
}
