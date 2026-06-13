package agentruntime

import (
	"context"
	"fmt"

	"github.com/usehivy/hivy/internal/model"
)

func PushAgentRuntimeConfigForSandbox(ctx context.Context, deps CompileDeps, sb *model.Sandbox) error {
	if deps.DB == nil {
		return fmt.Errorf("agent runtime config push: db is required")
	}
	if deps.EncKey == nil {
		return fmt.Errorf("agent runtime config push: encryption key is required")
	}
	if sb == nil || sb.AgentID == nil || sb.OrgID == nil {
		return fmt.Errorf("agent runtime config push: sandbox must have agent_id and org_id")
	}
	var agent model.Agent
	if err := deps.DB.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", *sb.AgentID, *sb.OrgID, "archived").
		First(&agent).Error; err != nil {
		return fmt.Errorf("load agent for runtime config push: %w", err)
	}
	return PushAgentRuntimeConfig(ctx, deps, &agent, sb)
}

func PushAgentRuntimeConfig(ctx context.Context, deps CompileDeps, agent *model.Agent, sb *model.Sandbox) error {
	if deps.EncKey == nil {
		return fmt.Errorf("agent runtime config push: encryption key is required")
	}
	if agent == nil || agent.OrgID == nil {
		return fmt.Errorf("agent runtime config push: agent must have org_id")
	}
	if sb == nil {
		return fmt.Errorf("agent runtime config push: sandbox is required")
	}
	runtimeSecret, err := deps.EncKey.DecryptString(sb.EncryptedRuntimeSecret)
	if err != nil {
		return fmt.Errorf("decrypt runtime secret: %w", err)
	}
	configUpdate, _, err := BuildAgentRuntimeConfigUpdate(ctx, deps, agent, sb, runtimeSecret)
	if err != nil {
		return fmt.Errorf("build agent runtime config: %w", err)
	}
	client := NewClient(sb.RuntimeURL, runtimeSecret)
	if err := client.Healthz(ctx); err != nil {
		return fmt.Errorf("agent runtime healthz: %w", err)
	}
	if _, err := client.PutRuntimeConfig(ctx, configUpdate); err != nil {
		return fmt.Errorf("agent runtime put config: %w", err)
	}
	if err := client.Readyz(ctx); err != nil {
		return fmt.Errorf("agent runtime readyz: %w", err)
	}
	// Repoint schedules only after the runtime accepts the config carrying them, so
	// a failed compile/push never mutates schedule rows.
	if err := RepointAgentSchedules(ctx, deps.DB, agent, sb); err != nil {
		return fmt.Errorf("repoint agent schedules: %w", err)
	}
	return nil
}
