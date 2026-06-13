package agentruntime

import (
	"reflect"
	"testing"

	"github.com/google/uuid"
)

func TestAgentEnvCatalogGolden(t *testing.T) {
	got := AgentEnvCatalog()
	var keys []string
	for _, spec := range got {
		keys = append(keys, spec.Key)
	}
	want := []string{
		AgentEnvRuntimeSecret,
		AgentEnvProxyAPIKey,
		AgentEnvAgentModel,
		AgentEnvAgentBaseURL,
		AgentEnvAgentAPIKeyEnv,
		AgentEnvAgentMultimodalModel,
		AgentEnvAgentMultimodalBaseURL,
		AgentEnvAgentMultimodalAPIKeyEnv,
		AgentEnvAgentID,
		AgentEnvCloudControlPlaneURL,
		AgentEnvDriveUploadBearer,
		AgentEnvWorkspaceRoot,
		AgentEnvDBPath,
		AgentEnvRuntimeBindAddr,
		AgentEnvTunnelPassword,
		AgentEnvSandboxID,
		AgentEnvOrgID,
		AgentEnvGitUsername,
		AgentEnvGitEmail,
		AgentEnvGitCredentialsURL,
		AgentEnvGitHubNoKeyring,
		AgentEnvDriveUploadURL,
		AgentEnvBugsinkURL,
		AgentEnvBugsinkDashboardBaseURL,
		AgentEnvBugsinkToken,
		AgentEnvGlitchTipURL,
		AgentEnvGlitchTipDashboardBaseURL,
		AgentEnvGlitchTipToken,
		AgentEnvLinearURL,
		AgentEnvLinearToken,
		AgentEnvNotionAPIURL,
		AgentEnvNotionToken,
		AgentEnvRailwayAPIURL,
		AgentEnvRailwayAPIKey,
		AgentEnvVercelAPIURL,
		AgentEnvVercelAPIKey,
		AgentEnvSlackAPIURL,
		AgentEnvSlackToken,
		AgentEnvPostgresURL,
		AgentEnvPostgresToken,
		AgentEnvMySQLURL,
		AgentEnvMySQLToken,
		AgentEnvMongoDBURL,
		AgentEnvMongoDBToken,
		AgentEnvSentryDSN,
		AgentEnvSentryEnvironment,
		AgentEnvSentrySampleRate,
		AgentEnvSentryTracesSampleRate,
		AgentEnvSentryEnableLogs,
		AgentEnvSentryRelease,
		AgentEnvHome,
		AgentEnvPath,
		AgentEnvLang,
		AgentEnvLCAll,
	}
	if !reflect.DeepEqual(gotKeys(got), want) {
		t.Fatalf("agent env catalog keys changed\ngot:  %#v\nwant: %#v", keys, want)
	}
}

func TestApplyServiceProxyEnvSetsAllProviderProxyVariables(t *testing.T) {
	agentID := mustParseUUID(t, "11111111-1111-1111-1111-111111111111")
	env := map[string]string{}
	ApplyServiceProxyEnv(env, "https://api.example.test/", agentID, "runtime-secret")

	want := map[string]string{
		AgentEnvBugsinkURL:     "https://api.example.test/internal/bugsink-proxy/11111111-1111-1111-1111-111111111111",
		AgentEnvBugsinkToken:   "runtime-secret",
		AgentEnvGlitchTipURL:   "https://api.example.test/internal/glitchtip-proxy/11111111-1111-1111-1111-111111111111",
		AgentEnvGlitchTipToken: "runtime-secret",
		AgentEnvLinearURL:      "https://api.example.test/internal/linear-proxy/11111111-1111-1111-1111-111111111111",
		AgentEnvLinearToken:    "runtime-secret",
		AgentEnvNotionAPIURL:   "https://api.example.test/internal/notion-proxy/11111111-1111-1111-1111-111111111111",
		AgentEnvNotionToken:    "runtime-secret",
		AgentEnvRailwayAPIURL:  "https://api.example.test/internal/railway-proxy/11111111-1111-1111-1111-111111111111",
		AgentEnvRailwayAPIKey:  "runtime-secret",
		AgentEnvVercelAPIURL:   "https://api.example.test/internal/vercel-proxy/11111111-1111-1111-1111-111111111111",
		AgentEnvVercelAPIKey:   "runtime-secret",
		AgentEnvSlackAPIURL:    "https://api.example.test/internal/slack-proxy/11111111-1111-1111-1111-111111111111",
		AgentEnvSlackToken:     "runtime-secret",
		AgentEnvPostgresURL:    "https://api.example.test/internal/database-proxy/postgres/11111111-1111-1111-1111-111111111111",
		AgentEnvPostgresToken:  "runtime-secret",
		AgentEnvMySQLURL:       "https://api.example.test/internal/database-proxy/mysql/11111111-1111-1111-1111-111111111111",
		AgentEnvMySQLToken:     "runtime-secret",
		AgentEnvMongoDBURL:     "https://api.example.test/internal/database-proxy/mongodb/11111111-1111-1111-1111-111111111111",
		AgentEnvMongoDBToken:   "runtime-secret",
	}
	if !reflect.DeepEqual(env, want) {
		t.Fatalf("proxy env = %#v, want %#v", env, want)
	}
}

func TestAgentEnvReport_TracksMissingForbiddenAndRedactedValues(t *testing.T) {
	report := AgentEnvReportFromEnv(map[string]string{
		AgentEnvRuntimeSecret: "runtime-secret",
		AgentEnvAgentBaseURL:  "https://proxy.example.test/v1",
		AgentEnvProxyAPIKey:   "ptok_test",
	}, false, false)
	byKey := map[string]AgentEnvReportEntry{}
	for _, entry := range report {
		byKey[entry.Key] = entry
	}
	if got := byKey[AgentEnvRuntimeSecret]; !got.Set || got.Status != AgentEnvStatusOK || !got.Sensitive || got.Value != AgentEnvValueRedacted || !got.Redacted {
		t.Fatalf("runtime secret report = %+v", got)
	}
	if got := byKey[AgentEnvAgentBaseURL]; !got.Set || got.Value != "https://proxy.example.test/v1" || got.Redacted {
		t.Fatalf("non-sensitive env report = %+v", got)
	}
}

func TestAgentEnvReport_PrintsSensitiveValuesWhenRequested(t *testing.T) {
	report := AgentEnvReportFromEnv(map[string]string{
		AgentEnvRuntimeSecret: "runtime-secret",
		"EXTRA_API_TOKEN":     "extra-token",
	}, true, true)
	byKey := map[string]AgentEnvReportEntry{}
	for _, entry := range report {
		byKey[entry.Key] = entry
	}
	if got := byKey[AgentEnvRuntimeSecret]; got.Value != "runtime-secret" || got.Redacted {
		t.Fatalf("runtime secret report = %+v", got)
	}
	if got := byKey["EXTRA_API_TOKEN"]; !got.Sensitive || got.Value != "extra-token" || got.Redacted {
		t.Fatalf("unexpected sensitive env report = %+v", got)
	}
}

func gotKeys(specs []AgentEnvSpec) []string {
	keys := make([]string, 0, len(specs))
	for _, spec := range specs {
		keys = append(keys, spec.Key)
	}
	return keys
}

func mustParseUUID(t *testing.T, value string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(value)
	if err != nil {
		t.Fatalf("parse uuid: %v", err)
	}
	return id
}
