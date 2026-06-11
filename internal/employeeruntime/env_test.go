package employeeruntime

import (
	"reflect"
	"testing"

	"github.com/google/uuid"
)

func TestEmployeeEnvCatalogGolden(t *testing.T) {
	got := EmployeeEnvCatalog()
	var keys []string
	for _, spec := range got {
		keys = append(keys, spec.Key)
	}
	want := []string{
		EmployeeEnvRuntimeSecret,
		EmployeeEnvProxyAPIKey,
		EmployeeEnvAgentModel,
		EmployeeEnvAgentBaseURL,
		EmployeeEnvAgentAPIKeyEnv,
		EmployeeEnvAgentMultimodalModel,
		EmployeeEnvAgentMultimodalBaseURL,
		EmployeeEnvAgentMultimodalAPIKeyEnv,
		EmployeeEnvEmployeeID,
		EmployeeEnvCloudControlPlaneURL,
		EmployeeEnvDriveUploadBearer,
		EmployeeEnvWorkspaceRoot,
		EmployeeEnvDBPath,
		EmployeeEnvRuntimeBindAddr,
		EmployeeEnvRuntimeMode,
		EmployeeEnvTunnelPassword,
		EmployeeEnvSandboxID,
		EmployeeEnvOrgID,
		EmployeeEnvGitUsername,
		EmployeeEnvGitEmail,
		EmployeeEnvGitCredentialsURL,
		EmployeeEnvGitHubNoKeyring,
		EmployeeEnvDriveUploadURL,
		EmployeeEnvBugsinkURL,
		EmployeeEnvBugsinkDashboardBaseURL,
		EmployeeEnvBugsinkToken,
		EmployeeEnvGlitchTipURL,
		EmployeeEnvGlitchTipDashboardBaseURL,
		EmployeeEnvGlitchTipToken,
		EmployeeEnvLinearURL,
		EmployeeEnvLinearToken,
		EmployeeEnvNotionAPIURL,
		EmployeeEnvNotionToken,
		EmployeeEnvRailwayAPIURL,
		EmployeeEnvRailwayAPIKey,
		EmployeeEnvVercelAPIURL,
		EmployeeEnvVercelAPIKey,
		EmployeeEnvSlackAPIURL,
		EmployeeEnvSlackToken,
		EmployeeEnvPostgresURL,
		EmployeeEnvPostgresToken,
		EmployeeEnvMySQLURL,
		EmployeeEnvMySQLToken,
		EmployeeEnvMongoDBURL,
		EmployeeEnvMongoDBToken,
		EmployeeEnvSentryDSN,
		EmployeeEnvSentryEnvironment,
		EmployeeEnvSentrySampleRate,
		EmployeeEnvSentryTracesSampleRate,
		EmployeeEnvSentryEnableLogs,
		EmployeeEnvSentryRelease,
		EmployeeEnvHome,
		EmployeeEnvPath,
		EmployeeEnvLang,
		EmployeeEnvLCAll,
	}
	if !reflect.DeepEqual(gotKeys(got), want) {
		t.Fatalf("employee env catalog keys changed\ngot:  %#v\nwant: %#v", keys, want)
	}
}

func TestApplyServiceProxyEnvSetsAllProviderProxyVariables(t *testing.T) {
	employeeID := mustParseUUID(t, "11111111-1111-1111-1111-111111111111")
	env := map[string]string{}
	ApplyServiceProxyEnv(env, "https://api.example.test/", employeeID, "runtime-secret")

	want := map[string]string{
		EmployeeEnvBugsinkURL:     "https://api.example.test/internal/bugsink-proxy/11111111-1111-1111-1111-111111111111",
		EmployeeEnvBugsinkToken:   "runtime-secret",
		EmployeeEnvGlitchTipURL:   "https://api.example.test/internal/glitchtip-proxy/11111111-1111-1111-1111-111111111111",
		EmployeeEnvGlitchTipToken: "runtime-secret",
		EmployeeEnvLinearURL:      "https://api.example.test/internal/linear-proxy/11111111-1111-1111-1111-111111111111",
		EmployeeEnvLinearToken:    "runtime-secret",
		EmployeeEnvNotionAPIURL:   "https://api.example.test/internal/notion-proxy/11111111-1111-1111-1111-111111111111",
		EmployeeEnvNotionToken:    "runtime-secret",
		EmployeeEnvRailwayAPIURL:  "https://api.example.test/internal/railway-proxy/11111111-1111-1111-1111-111111111111",
		EmployeeEnvRailwayAPIKey:  "runtime-secret",
		EmployeeEnvVercelAPIURL:   "https://api.example.test/internal/vercel-proxy/11111111-1111-1111-1111-111111111111",
		EmployeeEnvVercelAPIKey:   "runtime-secret",
		EmployeeEnvSlackAPIURL:    "https://api.example.test/internal/slack-proxy/11111111-1111-1111-1111-111111111111",
		EmployeeEnvSlackToken:     "runtime-secret",
		EmployeeEnvPostgresURL:    "https://api.example.test/internal/database-proxy/postgres/11111111-1111-1111-1111-111111111111",
		EmployeeEnvPostgresToken:  "runtime-secret",
		EmployeeEnvMySQLURL:       "https://api.example.test/internal/database-proxy/mysql/11111111-1111-1111-1111-111111111111",
		EmployeeEnvMySQLToken:     "runtime-secret",
		EmployeeEnvMongoDBURL:     "https://api.example.test/internal/database-proxy/mongodb/11111111-1111-1111-1111-111111111111",
		EmployeeEnvMongoDBToken:   "runtime-secret",
	}
	if !reflect.DeepEqual(env, want) {
		t.Fatalf("proxy env = %#v, want %#v", env, want)
	}
}

func TestEmployeeEnvReport_TracksMissingForbiddenAndRedactedValues(t *testing.T) {
	report := EmployeeEnvReportFromEnv(map[string]string{
		EmployeeEnvRuntimeSecret: "runtime-secret",
		EmployeeEnvAgentBaseURL:  "https://proxy.example.test/v1",
		EmployeeEnvProxyAPIKey:   "ptok_test",
	}, false, false)
	byKey := map[string]EmployeeEnvReportEntry{}
	for _, entry := range report {
		byKey[entry.Key] = entry
	}
	if got := byKey[EmployeeEnvRuntimeSecret]; !got.Set || got.Status != EmployeeEnvStatusOK || !got.Sensitive || got.Value != EmployeeEnvValueRedacted || !got.Redacted {
		t.Fatalf("runtime secret report = %+v", got)
	}
	if got := byKey[EmployeeEnvAgentBaseURL]; !got.Set || got.Value != "https://proxy.example.test/v1" || got.Redacted {
		t.Fatalf("non-sensitive env report = %+v", got)
	}
}

func TestEmployeeEnvReport_PrintsSensitiveValuesWhenRequested(t *testing.T) {
	report := EmployeeEnvReportFromEnv(map[string]string{
		EmployeeEnvRuntimeSecret: "runtime-secret",
		"EXTRA_API_TOKEN":        "extra-token",
	}, true, true)
	byKey := map[string]EmployeeEnvReportEntry{}
	for _, entry := range report {
		byKey[entry.Key] = entry
	}
	if got := byKey[EmployeeEnvRuntimeSecret]; got.Value != "runtime-secret" || got.Redacted {
		t.Fatalf("runtime secret report = %+v", got)
	}
	if got := byKey["EXTRA_API_TOKEN"]; !got.Sensitive || got.Value != "extra-token" || got.Redacted {
		t.Fatalf("unexpected sensitive env report = %+v", got)
	}
}

func gotKeys(specs []EmployeeEnvSpec) []string {
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
