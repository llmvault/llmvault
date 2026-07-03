package agentruntime

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

const (
	runtimeWorkspaceRoot = "/workspace"
	runtimeDBPath        = AgentRuntimeDBPath
	runtimePort          = 7080
)

func BuildRuntimeEnv(ctx context.Context, deps CompileDeps, agent *model.Agent, sb *model.Sandbox, runtimeSecret string) (map[string]string, error) {
	sandboxID := uuid.Nil
	if sb != nil {
		sandboxID = sb.ID
	}
	token, err := MintProxyToken(ctx, deps, agent, sandboxID)
	if err != nil {
		return nil, err
	}
	return BuildRuntimeEnvWithProxyToken(ctx, deps, agent, sb, runtimeSecret, token, uuid.Nil)
}

func BuildAgentRuntimeConfigUpdate(ctx context.Context, deps CompileDeps, agent *model.Agent, sb *model.Sandbox, runtimeSecret string) (ConfigUpdateRequest, *ProxyTokenResult, error) {
	return BuildAgentRuntimeConfigUpdateWithOptions(ctx, deps, agent, sb, runtimeSecret, RuntimeConfigOptions{})
}

type RuntimeConfigOptions struct {
	ModelID         string
	ReasoningEffort string
	// ChannelID scopes which per-channel env vars are injected. When Nil, the
	// builder resolves it from the sandbox's session. Session creation sets it
	// explicitly because the session↔sandbox link is not yet persisted at the
	// first push.
	ChannelID uuid.UUID
}

func BuildAgentRuntimeConfigUpdateWithOptions(ctx context.Context, deps CompileDeps, agent *model.Agent, sb *model.Sandbox, runtimeSecret string, opts RuntimeConfigOptions) (ConfigUpdateRequest, *ProxyTokenResult, error) {
	sandboxID := uuid.Nil
	if sb != nil {
		sandboxID = sb.ID
	}
	runtimeAgent := agentWithRuntimeModel(agent, opts.ModelID)
	token, err := MintProxyToken(ctx, deps, runtimeAgent, sandboxID)
	if err != nil {
		return ConfigUpdateRequest{}, nil, err
	}
	config, err := BuildAgentRuntimeConfigUpdateWithProxyTokenOptions(ctx, deps, agent, sb, runtimeSecret, token, opts)
	return config, token, err
}

func BuildAgentRuntimeConfigUpdateWithProxyToken(ctx context.Context, deps CompileDeps, agent *model.Agent, sb *model.Sandbox, runtimeSecret string, token *ProxyTokenResult) (ConfigUpdateRequest, error) {
	return BuildAgentRuntimeConfigUpdateWithProxyTokenOptions(ctx, deps, agent, sb, runtimeSecret, token, RuntimeConfigOptions{})
}

func BuildAgentRuntimeConfigUpdateWithProxyTokenOptions(ctx context.Context, deps CompileDeps, agent *model.Agent, sb *model.Sandbox, runtimeSecret string, token *ProxyTokenResult, opts RuntimeConfigOptions) (ConfigUpdateRequest, error) {
	phaseLog := newRuntimeConfigBuildPhaseLogger(ctx, agent, sb)
	runtimeAgent := agentWithRuntimeModel(agent, opts.ModelID)
	modelID := ""
	if runtimeAgent != nil {
		modelID = strings.TrimSpace(runtimeAgent.Model)
	}
	phaseLog.log("start", "model", modelID, "has_reasoning_effort", strings.TrimSpace(opts.ReasoningEffort) != "")
	env, err := BuildRuntimeEnvWithProxyToken(ctx, deps, runtimeAgent, sb, runtimeSecret, token, opts.ChannelID)
	if err != nil {
		return ConfigUpdateRequest{}, err
	}
	phaseLog.log("build runtime env", "env_key_count", len(env))
	def, err := CompileWithProxyToken(ctx, deps, runtimeAgent, token)
	if err != nil {
		return ConfigUpdateRequest{}, err
	}
	phaseLog.log("compile definition",
		"tool_count", len(def.Tools),
		"mcp_server_count", len(def.McpServers),
		"subagent_count", len(def.SubAgents),
	)
	if effort := strings.TrimSpace(opts.ReasoningEffort); effort != "" {
		def.Model.ReasoningEffort = &effort
	}
	sandboxID := uuid.Nil
	if sb != nil {
		sandboxID = sb.ID
	}
	def.OutboundChannels = ControlPlaneOutboundChannels(deps.Cfg, sandboxID)
	phaseLog.log("build outbound channels", "outbound_channel_count", len(def.OutboundChannels))
	workspace, err := BuildWorkspaceConfig(ctx, deps, runtimeAgent)
	if err != nil {
		return ConfigUpdateRequest{}, err
	}
	phaseLog.log("build workspace config", "workspace_repo_count", len(workspace.Repos))
	phaseLog.log("complete")
	return ConfigUpdateRequest{
		Definition: def,
		RuntimeEnv: env,
		Workspace:  &workspace,
	}, nil
}

func BuildRuntimeEnvWithProxyToken(ctx context.Context, deps CompileDeps, agent *model.Agent, sb *model.Sandbox, runtimeSecret string, token *ProxyTokenResult, channelID uuid.UUID) (map[string]string, error) {
	env := make(map[string]string)
	if agent == nil {
		return env, nil
	}
	if token == nil || token.Token == "" || token.JTI == "" {
		return nil, fmt.Errorf("runtime env proxy token is required")
	}

	// Merge user env first so the reserved HIVY_ keys written below always win.
	if channelID == uuid.Nil {
		channelID = resolveChannelIDForSandbox(ctx, deps, agent, sb)
	}
	if err := mergeChannelEnvVars(ctx, deps, env, channelID); err != nil {
		return nil, err
	}

	if sb != nil {
		env[AgentEnvSandboxID] = sb.ID.String()
	}
	env[AgentEnvRuntimeSecret] = runtimeSecret
	env[AgentEnvDriveUploadBearer] = runtimeSecret
	env[AgentEnvAgentID] = agent.ID.String()
	if agent.OrgID != nil {
		env[AgentEnvOrgID] = agent.OrgID.String()
	}
	if deps.Cfg != nil {
		env[AgentEnvAgentBaseURL] = deps.Cfg.ProxyOpenAIBaseURL()
		env[AgentEnvWorkspaceRoot] = runtimeWorkspaceRoot
		env[AgentEnvDBPath] = runtimeDBPath
		env[AgentEnvRuntimeBindAddr] = fmt.Sprintf("0.0.0.0:%d", runtimePort)
	}
	modelID := strings.TrimSpace(agent.Model)
	if modelID == "" {
		modelID = DefaultAgentModel
	}
	env[AgentEnvAgentModel] = modelID
	env[AgentEnvAgentAPIKeyEnv] = ProxyAPIKeyEnv
	// Provision a tunnel password so the tunnel proxy fails closed (it is an open
	// proxy to every sandbox localhost port when unset).
	env[AgentEnvTunnelPassword] = runtimeSecret
	env[ProxyAPIKeyEnv] = token.Token
	addControlPlaneRuntimeEnv(ctx, deps, env, agent, sb, runtimeSecret)
	if deps.Cfg != nil && sb != nil {
		env[AgentEnvRuntimeEventWSURL] = RuntimeEventWebSocketURL(deps.Cfg, sb.ID)
	}

	return env, nil
}

// mergeChannelEnvVars decrypts and merges the channel's user-supplied env vars
// into env, each injected as __ENV__<NAME>. The sandbox runtime strips the
// prefix before exposing the clean name to the workload. It must run before the
// reserved control-plane keys are written so those keys stay authoritative.
func mergeChannelEnvVars(ctx context.Context, deps CompileDeps, env map[string]string, channelID uuid.UUID) error {
	if channelID == uuid.Nil {
		return nil
	}
	if deps.DB == nil {
		return nil
	}
	if deps.EncKey == nil {
		return fmt.Errorf("runtime env decrypt: encryption key is required")
	}
	var vars []model.ChannelEnvVar
	if err := deps.DB.WithContext(ctx).
		Where("channel_id = ?", channelID).
		Find(&vars).Error; err != nil {
		return fmt.Errorf("load channel env vars: %w", err)
	}
	for _, v := range vars {
		value, err := deps.EncKey.DecryptString(v.EncryptedValue)
		if err != nil {
			return fmt.Errorf("decrypt channel env var %q: %w", v.Name, err)
		}
		env[channelEnvInjectPrefix+v.Name] = value
	}
	return nil
}

// channelEnvInjectPrefix marks user-supplied env vars in the runtime env. The
// sandbox runtime strips it so the workload sees the clean name.
const channelEnvInjectPrefix = "__ENV__"

// resolveChannelIDForSandbox recovers the channel of the sandbox's session.
// Used by every push path except session creation, where the session↔sandbox
// link is not yet persisted and the channel is passed explicitly.
func resolveChannelIDForSandbox(ctx context.Context, deps CompileDeps, agent *model.Agent, sb *model.Sandbox) uuid.UUID {
	if deps.DB == nil || sb == nil || agent == nil || agent.OrgID == nil {
		return uuid.Nil
	}
	var session model.Session
	err := deps.DB.WithContext(ctx).
		Where("sandbox_id = ? AND org_id = ? AND agent_id = ? AND status <> ?", sb.ID, *agent.OrgID, agent.ID, "archived").
		Order("created_at DESC").
		First(&session).Error
	if err != nil {
		return uuid.Nil
	}
	return session.ChannelID
}

func RuntimeEventWebSocketURL(cfg *config.Config, sandboxID uuid.UUID) string {
	if cfg == nil || sandboxID == uuid.Nil {
		return ""
	}
	base := strings.TrimRight(cfg.RuntimeControlPlaneBaseURL(), "/")
	parsed, err := url.Parse(base)
	if err == nil && parsed.Scheme != "" {
		switch parsed.Scheme {
		case "http":
			parsed.Scheme = "ws"
		case "https":
			parsed.Scheme = "wss"
		}
		base = strings.TrimRight(parsed.String(), "/")
	} else if strings.HasPrefix(base, "http://") {
		base = "ws://" + strings.TrimPrefix(base, "http://")
	} else if strings.HasPrefix(base, "https://") {
		base = "wss://" + strings.TrimPrefix(base, "https://")
	}
	return fmt.Sprintf("%s/internal/runtime-events/sandboxes/%s/sessions/{session_id}/ws", base, sandboxID)
}

func RuntimeTurnStateWebhookURL(cfg *config.Config, sandboxID uuid.UUID) string {
	if cfg == nil || sandboxID == uuid.Nil {
		return ""
	}
	base := strings.TrimRight(cfg.RuntimeControlPlaneBaseURL(), "/")
	return fmt.Sprintf("%s/internal/runtime-events/sandboxes/%s/turn-state", base, sandboxID)
}

func AgentGitUsername(agent *model.Agent) string {
	if agent == nil {
		return "agent"
	}
	if username := sanitizeGitIdentity(agent.Name); username != "" {
		return username
	}
	return "hivy"
}

func AgentGitEmail(agent *model.Agent) string {
	return AgentGitUsername(agent) + "@users.noreply.github.com"
}

func sanitizeGitIdentity(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
