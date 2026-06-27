package agentruntime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"gorm.io/gorm"
)

func PushAgentRuntimeConfigForSandbox(ctx context.Context, deps CompileDeps, sb *model.Sandbox) error {
	return PushAgentRuntimeConfigForSandboxWithProxyToken(ctx, deps, sb, nil)
}

func PushAgentRuntimeConfigForSandboxWithProxyToken(ctx context.Context, deps CompileDeps, sb *model.Sandbox, proxyToken *ProxyTokenResult) error {
	return PushAgentRuntimeConfigForSandboxWithProxyTokenOptions(ctx, deps, sb, proxyToken, RuntimeConfigOptions{})
}

func PushAgentRuntimeConfigForSandboxWithProxyTokenOptions(ctx context.Context, deps CompileDeps, sb *model.Sandbox, proxyToken *ProxyTokenResult, opts RuntimeConfigOptions) error {
	if deps.DB == nil {
		return fmt.Errorf("agent runtime config push: db is required")
	}
	if deps.EncKey == nil {
		return fmt.Errorf("agent runtime config push: encryption key is required")
	}
	if sb == nil || sb.AgentID == nil || sb.OrgID == nil {
		return fmt.Errorf("agent runtime config push: sandbox must have agent_id and org_id")
	}
	totalStarted := time.Now()
	phaseStarted := totalStarted
	logPhase := func(phase string, attrs ...any) {
		attrs = append(attrs,
			"total_ms", time.Since(totalStarted).Milliseconds(),
			"sandbox_id", sb.ID,
			"agent_id", *sb.AgentID,
			"org_id", *sb.OrgID,
		)
		logging.LogPhase(ctx, "agent runtime config push load phase", phase, phaseStarted, attrs...)
		phaseStarted = time.Now()
	}
	var agent model.Agent
	if err := deps.DB.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", *sb.AgentID, *sb.OrgID, "archived").
		First(&agent).Error; err != nil {
		return fmt.Errorf("load agent for runtime config push: %w", err)
	}
	logPhase("load agent")
	resolvedOpts, err := resolveSandboxRuntimeConfigOptions(ctx, deps, sb, &agent, opts)
	if err != nil {
		return err
	}
	logPhase("resolve runtime options", "resolved_model", strings.TrimSpace(resolvedOpts.ModelID), "has_reasoning_effort", strings.TrimSpace(resolvedOpts.ReasoningEffort) != "")
	return PushAgentRuntimeConfigWithProxyTokenOptions(ctx, deps, &agent, sb, proxyToken, resolvedOpts)
}

func PushAgentRuntimeConfig(ctx context.Context, deps CompileDeps, agent *model.Agent, sb *model.Sandbox) error {
	return PushAgentRuntimeConfigWithProxyToken(ctx, deps, agent, sb, nil)
}

func PushAgentRuntimeConfigWithProxyToken(ctx context.Context, deps CompileDeps, agent *model.Agent, sb *model.Sandbox, proxyToken *ProxyTokenResult) error {
	return PushAgentRuntimeConfigWithProxyTokenOptions(ctx, deps, agent, sb, proxyToken, RuntimeConfigOptions{})
}

func PushAgentRuntimeConfigWithProxyTokenOptions(ctx context.Context, deps CompileDeps, agent *model.Agent, sb *model.Sandbox, proxyToken *ProxyTokenResult, opts RuntimeConfigOptions) error {
	if deps.EncKey == nil {
		return fmt.Errorf("agent runtime config push: encryption key is required")
	}
	if agent == nil || agent.OrgID == nil {
		return fmt.Errorf("agent runtime config push: agent must have org_id")
	}
	if sb == nil {
		return fmt.Errorf("agent runtime config push: sandbox is required")
	}
	totalStarted := time.Now()
	phaseStarted := totalStarted
	logPhase := func(phase string, attrs ...any) {
		attrs = append(attrs,
			"total_ms", time.Since(totalStarted).Milliseconds(),
			"sandbox_id", sb.ID,
			"agent_id", agent.ID,
			"org_id", *agent.OrgID,
		)
		logging.LogPhase(ctx, "agent runtime config push phase", phase, phaseStarted, attrs...)
		phaseStarted = time.Now()
	}
	logPhase("start",
		"has_proxy_token", proxyToken != nil,
		"runtime_url_set", strings.TrimSpace(sb.RuntimeURL) != "",
		"model", strings.TrimSpace(opts.ModelID),
		"has_reasoning_effort", strings.TrimSpace(opts.ReasoningEffort) != "",
	)
	runtimeSecret, err := deps.EncKey.DecryptString(sb.EncryptedRuntimeSecret)
	if err != nil {
		return fmt.Errorf("decrypt runtime secret: %w", err)
	}
	logPhase("decrypt runtime secret")
	var configUpdate ConfigUpdateRequest
	if proxyToken != nil {
		if err := AttachProxyTokenToSandbox(ctx, deps, agent, sb.ID, proxyToken.JTI); err != nil {
			return err
		}
		logPhase("attach proxy token")
		var buildErr error
		configUpdate, buildErr = BuildAgentRuntimeConfigUpdateWithProxyTokenOptions(ctx, deps, agent, sb, runtimeSecret, proxyToken, opts)
		if buildErr != nil {
			return fmt.Errorf("build agent runtime config: %w", buildErr)
		}
	} else {
		var buildErr error
		configUpdate, _, buildErr = BuildAgentRuntimeConfigUpdateWithOptions(ctx, deps, agent, sb, runtimeSecret, opts)
		if buildErr != nil {
			return fmt.Errorf("build agent runtime config: %w", buildErr)
		}
	}
	logPhase("build runtime config", runtimeConfigLogAttrs(configUpdate)...)
	client := NewClient(sb.RuntimeURL, runtimeSecret)
	if err := client.Healthz(ctx); err != nil {
		return fmt.Errorf("agent runtime healthz: %w", err)
	}
	logPhase("runtime healthz")
	if _, err := client.PutRuntimeConfig(ctx, configUpdate); err != nil {
		return fmt.Errorf("agent runtime put config: %w", err)
	}
	logPhase("runtime put config")
	if err := client.Readyz(ctx); err != nil {
		return fmt.Errorf("agent runtime readyz: %w", err)
	}
	logPhase("runtime readyz")
	logPhase("complete")
	return nil
}

func runtimeConfigLogAttrs(update ConfigUpdateRequest) []any {
	attrs := []any{"env_key_count", len(update.RuntimeEnv)}
	if update.Definition != nil {
		attrs = append(attrs,
			"tool_count", len(update.Definition.Tools),
			"mcp_server_count", len(update.Definition.McpServers),
			"skill_count", len(update.Definition.Skills),
			"subagent_count", len(update.Definition.SubAgents),
			"outbound_channel_count", len(update.Definition.OutboundChannels),
		)
	}
	if update.Workspace != nil {
		attrs = append(attrs, "workspace_repo_count", len(update.Workspace.Repos))
	}
	return attrs
}

func resolveSandboxRuntimeConfigOptions(ctx context.Context, deps CompileDeps, sb *model.Sandbox, agent *model.Agent, opts RuntimeConfigOptions) (RuntimeConfigOptions, error) {
	if strings.TrimSpace(opts.ModelID) != "" || strings.TrimSpace(opts.ReasoningEffort) != "" {
		return opts, nil
	}
	if deps.DB == nil || sb == nil || agent == nil {
		return opts, nil
	}
	var session model.Session
	err := deps.DB.WithContext(ctx).
		Where("sandbox_id = ? AND org_id = ? AND agent_id = ? AND status <> ?", sb.ID, *sb.OrgID, agent.ID, "archived").
		Order("created_at DESC").
		First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return opts, nil
	}
	if err != nil {
		return opts, fmt.Errorf("load session for runtime config push: %w", err)
	}
	opts.ModelID = session.Model
	opts.ReasoningEffort = session.ReasoningEffort
	return opts, nil
}
