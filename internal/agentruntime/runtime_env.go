package agentruntime

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/agentenvaccess"
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
	MCPContext      MCPRuntimeContext
	// SessionID, ProvisioningAttemptID, and TraceID form the observability
	// correlation contract for session-scoped sandbox provisioning.
	SessionID             uuid.UUID
	ProvisioningAttemptID uuid.UUID
	TraceID               string
	// TeamID scopes which team env vars are injected. When Nil, the builder
	// resolves it from the sandbox's session channel. Session creation sets it
	// explicitly because the session↔sandbox link is not yet persisted at the
	// first push.
	TeamID uuid.UUID
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
	teamID := opts.TeamID
	if teamID == uuid.Nil {
		teamID = resolveTeamIDForSandbox(ctx, deps, runtimeAgent, sb)
	}
	env, err := BuildRuntimeEnvWithProxyToken(ctx, deps, runtimeAgent, sb, runtimeSecret, token, teamID)
	if err != nil {
		return ConfigUpdateRequest{}, err
	}
	if opts.SessionID != uuid.Nil {
		env[AgentEnvSessionID] = opts.SessionID.String()
	}
	if opts.ProvisioningAttemptID != uuid.Nil {
		env[AgentEnvProvisioningAttemptID] = opts.ProvisioningAttemptID.String()
	}
	if traceID := strings.TrimSpace(opts.TraceID); traceID != "" {
		env[AgentEnvTraceID] = traceID
	}
	phaseLog.log("build runtime env", "env_key_count", len(env))
	opts.TeamID = teamID
	def, err := CompileWithProxyTokenOptions(ctx, deps, runtimeAgent, token, opts)
	if err != nil {
		return ConfigUpdateRequest{}, err
	}
	orgID := uuid.Nil
	agentID := uuid.Nil
	if runtimeAgent != nil && runtimeAgent.OrgID != nil {
		orgID = *runtimeAgent.OrgID
		agentID = runtimeAgent.ID
	}
	if err := appendTeamEnvVarPromptDoc(ctx, deps, def, orgID, teamID, agentID); err != nil {
		return ConfigUpdateRequest{}, err
	}
	if err := appendTeamKnowledgeSourcePromptDoc(ctx, deps, def, orgID, teamID); err != nil {
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

func BuildRuntimeEnvWithProxyToken(ctx context.Context, deps CompileDeps, agent *model.Agent, sb *model.Sandbox, runtimeSecret string, token *ProxyTokenResult, teamID uuid.UUID) (map[string]string, error) {
	env := make(map[string]string)
	if agent == nil {
		return env, nil
	}
	if token == nil || token.Token == "" || token.JTI == "" {
		return nil, fmt.Errorf("runtime env proxy token is required")
	}

	// Merge user env first so the reserved HIVY_ keys written below always win.
	if teamID == uuid.Nil {
		teamID = resolveTeamIDForSandbox(ctx, deps, agent, sb)
	}
	orgID := uuid.Nil
	if agent.OrgID != nil {
		orgID = *agent.OrgID
	}
	if err := mergeTeamEnvVars(ctx, deps, env, orgID, teamID, agent.ID); err != nil {
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

// mergeTeamEnvVars decrypts and merges the team's user-supplied env vars
// into env, each injected as __ENV__<NAME>. The sandbox runtime strips the
// prefix before exposing the clean name to the workload. It must run before the
// reserved control-plane keys are written so those keys stay authoritative.
func mergeTeamEnvVars(ctx context.Context, deps CompileDeps, env map[string]string, orgID, teamID, agentID uuid.UUID) error {
	if orgID == uuid.Nil || teamID == uuid.Nil {
		return nil
	}
	if deps.DB == nil {
		return nil
	}
	if deps.EncKey == nil {
		return fmt.Errorf("runtime env decrypt: encryption key is required")
	}
	vars, err := agentenvaccess.EnabledTeamEnvVars(ctx, deps.DB, orgID, teamID, agentID)
	if err != nil {
		return fmt.Errorf("load team env vars: %w", err)
	}
	for _, v := range vars {
		value, err := deps.EncKey.DecryptString(v.EncryptedValue)
		if err != nil {
			return fmt.Errorf("decrypt team env var %q: %w", v.Name, err)
		}
		env[teamEnvInjectPrefix+v.Name] = value
	}
	return nil
}

// teamEnvInjectPrefix marks user-supplied env vars in the runtime env. The
// sandbox runtime strips it so the workload sees the clean name.
const teamEnvInjectPrefix = "__ENV__"

// resolveTeamIDForSandbox recovers the team of the sandbox session.
// Used by every push path except session creation, where the session↔sandbox
// link is not yet persisted and the team is passed explicitly.
func resolveTeamIDForSandbox(ctx context.Context, deps CompileDeps, agent *model.Agent, sb *model.Sandbox) uuid.UUID {
	if deps.DB == nil || sb == nil || agent == nil || agent.OrgID == nil {
		return uuid.Nil
	}
	var rawTeamID string
	err := deps.DB.WithContext(ctx).
		Table("sessions").
		Select("sessions.team_id").
		Where("sessions.sandbox_id = ? AND sessions.org_id = ? AND sessions.agent_id = ? AND sessions.status <> ?", sb.ID, *agent.OrgID, agent.ID, "archived").
		Order("sessions.created_at DESC").
		Limit(1).
		Scan(&rawTeamID).Error
	if err != nil {
		return uuid.Nil
	}
	teamID, err := uuid.Parse(rawTeamID)
	if err != nil {
		return uuid.Nil
	}
	return teamID
}
