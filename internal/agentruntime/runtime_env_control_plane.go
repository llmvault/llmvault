package agentruntime

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

func addControlPlaneRuntimeEnv(ctx context.Context, deps CompileDeps, env map[string]string, agent *model.Agent, sb *model.Sandbox, runtimeSecret string) {
	if env == nil || deps.Cfg == nil || agent == nil || agent.ID == uuid.Nil || runtimeSecret == "" {
		return
	}
	opts := ControlPlaneRuntimeEnvOptions{}
	if sb != nil {
		opts.SandboxID = sb.ID
	}
	if deps.DB != nil && agent.OrgID != nil {
		opts.BugsinkDashboardBaseURL = BugsinkDashboardBaseURL(ctx, deps.DB, *agent.OrgID, *agent)
		opts.GlitchTipDashboardBaseURL = GlitchTipDashboardBaseURL(ctx, deps.DB, *agent.OrgID, *agent)
	}
	if allowed, err := AllowedServiceProxyProviders(ctx, deps.DB, *agent); err == nil {
		opts.AllowedServiceProxyProviders = allowed
	}
	ApplyControlPlaneRuntimeEnv(env, deps.Cfg, agent, runtimeSecret, opts)
}

type ControlPlaneRuntimeEnvOptions struct {
	SandboxID                    uuid.UUID
	GitUsername                  string
	GitEmail                     string
	BugsinkDashboardBaseURL      string
	GlitchTipDashboardBaseURL    string
	AllowedServiceProxyProviders map[string]bool
}

func ApplyControlPlaneRuntimeEnv(env map[string]string, cfg *config.Config, agent *model.Agent, runtimeSecret string, opts ControlPlaneRuntimeEnvOptions) {
	if env == nil || cfg == nil || agent == nil {
		return
	}
	controlPlaneBaseURL := cfg.RuntimeControlPlaneBaseURL()
	env[AgentEnvCloudControlPlaneURL] = controlPlaneBaseURL
	if agent.ID != uuid.Nil {
		env[AgentEnvGitCredentialsURL] = fmt.Sprintf("%s/internal/git-credentials/%s", controlPlaneBaseURL, agent.ID)
		if opts.SandboxID != uuid.Nil {
			env[AgentEnvDriveUploadURL] = AgentDriveUploadURL(controlPlaneBaseURL, agent.ID, opts.SandboxID)
		}
		if runtimeSecret != "" {
			ApplyServiceProxyEnv(env, controlPlaneBaseURL, agent.ID, runtimeSecret, opts.AllowedServiceProxyProviders)
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
