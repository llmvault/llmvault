package agentruntime

import (
	"context"
	"fmt"

	"github.com/usehivy/hivy/internal/model"
)

func PushAgentRuntimeConfigForSandbox(ctx context.Context, deps CompileDeps, sb *model.Sandbox) error {
	return PushAgentRuntimeConfigForSandboxWithProxyToken(ctx, deps, sb, nil)
}

func PushAgentRuntimeConfigForSandboxWithProxyToken(ctx context.Context, deps CompileDeps, sb *model.Sandbox, proxyToken *ProxyTokenResult) error {
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
	return PushAgentRuntimeConfigWithProxyToken(ctx, deps, &agent, sb, proxyToken)
}

func PushAgentRuntimeConfig(ctx context.Context, deps CompileDeps, agent *model.Agent, sb *model.Sandbox) error {
	return PushAgentRuntimeConfigWithProxyToken(ctx, deps, agent, sb, nil)
}

func PushAgentRuntimeConfigWithProxyToken(ctx context.Context, deps CompileDeps, agent *model.Agent, sb *model.Sandbox, proxyToken *ProxyTokenResult) error {
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
	var configUpdate ConfigUpdateRequest
	if proxyToken != nil {
		if err := AttachProxyTokenToSandbox(ctx, deps, agent, sb.ID, proxyToken.JTI); err != nil {
			return err
		}
		var buildErr error
		configUpdate, buildErr = BuildAgentRuntimeConfigUpdateWithProxyToken(ctx, deps, agent, sb, runtimeSecret, proxyToken)
		if buildErr != nil {
			return fmt.Errorf("build agent runtime config: %w", buildErr)
		}
	} else {
		var buildErr error
		configUpdate, _, buildErr = BuildAgentRuntimeConfigUpdate(ctx, deps, agent, sb, runtimeSecret)
		if buildErr != nil {
			return fmt.Errorf("build agent runtime config: %w", buildErr)
		}
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
	return nil
}
