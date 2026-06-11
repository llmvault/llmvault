package sandbox

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/model"
)

func employeeSandboxEnvVars(cfg *config.Config, runtimeSecret string, sb *model.Sandbox, orgID uuid.UUID, agent *model.Employee, secrets *employeeruntime.StartupSecrets, gitIdentity *employeeGitIdentity, bugsinkDashboardURL string, glitchTipDashboardURL string) map[string]string {
	controlPlaneBaseURL := cfg.RuntimeControlPlaneBaseURL()
	proxyBaseURL := cfg.ProxyOpenAIBaseURL()
	envVars := map[string]string{
		employeeruntime.EmployeeEnvRuntimeSecret:            runtimeSecret,
		employeeruntime.ProxyAPIKeyEnv:                      secrets.ProxyToken,
		employeeruntime.EmployeeEnvAgentModel:               employeeruntime.DefaultEmployeeModel,
		employeeruntime.EmployeeEnvAgentBaseURL:             proxyBaseURL,
		employeeruntime.EmployeeEnvAgentAPIKeyEnv:           employeeruntime.ProxyAPIKeyEnv,
		employeeruntime.EmployeeEnvAgentMultimodalModel:     employeeruntime.DefaultEmployeeMultimodalModel,
		employeeruntime.EmployeeEnvAgentMultimodalBaseURL:   proxyBaseURL,
		employeeruntime.EmployeeEnvAgentMultimodalAPIKeyEnv: employeeruntime.ProxyAPIKeyEnv,
		employeeruntime.EmployeeEnvEmployeeID:               agent.ID.String(),
		employeeruntime.EmployeeEnvCloudControlPlaneURL:     controlPlaneBaseURL,
		employeeruntime.EmployeeEnvDriveUploadBearer:        runtimeSecret,
		employeeruntime.EmployeeEnvWorkspaceRoot:            "/workspace",
		employeeruntime.EmployeeEnvDBPath:                   "/app/data/hivy-sandboxes-runtime.db",
		employeeruntime.EmployeeEnvRuntimeBindAddr:          fmt.Sprintf("0.0.0.0:%d", EmployeeSandboxPort),
		employeeruntime.EmployeeEnvRuntimeMode:              "employee",
		// Provision a tunnel password so the tunnel proxy fails closed (an open proxy
		// to every sandbox localhost port when unset).
		employeeruntime.EmployeeEnvTunnelPassword: runtimeSecret,
		employeeruntime.EmployeeEnvSandboxID:      sb.ID.String(),
		employeeruntime.EmployeeEnvOrgID:          orgID.String(),
	}
	opts := employeeruntime.ControlPlaneRuntimeEnvOptions{
		GitUsername:               employeeGitUsername(agent, gitIdentity),
		GitEmail:                  employeeGitEmail(agent, gitIdentity),
		BugsinkDashboardBaseURL:   bugsinkDashboardURL,
		GlitchTipDashboardBaseURL: glitchTipDashboardURL,
	}
	employeeruntime.ApplyControlPlaneRuntimeEnv(envVars, cfg, agent, runtimeSecret, opts)
	return envVars
}

func buildEmployeeSandboxName(agent *model.Employee) string {
	return fmt.Sprintf("hivy-employee-%s-%s-%d", sanitizeName(agent.Name), shortID(agent.ID), time.Now().Unix())
}

func buildSpecialistRuntimeSandboxName(agent *model.Employee) string {
	return fmt.Sprintf("hivy-specialist-%s-%s-%d", sanitizeName(agent.Name), shortID(agent.ID), time.Now().Unix())
}
