package agentruntime

import (
	"context"
	"encoding/json"
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
	return BuildRuntimeEnvWithProxyToken(ctx, deps, agent, sb, runtimeSecret, token)
}

func BuildAgentRuntimeConfigUpdate(ctx context.Context, deps CompileDeps, agent *model.Agent, sb *model.Sandbox, runtimeSecret string) (ConfigUpdateRequest, *ProxyTokenResult, error) {
	return BuildAgentRuntimeConfigUpdateWithOptions(ctx, deps, agent, sb, runtimeSecret, RuntimeConfigOptions{})
}

type RuntimeConfigOptions struct {
	ModelID         string
	ReasoningEffort string
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
	env, err := BuildRuntimeEnvWithProxyToken(ctx, deps, runtimeAgent, sb, runtimeSecret, token)
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
		"skill_count", len(def.Skills),
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

func BuildRuntimeEnvWithProxyToken(ctx context.Context, deps CompileDeps, agent *model.Agent, sb *model.Sandbox, runtimeSecret string, token *ProxyTokenResult) (map[string]string, error) {
	env := make(map[string]string)
	if agent == nil {
		return env, nil
	}
	if token == nil || token.Token == "" || token.JTI == "" {
		return nil, fmt.Errorf("runtime env proxy token is required")
	}

	// Merge user env first so the reserved HIVY_ keys written below always win.
	if err := mergeAgentEnvVars(deps, env, agent); err != nil {
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

// mergeAgentEnvVars decrypts and merges org-supplied env vars into env. It must
// run before the reserved control-plane keys are written so a user HIVY_* key is
// overwritten by the authoritative value.
func mergeAgentEnvVars(deps CompileDeps, env map[string]string, agent *model.Agent) error {
	if len(agent.EncryptedEnvVars) == 0 {
		return nil
	}
	if deps.EncKey == nil {
		return fmt.Errorf("runtime env decrypt: encryption key is required")
	}
	decrypted, err := deps.EncKey.DecryptString(agent.EncryptedEnvVars)
	if err != nil {
		return err
	}
	decrypted = strings.TrimSpace(decrypted)
	if decrypted == "" {
		return nil
	}
	rawEnv := map[string]string{}
	if err := json.Unmarshal([]byte(decrypted), &rawEnv); err != nil {
		return fmt.Errorf("decode env vars: %w", err)
	}
	for key, value := range rawEnv {
		env[key] = value
	}
	return nil
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
