package sandbox

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

func agentSandboxEnvVars(ctx context.Context, db *gorm.DB, cfg *config.Config, runtimeSecret string, sb *model.Sandbox, orgID uuid.UUID, agent *model.Agent, secrets *agentruntime.StartupSecrets, gitIdentity *agentGitIdentity, bugsinkDashboardURL string, glitchTipDashboardURL string) map[string]string {
	controlPlaneBaseURL := cfg.RuntimeControlPlaneBaseURL()
	proxyBaseURL := cfg.ProxyOpenAIBaseURL()
	envVars := map[string]string{
		agentruntime.AgentEnvRuntimeSecret:        runtimeSecret,
		agentruntime.ProxyAPIKeyEnv:               secrets.ProxyToken,
		agentruntime.AgentEnvAgentModel:           agentruntime.DefaultAgentModel,
		agentruntime.AgentEnvAgentBaseURL:         proxyBaseURL,
		agentruntime.AgentEnvAgentAPIKeyEnv:       agentruntime.ProxyAPIKeyEnv,
		agentruntime.AgentEnvAgentID:              agent.ID.String(),
		agentruntime.AgentEnvCloudControlPlaneURL: controlPlaneBaseURL,
		agentruntime.AgentEnvDriveUploadBearer:    runtimeSecret,
		agentruntime.AgentEnvWorkspaceRoot:        "/workspace",
		agentruntime.AgentEnvDBPath:               agentruntime.AgentRuntimeDBPath,
		agentruntime.AgentEnvRuntimeBindAddr:      fmt.Sprintf("0.0.0.0:%d", AgentSandboxPort),
		// Provision a tunnel password so the tunnel proxy fails closed (an open proxy
		// to every sandbox localhost port when unset).
		agentruntime.AgentEnvTunnelPassword: runtimeSecret,
		agentruntime.AgentEnvSandboxID:      sb.ID.String(),
		agentruntime.AgentEnvOrgID:          orgID.String(),
	}
	opts := agentruntime.ControlPlaneRuntimeEnvOptions{
		SandboxID:                 sb.ID,
		GitUsername:               agentGitUsername(agent, gitIdentity),
		GitEmail:                  agentGitEmail(agent, gitIdentity),
		BugsinkDashboardBaseURL:   bugsinkDashboardURL,
		GlitchTipDashboardBaseURL: glitchTipDashboardURL,
	}
	if allowed, err := agentruntime.AllowedServiceProxyProviders(ctx, db, *agent); err == nil {
		opts.AllowedServiceProxyProviders = allowed
	}
	agentruntime.ApplyControlPlaneRuntimeEnv(envVars, cfg, agent, runtimeSecret, opts)
	return envVars
}

func buildAgentSandboxName(agent *model.Agent) string {
	return fmt.Sprintf("hivy-agent-%s-%s-%d", sanitizeName(agent.Name), shortID(agent.ID), time.Now().Unix())
}
