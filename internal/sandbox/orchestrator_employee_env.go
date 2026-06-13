package sandbox

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

func employeeSandboxEnvVars(cfg *config.Config, runtimeSecret string, sb *model.Sandbox, orgID uuid.UUID, agent *model.Employee, secrets *agentruntime.StartupSecrets, gitIdentity *employeeGitIdentity, bugsinkDashboardURL string, glitchTipDashboardURL string) map[string]string {
	controlPlaneBaseURL := cfg.RuntimeControlPlaneBaseURL()
	proxyBaseURL := cfg.ProxyOpenAIBaseURL()
	envVars := map[string]string{
		agentruntime.EmployeeEnvRuntimeSecret:            runtimeSecret,
		agentruntime.ProxyAPIKeyEnv:                      secrets.ProxyToken,
		agentruntime.EmployeeEnvAgentModel:               agentruntime.DefaultEmployeeModel,
		agentruntime.EmployeeEnvAgentBaseURL:             proxyBaseURL,
		agentruntime.EmployeeEnvAgentAPIKeyEnv:           agentruntime.ProxyAPIKeyEnv,
		agentruntime.EmployeeEnvAgentMultimodalModel:     agentruntime.DefaultEmployeeMultimodalModel,
		agentruntime.EmployeeEnvAgentMultimodalBaseURL:   proxyBaseURL,
		agentruntime.EmployeeEnvAgentMultimodalAPIKeyEnv: agentruntime.ProxyAPIKeyEnv,
		agentruntime.EmployeeEnvEmployeeID:               agent.ID.String(),
		agentruntime.EmployeeEnvCloudControlPlaneURL:     controlPlaneBaseURL,
		agentruntime.EmployeeEnvDriveUploadBearer:        runtimeSecret,
		agentruntime.EmployeeEnvWorkspaceRoot:            "/workspace",
		agentruntime.EmployeeEnvDBPath:                   "/app/data/hivy-sandboxes-runtime.db",
		agentruntime.EmployeeEnvRuntimeBindAddr:          fmt.Sprintf("0.0.0.0:%d", EmployeeSandboxPort),
		// Provision a tunnel password so the tunnel proxy fails closed (an open proxy
		// to every sandbox localhost port when unset).
		agentruntime.EmployeeEnvTunnelPassword: runtimeSecret,
		agentruntime.EmployeeEnvSandboxID:      sb.ID.String(),
		agentruntime.EmployeeEnvOrgID:          orgID.String(),
	}
	opts := agentruntime.ControlPlaneRuntimeEnvOptions{
		GitUsername:               employeeGitUsername(agent, gitIdentity),
		GitEmail:                  employeeGitEmail(agent, gitIdentity),
		BugsinkDashboardBaseURL:   bugsinkDashboardURL,
		GlitchTipDashboardBaseURL: glitchTipDashboardURL,
	}
	agentruntime.ApplyControlPlaneRuntimeEnv(envVars, cfg, agent, runtimeSecret, opts)
	return envVars
}

func buildEmployeeSandboxName(agent *model.Employee) string {
	return fmt.Sprintf("hivy-employee-%s-%s-%d", sanitizeName(agent.Name), shortID(agent.ID), time.Now().Unix())
}
