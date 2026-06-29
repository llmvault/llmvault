package agentruntime

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type ServiceProxyEnvSpec struct {
	Provider   string
	SkillName  string
	BaseURLEnv string
	AuthEnv    string
	Path       string
}

var serviceProxyEnvSpecs = []ServiceProxyEnvSpec{
	{Provider: "bugsink", SkillName: "bugsink", BaseURLEnv: AgentEnvBugsinkURL, AuthEnv: AgentEnvBugsinkToken, Path: "/internal/bugsink-proxy/%s"},
	{Provider: "glitchtip", SkillName: "glitchtip", BaseURLEnv: AgentEnvGlitchTipURL, AuthEnv: AgentEnvGlitchTipToken, Path: "/internal/glitchtip-proxy/%s"},
	{Provider: "linear", SkillName: "linear", BaseURLEnv: AgentEnvLinearURL, AuthEnv: AgentEnvLinearToken, Path: "/internal/linear-proxy/%s"},
	{Provider: "notion", SkillName: "notion", BaseURLEnv: AgentEnvNotionAPIURL, AuthEnv: AgentEnvNotionToken, Path: "/internal/notion-proxy/%s"},
	{Provider: "railway", SkillName: "railway", BaseURLEnv: AgentEnvRailwayAPIURL, AuthEnv: AgentEnvRailwayAPIKey, Path: "/internal/railway-proxy/%s"},
	{Provider: "vercel", SkillName: "vercel", BaseURLEnv: AgentEnvVercelAPIURL, AuthEnv: AgentEnvVercelAPIKey, Path: "/internal/vercel-proxy/%s"},
	{Provider: "slack", SkillName: "slack", BaseURLEnv: AgentEnvSlackAPIURL, AuthEnv: AgentEnvSlackToken, Path: "/internal/slack-proxy/%s"},
	{Provider: "postgres", SkillName: "postgres", BaseURLEnv: AgentEnvPostgresURL, AuthEnv: AgentEnvPostgresToken, Path: "/internal/database-proxy/postgres/%s"},
	{Provider: "mysql", SkillName: "mysql", BaseURLEnv: AgentEnvMySQLURL, AuthEnv: AgentEnvMySQLToken, Path: "/internal/database-proxy/mysql/%s"},
	{Provider: "mongodb", SkillName: "mongodb", BaseURLEnv: AgentEnvMongoDBURL, AuthEnv: AgentEnvMongoDBToken, Path: "/internal/database-proxy/mongodb/%s"},
	{Provider: "redis", SkillName: "redis", BaseURLEnv: AgentEnvRedisURL, AuthEnv: AgentEnvRedisToken, Path: "/internal/database-proxy/redis/%s"},
}

func ServiceProxyEnvSpecs() []ServiceProxyEnvSpec {
	out := make([]ServiceProxyEnvSpec, len(serviceProxyEnvSpecs))
	copy(out, serviceProxyEnvSpecs)
	return out
}

func ApplyServiceProxyEnv(env map[string]string, controlPlaneBaseURL string, agentID uuid.UUID, runtimeSecret string) {
	if env == nil || agentID == uuid.Nil || strings.TrimSpace(controlPlaneBaseURL) == "" || runtimeSecret == "" {
		return
	}
	base := strings.TrimRight(controlPlaneBaseURL, "/")
	for _, spec := range serviceProxyEnvSpecs {
		env[spec.BaseURLEnv] = base + fmt.Sprintf(spec.Path, agentID)
		env[spec.AuthEnv] = runtimeSecret
	}
}

func AgentDriveUploadURL(controlPlaneBaseURL string, agentID, sandboxID uuid.UUID) string {
	if sandboxID == uuid.Nil {
		return ""
	}
	return fmt.Sprintf("%s/internal/agents/%s/sandboxes/%s/drive", strings.TrimRight(controlPlaneBaseURL, "/"), agentID, sandboxID)
}
