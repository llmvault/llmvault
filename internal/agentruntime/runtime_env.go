package agentruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

const (
	runtimeWorkspaceRoot = "/workspace"
	runtimeDBPath        = "/app/data/hivy-sandboxes-runtime.db"
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
	sandboxID := uuid.Nil
	if sb != nil {
		sandboxID = sb.ID
	}
	token, err := MintProxyToken(ctx, deps, agent, sandboxID)
	if err != nil {
		return ConfigUpdateRequest{}, nil, err
	}
	env, err := BuildRuntimeEnvWithProxyToken(ctx, deps, agent, sb, runtimeSecret, token)
	if err != nil {
		return ConfigUpdateRequest{}, token, err
	}
	def, err := CompileWithProxyToken(ctx, deps, agent, token)
	if err != nil {
		return ConfigUpdateRequest{}, token, err
	}
	def.OutboundChannels = ControlPlaneOutboundChannels(deps.Cfg, sandboxID)
	schedules, err := BuildRuntimeSchedules(ctx, deps.DB, agent, sb)
	if err != nil {
		return ConfigUpdateRequest{}, token, err
	}
	return ConfigUpdateRequest{
		Definition: def,
		RuntimeEnv: env,
		Schedules:  schedules,
	}, token, nil
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
		env[AgentEnvAgentMultimodalBaseURL] = deps.Cfg.ProxyOpenAIBaseURL()
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
	env[AgentEnvAgentMultimodalModel] = DefaultAgentMultimodalModel
	env[AgentEnvAgentMultimodalAPIKeyEnv] = ProxyAPIKeyEnv
	// Provision a tunnel password so the tunnel proxy fails closed (it is an open
	// proxy to every sandbox localhost port when unset).
	env[AgentEnvTunnelPassword] = runtimeSecret
	env[ProxyAPIKeyEnv] = token.Token
	addControlPlaneRuntimeEnv(ctx, deps, env, agent, runtimeSecret)

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

func addAgentRuntimeEnv(ctx context.Context, deps CompileDeps, env map[string]string, agent *model.Agent, runtimeSecret string) error {
	if err := mergeAgentEnvVars(deps, env, agent); err != nil {
		return err
	}
	addControlPlaneRuntimeEnv(ctx, deps, env, agent, runtimeSecret)
	return nil
}

func addControlPlaneRuntimeEnv(ctx context.Context, deps CompileDeps, env map[string]string, agent *model.Agent, runtimeSecret string) {
	if env == nil || deps.Cfg == nil || agent == nil || agent.ID == uuid.Nil || runtimeSecret == "" {
		return
	}
	opts := ControlPlaneRuntimeEnvOptions{}
	if deps.DB != nil && agent.OrgID != nil {
		opts.BugsinkDashboardBaseURL = BugsinkDashboardBaseURL(ctx, deps.DB, *agent.OrgID, *agent)
		opts.GlitchTipDashboardBaseURL = GlitchTipDashboardBaseURL(ctx, deps.DB, *agent.OrgID, *agent)
	}
	ApplyControlPlaneRuntimeEnv(env, deps.Cfg, agent, runtimeSecret, opts)
}

type ControlPlaneRuntimeEnvOptions struct {
	GitUsername               string
	GitEmail                  string
	BugsinkDashboardBaseURL   string
	GlitchTipDashboardBaseURL string
}

func ApplyControlPlaneRuntimeEnv(env map[string]string, cfg *config.Config, agent *model.Agent, runtimeSecret string, opts ControlPlaneRuntimeEnvOptions) {
	if env == nil || cfg == nil || agent == nil {
		return
	}
	controlPlaneBaseURL := cfg.RuntimeControlPlaneBaseURL()
	env[AgentEnvCloudControlPlaneURL] = controlPlaneBaseURL
	if agent.ID != uuid.Nil {
		env[AgentEnvGitCredentialsURL] = fmt.Sprintf("%s/internal/git-credentials/%s", controlPlaneBaseURL, agent.ID)
		env[AgentEnvDriveUploadURL] = AgentDriveUploadURL(controlPlaneBaseURL, agent.ID)
		if runtimeSecret != "" {
			ApplyServiceProxyEnv(env, controlPlaneBaseURL, agent.ID, runtimeSecret)
		}
	}
	env[AgentEnvGitHubNoKeyring] = "1"
	env[AgentEnvGitUsername] = strings.TrimSpace(opts.GitUsername)
	if env[AgentEnvGitUsername] == "" {
		env[AgentEnvGitUsername] = AgentGitUsername(agent)
	}
	env[AgentEnvGitEmail] = strings.TrimSpace(opts.GitEmail)
	if env[AgentEnvGitEmail] == "" {
		env[AgentEnvGitEmail] = AgentGitEmail(agent)
	}
	if strings.TrimSpace(opts.BugsinkDashboardBaseURL) != "" {
		env[AgentEnvBugsinkDashboardBaseURL] = strings.TrimSpace(opts.BugsinkDashboardBaseURL)
	}
	if strings.TrimSpace(opts.GlitchTipDashboardBaseURL) != "" {
		env[AgentEnvGlitchTipDashboardBaseURL] = strings.TrimSpace(opts.GlitchTipDashboardBaseURL)
	}
	ApplySandboxSentryEnv(env, cfg, cfg.AgentSandboxSentryDSN)
}

func ApplySandboxSentryEnv(env map[string]string, cfg *config.Config, dsn string) {
	if env == nil || cfg == nil || strings.TrimSpace(dsn) == "" {
		return
	}
	env[AgentEnvSentryDSN] = strings.TrimSpace(dsn)
	env[AgentEnvSentryEnvironment] = cfg.Environment
	env[AgentEnvSentrySampleRate] = "1"
	env[AgentEnvSentryTracesSampleRate] = fmt.Sprintf("%g", cfg.SentryTracesSampleRate)
	env[AgentEnvSentryEnableLogs] = "true"
	if strings.TrimSpace(cfg.SentryRelease) != "" {
		env[AgentEnvSentryRelease] = cfg.SentryRelease
	}
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
