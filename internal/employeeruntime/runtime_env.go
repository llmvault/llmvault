package employeeruntime

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

func BuildRuntimeEnv(ctx context.Context, deps CompileDeps, agent *model.Employee, sb *model.Sandbox, runtimeSecret string) (map[string]string, error) {
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

func BuildEmployeeRuntimeConfigUpdate(ctx context.Context, deps CompileDeps, agent *model.Employee, sb *model.Sandbox, runtimeSecret string) (ConfigUpdateRequest, *ProxyTokenResult, error) {
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

func BuildRuntimeEnvWithProxyToken(ctx context.Context, deps CompileDeps, agent *model.Employee, sb *model.Sandbox, runtimeSecret string, token *ProxyTokenResult) (map[string]string, error) {
	env := make(map[string]string)
	if agent == nil {
		return env, nil
	}
	if token == nil || token.Token == "" || token.JTI == "" {
		return nil, fmt.Errorf("runtime env proxy token is required")
	}
	if sb != nil {
		env[EmployeeEnvSandboxID] = sb.ID.String()
	}
	env[EmployeeEnvRuntimeSecret] = runtimeSecret
	env[EmployeeEnvDriveUploadBearer] = runtimeSecret
	env[EmployeeEnvEmployeeID] = agent.ID.String()
	if agent.OrgID != nil {
		env[EmployeeEnvOrgID] = agent.OrgID.String()
	}
	if deps.Cfg != nil {
		env[EmployeeEnvAgentBaseURL] = deps.Cfg.ProxyOpenAIBaseURL()
		env[EmployeeEnvAgentMultimodalBaseURL] = deps.Cfg.ProxyOpenAIBaseURL()
		env[EmployeeEnvWorkspaceRoot] = runtimeWorkspaceRoot
		env[EmployeeEnvDBPath] = runtimeDBPath
		env[EmployeeEnvRuntimeBindAddr] = fmt.Sprintf("0.0.0.0:%d", runtimePort)
	}
	env[EmployeeEnvAgentModel] = DefaultEmployeeModel
	env[EmployeeEnvAgentAPIKeyEnv] = ProxyAPIKeyEnv
	env[EmployeeEnvAgentMultimodalModel] = DefaultEmployeeMultimodalModel
	env[EmployeeEnvAgentMultimodalAPIKeyEnv] = ProxyAPIKeyEnv
	env[EmployeeEnvRuntimeMode] = "employee"
	env[ProxyAPIKeyEnv] = token.Token

	if err := addAgentRuntimeEnv(ctx, deps, env, agent, runtimeSecret); err != nil {
		return nil, err
	}
	return env, nil
}

func addAgentRuntimeEnv(ctx context.Context, deps CompileDeps, env map[string]string, agent *model.Employee, runtimeSecret string) error {
	if len(agent.EncryptedEnvVars) > 0 {
		if deps.EncKey == nil {
			return fmt.Errorf("runtime env decrypt: encryption key is required")
		}
		decrypted, err := deps.EncKey.DecryptString(agent.EncryptedEnvVars)
		if err != nil {
			return err
		}
		decrypted = strings.TrimSpace(decrypted)
		if decrypted != "" {
			rawEnv := map[string]string{}
			if err := json.Unmarshal([]byte(decrypted), &rawEnv); err != nil {
				return fmt.Errorf("decode env vars: %w", err)
			}
			for key, value := range rawEnv {
				env[key] = value
			}
		}
	}
	addControlPlaneRuntimeEnv(ctx, deps, env, agent, runtimeSecret)
	return nil
}

func addControlPlaneRuntimeEnv(ctx context.Context, deps CompileDeps, env map[string]string, agent *model.Employee, runtimeSecret string) {
	if env == nil || deps.Cfg == nil || agent == nil || agent.ID == uuid.Nil || runtimeSecret == "" {
		return
	}
	opts := ControlPlaneRuntimeEnvOptions{}
	if deps.DB != nil && agent.OrgID != nil {
		opts.BugsinkDashboardBaseURL = BugsinkDashboardBaseURL(ctx, deps.DB, *agent.OrgID, *agent)
	}
	ApplyControlPlaneRuntimeEnv(env, deps.Cfg, agent, runtimeSecret, opts)
}

type ControlPlaneRuntimeEnvOptions struct {
	GitUsername             string
	GitEmail                string
	BugsinkDashboardBaseURL string
}

func ApplyControlPlaneRuntimeEnv(env map[string]string, cfg *config.Config, agent *model.Employee, runtimeSecret string, opts ControlPlaneRuntimeEnvOptions) {
	if env == nil || cfg == nil || agent == nil {
		return
	}
	controlPlaneBaseURL := cfg.RuntimeControlPlaneBaseURL()
	env[EmployeeEnvCloudControlPlaneURL] = controlPlaneBaseURL
	if agent.ID != uuid.Nil {
		env[EmployeeEnvGitCredentialsURL] = fmt.Sprintf("%s/internal/git-credentials/%s", controlPlaneBaseURL, agent.ID)
		env[EmployeeEnvDriveUploadURL] = EmployeeDriveUploadURL(controlPlaneBaseURL, agent.ID)
		if runtimeSecret != "" {
			ApplyServiceProxyEnv(env, controlPlaneBaseURL, agent.ID, runtimeSecret)
		}
	}
	env[EmployeeEnvGitHubNoKeyring] = "1"
	env[EmployeeEnvGitUsername] = strings.TrimSpace(opts.GitUsername)
	if env[EmployeeEnvGitUsername] == "" {
		env[EmployeeEnvGitUsername] = EmployeeGitUsername(agent)
	}
	env[EmployeeEnvGitEmail] = strings.TrimSpace(opts.GitEmail)
	if env[EmployeeEnvGitEmail] == "" {
		env[EmployeeEnvGitEmail] = EmployeeGitEmail(agent)
	}
	if strings.TrimSpace(opts.BugsinkDashboardBaseURL) != "" {
		env[EmployeeEnvBugsinkDashboardBaseURL] = strings.TrimSpace(opts.BugsinkDashboardBaseURL)
	}
	ApplySandboxSentryEnv(env, cfg, cfg.AgentSandboxSentryDSN)
}

func ApplySandboxSentryEnv(env map[string]string, cfg *config.Config, dsn string) {
	if env == nil || cfg == nil || strings.TrimSpace(dsn) == "" {
		return
	}
	env[EmployeeEnvSentryDSN] = strings.TrimSpace(dsn)
	env[EmployeeEnvSentryEnvironment] = cfg.Environment
	env[EmployeeEnvSentrySampleRate] = "1"
	env[EmployeeEnvSentryTracesSampleRate] = fmt.Sprintf("%g", cfg.SentryTracesSampleRate)
	env[EmployeeEnvSentryEnableLogs] = "true"
	if strings.TrimSpace(cfg.SentryRelease) != "" {
		env[EmployeeEnvSentryRelease] = cfg.SentryRelease
	}
}

func EmployeeGitUsername(agent *model.Employee) string {
	if agent == nil {
		return "agent"
	}
	if username := sanitizeGitIdentity(agent.Name); username != "" {
		return username
	}
	return "hivy"
}

func EmployeeGitEmail(agent *model.Employee) string {
	return EmployeeGitUsername(agent) + "@users.noreply.github.com"
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
