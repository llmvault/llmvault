package agentruntime

import (
	"sort"
	"strings"
)

const (
	AgentEnvRuntimeSecret              = "HIVY_RUNTIME_SECRET"
	AgentEnvProxyAPIKey                = "HIVY_PROXY_API_KEY"
	AgentEnvAgentModel                 = "HIVY_AGENT_MODEL"
	AgentEnvAgentBaseURL               = "HIVY_AGENT_BASE_URL"
	AgentEnvAgentAPIKeyEnv             = "HIVY_AGENT_API_KEY_ENV"
	AgentEnvAgentMultimodalModel       = "HIVY_AGENT_MULTIMODAL_MODEL"
	AgentEnvAgentMultimodalBaseURL     = "HIVY_AGENT_MULTIMODAL_BASE_URL"
	AgentEnvAgentMultimodalAPIKeyEnv   = "HIVY_AGENT_MULTIMODAL_API_KEY_ENV"
	AgentEnvAgentID                    = "HIVY_AGENT_ID"
	AgentEnvCloudControlPlaneURL       = "HIVY_CONTROL_PLANE_URL"
	AgentEnvDriveUploadBearer          = "HIVY_DRIVE_UPLOAD_BEARER"
	AgentEnvWorkspaceRoot              = "HIVY_WORKSPACE_ROOT"
	AgentEnvDBPath                     = "HIVY_DB_PATH"
	AgentEnvRuntimeBindAddr            = "HIVY_RUNTIME_BIND_ADDR"
	AgentEnvTunnelPassword             = "HIVY_TUNNEL_PASSWORD"
	AgentEnvSandboxID                  = "HIVY_SANDBOX_ID"
	AgentEnvOrgID                      = "HIVY_ORG_ID"
	AgentEnvGitUsername                = "HIVY_GIT_USERNAME"
	AgentEnvGitEmail                   = "HIVY_GIT_EMAIL"
	AgentEnvGitCredentialsURL          = "HIVY_GIT_CREDENTIALS_URL"
	AgentEnvGitHubNoKeyring            = "GH_NO_KEYRING"
	AgentEnvDriveUploadURL             = "HIVY_DRIVE_UPLOAD_URL"
	AgentEnvBugsinkURL                 = "HIVY_BUGSINK_URL"
	AgentEnvBugsinkDashboardBaseURL    = "HIVY_BUGSINK_DASHBOARD_BASE_URL"
	AgentEnvBugsinkToken               = "HIVY_BUGSINK_TOKEN"
	AgentEnvGlitchTipURL               = "HIVY_GLITCHTIP_URL"
	AgentEnvGlitchTipDashboardBaseURL  = "HIVY_GLITCHTIP_DASHBOARD_BASE_URL"
	AgentEnvGlitchTipToken             = "HIVY_GLITCHTIP_TOKEN"
	AgentEnvLinearURL                  = "HIVY_LINEAR_URL"
	AgentEnvLinearToken                = "HIVY_LINEAR_TOKEN"
	AgentEnvNotionAPIURL               = "HIVY_NOTION_API_URL"
	AgentEnvNotionToken                = "HIVY_NOTION_TOKEN"
	AgentEnvRailwayAPIURL              = "HIVY_RAILWAY_API_URL"
	AgentEnvRailwayAPIKey              = "HIVY_RAILWAY_API_KEY"
	AgentEnvVercelAPIURL               = "HIVY_VERCEL_API_URL"
	AgentEnvVercelAPIKey               = "HIVY_VERCEL_API_KEY"
	AgentEnvSlackAPIURL                = "HIVY_SLACK_API_URL"
	AgentEnvSlackToken                 = "HIVY_SLACK_TOKEN"
	AgentEnvPostgresURL                = "HIVY_POSTGRES_URL"
	AgentEnvPostgresToken              = "HIVY_POSTGRES_TOKEN"
	AgentEnvMySQLURL                   = "HIVY_MYSQL_URL"
	AgentEnvMySQLToken                 = "HIVY_MYSQL_TOKEN"
	AgentEnvMongoDBURL                 = "HIVY_MONGODB_URL"
	AgentEnvMongoDBToken               = "HIVY_MONGODB_TOKEN"
	AgentEnvSentryDSN                  = "SENTRY_DSN"
	AgentEnvSentryEnvironment          = "SENTRY_ENVIRONMENT"
	AgentEnvSentrySampleRate           = "SENTRY_SAMPLE_RATE"
	AgentEnvSentryTracesSampleRate     = "SENTRY_TRACES_SAMPLE_RATE"
	AgentEnvSentryEnableLogs           = "SENTRY_ENABLE_LOGS"
	AgentEnvSentryRelease              = "SENTRY_RELEASE"
	AgentEnvHome                       = "HOME"
	AgentEnvPath                       = "PATH"
	AgentEnvLang                       = "LANG"
	AgentEnvLCAll                      = "LC_ALL"
	AgentEnvSourceControlPlaneInjected = "control_plane_injected"
	AgentEnvSourceConditionalSentry    = "conditional_sentry"
	AgentEnvSourceContainerOS          = "container_os"
	AgentEnvStatusOK                   = "ok"
	AgentEnvStatusMissing              = "missing"
	AgentEnvStatusMissingOptional      = "missing_optional"
	AgentEnvStatusForbidden            = "forbidden"
	AgentEnvStatusUnexpected           = "unexpected"
	AgentEnvValueRedacted              = "<redacted>"
)

// ProxyAPIKeyEnv is kept for callers that already depend on this exported name.
const ProxyAPIKeyEnv = AgentEnvProxyAPIKey

type AgentEnvSpec struct {
	Key       string `json:"key"`
	Source    string `json:"source"`
	Sensitive bool   `json:"sensitive"`
	Forbidden bool   `json:"forbidden"`
	Optional  bool   `json:"optional"`
}

type AgentEnvReportEntry struct {
	Key       string `json:"key"`
	Source    string `json:"source"`
	Set       bool   `json:"set"`
	Sensitive bool   `json:"sensitive"`
	Forbidden bool   `json:"forbidden"`
	Status    string `json:"status"`
	Value     string `json:"value,omitempty"`
	Redacted  bool   `json:"redacted,omitempty"`
}

var agentEnvCatalog = []AgentEnvSpec{
	{Key: AgentEnvRuntimeSecret, Source: AgentEnvSourceControlPlaneInjected, Sensitive: true},
	{Key: AgentEnvProxyAPIKey, Source: AgentEnvSourceControlPlaneInjected, Sensitive: true},
	{Key: AgentEnvAgentModel, Source: AgentEnvSourceControlPlaneInjected},
	{Key: AgentEnvAgentBaseURL, Source: AgentEnvSourceControlPlaneInjected},
	{Key: AgentEnvAgentAPIKeyEnv, Source: AgentEnvSourceControlPlaneInjected},
	{Key: AgentEnvAgentMultimodalModel, Source: AgentEnvSourceControlPlaneInjected},
	{Key: AgentEnvAgentMultimodalBaseURL, Source: AgentEnvSourceControlPlaneInjected},
	{Key: AgentEnvAgentMultimodalAPIKeyEnv, Source: AgentEnvSourceControlPlaneInjected},
	{Key: AgentEnvAgentID, Source: AgentEnvSourceControlPlaneInjected},
	{Key: AgentEnvCloudControlPlaneURL, Source: AgentEnvSourceControlPlaneInjected},
	{Key: AgentEnvDriveUploadBearer, Source: AgentEnvSourceControlPlaneInjected, Sensitive: true},
	{Key: AgentEnvWorkspaceRoot, Source: AgentEnvSourceControlPlaneInjected},
	{Key: AgentEnvDBPath, Source: AgentEnvSourceControlPlaneInjected},
	{Key: AgentEnvRuntimeBindAddr, Source: AgentEnvSourceControlPlaneInjected},
	{Key: AgentEnvTunnelPassword, Source: AgentEnvSourceControlPlaneInjected, Sensitive: true},
	{Key: AgentEnvSandboxID, Source: AgentEnvSourceControlPlaneInjected},
	{Key: AgentEnvOrgID, Source: AgentEnvSourceControlPlaneInjected},
	{Key: AgentEnvGitUsername, Source: AgentEnvSourceControlPlaneInjected},
	{Key: AgentEnvGitEmail, Source: AgentEnvSourceControlPlaneInjected},
	{Key: AgentEnvGitCredentialsURL, Source: AgentEnvSourceControlPlaneInjected},
	{Key: AgentEnvGitHubNoKeyring, Source: AgentEnvSourceControlPlaneInjected},
	{Key: AgentEnvDriveUploadURL, Source: AgentEnvSourceControlPlaneInjected},
	{Key: AgentEnvBugsinkURL, Source: AgentEnvSourceControlPlaneInjected},
	{Key: AgentEnvBugsinkDashboardBaseURL, Source: AgentEnvSourceControlPlaneInjected, Optional: true},
	{Key: AgentEnvBugsinkToken, Source: AgentEnvSourceControlPlaneInjected, Sensitive: true},
	{Key: AgentEnvGlitchTipURL, Source: AgentEnvSourceControlPlaneInjected},
	{Key: AgentEnvGlitchTipDashboardBaseURL, Source: AgentEnvSourceControlPlaneInjected, Optional: true},
	{Key: AgentEnvGlitchTipToken, Source: AgentEnvSourceControlPlaneInjected, Sensitive: true},
	{Key: AgentEnvLinearURL, Source: AgentEnvSourceControlPlaneInjected},
	{Key: AgentEnvLinearToken, Source: AgentEnvSourceControlPlaneInjected, Sensitive: true},
	{Key: AgentEnvNotionAPIURL, Source: AgentEnvSourceControlPlaneInjected},
	{Key: AgentEnvNotionToken, Source: AgentEnvSourceControlPlaneInjected, Sensitive: true},
	{Key: AgentEnvRailwayAPIURL, Source: AgentEnvSourceControlPlaneInjected},
	{Key: AgentEnvRailwayAPIKey, Source: AgentEnvSourceControlPlaneInjected, Sensitive: true},
	{Key: AgentEnvVercelAPIURL, Source: AgentEnvSourceControlPlaneInjected},
	{Key: AgentEnvVercelAPIKey, Source: AgentEnvSourceControlPlaneInjected, Sensitive: true},
	{Key: AgentEnvSlackAPIURL, Source: AgentEnvSourceControlPlaneInjected},
	{Key: AgentEnvSlackToken, Source: AgentEnvSourceControlPlaneInjected, Sensitive: true},
	{Key: AgentEnvPostgresURL, Source: AgentEnvSourceControlPlaneInjected},
	{Key: AgentEnvPostgresToken, Source: AgentEnvSourceControlPlaneInjected, Sensitive: true},
	{Key: AgentEnvMySQLURL, Source: AgentEnvSourceControlPlaneInjected},
	{Key: AgentEnvMySQLToken, Source: AgentEnvSourceControlPlaneInjected, Sensitive: true},
	{Key: AgentEnvMongoDBURL, Source: AgentEnvSourceControlPlaneInjected},
	{Key: AgentEnvMongoDBToken, Source: AgentEnvSourceControlPlaneInjected, Sensitive: true},
	{Key: AgentEnvSentryDSN, Source: AgentEnvSourceConditionalSentry, Optional: true},
	{Key: AgentEnvSentryEnvironment, Source: AgentEnvSourceConditionalSentry, Optional: true},
	{Key: AgentEnvSentrySampleRate, Source: AgentEnvSourceConditionalSentry, Optional: true},
	{Key: AgentEnvSentryTracesSampleRate, Source: AgentEnvSourceConditionalSentry, Optional: true},
	{Key: AgentEnvSentryEnableLogs, Source: AgentEnvSourceConditionalSentry, Optional: true},
	{Key: AgentEnvSentryRelease, Source: AgentEnvSourceConditionalSentry, Optional: true},
	{Key: AgentEnvHome, Source: AgentEnvSourceContainerOS, Optional: true},
	{Key: AgentEnvPath, Source: AgentEnvSourceContainerOS, Optional: true},
	{Key: AgentEnvLang, Source: AgentEnvSourceContainerOS, Optional: true},
	{Key: AgentEnvLCAll, Source: AgentEnvSourceContainerOS, Optional: true},
}

func AgentEnvCatalog() []AgentEnvSpec {
	out := make([]AgentEnvSpec, len(agentEnvCatalog))
	copy(out, agentEnvCatalog)
	return out
}

func AgentForbiddenRawProviderEnvKeys() []string {
	out := make([]string, 0, 4)
	for _, spec := range agentEnvCatalog {
		if spec.Forbidden {
			out = append(out, spec.Key)
		}
	}
	sort.Strings(out)
	return out
}

func AgentEnvReportFromKeys(keys []string, includeUnexpected bool) []AgentEnvReportEntry {
	env := make(map[string]string, len(keys))
	for _, key := range keys {
		if key != "" {
			env[key] = ""
		}
	}
	return AgentEnvReportFromEnv(env, includeUnexpected, false)
}

func AgentEnvReportFromEnv(env map[string]string, includeUnexpected bool, includeSensitive bool) []AgentEnvReportEntry {
	known := make(map[string]AgentEnvSpec, len(agentEnvCatalog))
	out := make([]AgentEnvReportEntry, 0, len(agentEnvCatalog))
	for _, spec := range agentEnvCatalog {
		known[spec.Key] = spec
		value, present := env[spec.Key]
		status := AgentEnvStatusOK
		switch {
		case spec.Forbidden && present:
			status = AgentEnvStatusForbidden
		case spec.Forbidden:
			status = AgentEnvStatusOK
		case !present && spec.Optional:
			status = AgentEnvStatusMissingOptional
		case !present:
			status = AgentEnvStatusMissing
		}
		value, redacted := reportValue(value, present, spec.Sensitive, includeSensitive)
		out = append(out, AgentEnvReportEntry{
			Key:       spec.Key,
			Source:    spec.Source,
			Set:       present,
			Sensitive: spec.Sensitive,
			Forbidden: spec.Forbidden,
			Status:    status,
			Value:     value,
			Redacted:  redacted,
		})
	}
	if includeUnexpected {
		for key, value := range env {
			if _, ok := known[key]; ok {
				continue
			}
			sensitive := looksSensitiveEnvKey(key)
			value, redacted := reportValue(value, true, sensitive, includeSensitive)
			out = append(out, AgentEnvReportEntry{
				Key:       key,
				Source:    AgentEnvStatusUnexpected,
				Set:       true,
				Sensitive: sensitive,
				Status:    AgentEnvStatusUnexpected,
				Value:     value,
				Redacted:  redacted,
			})
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Status == out[j].Status {
				return out[i].Key < out[j].Key
			}
			return out[i].Status < out[j].Status
		})
	}
	return out
}

func reportValue(value string, present bool, sensitive bool, includeSensitive bool) (string, bool) {
	if !present {
		return "", false
	}
	if sensitive && !includeSensitive {
		return AgentEnvValueRedacted, true
	}
	return value, false
}

// IsSensitiveEnvKey reports whether an env var should be treated as a secret (for
// log redaction), by catalog Sensitive flag or a credential-looking key name.
func IsSensitiveEnvKey(key string) bool {
	for _, spec := range agentEnvCatalog {
		if spec.Key == key {
			return spec.Sensitive
		}
	}
	return looksSensitiveEnvKey(key)
}

func looksSensitiveEnvKey(key string) bool {
	key = strings.ToUpper(key)
	for _, marker := range []string{"TOKEN", "SECRET", "PASSWORD", "CREDENTIAL", "AUTH", "BEARER"} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}
