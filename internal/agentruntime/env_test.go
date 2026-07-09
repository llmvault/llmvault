package agentruntime

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
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
		AgentEnvAgentID,
		AgentEnvCloudControlPlaneURL,
		AgentEnvRuntimeEventWSURL,
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
		AgentEnvApifyURL,
		AgentEnvApifyToken,
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
		AgentEnvRedisURL,
		AgentEnvRedisToken,
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
	ApplyServiceProxyEnv(env, "https://api.example.test/", agentID, "runtime-secret", nil)

	want := map[string]string{
		AgentEnvBugsinkURL:     "https://api.example.test/internal/bugsink-proxy/11111111-1111-1111-1111-111111111111",
		AgentEnvBugsinkToken:   "runtime-secret",
		AgentEnvGlitchTipURL:   "https://api.example.test/internal/glitchtip-proxy/11111111-1111-1111-1111-111111111111",
		AgentEnvGlitchTipToken: "runtime-secret",
		AgentEnvApifyURL:       "https://api.example.test/internal/apify-proxy/11111111-1111-1111-1111-111111111111",
		AgentEnvApifyToken:     "runtime-secret",
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
		AgentEnvRedisURL:       "https://api.example.test/internal/database-proxy/redis/11111111-1111-1111-1111-111111111111",
		AgentEnvRedisToken:     "runtime-secret",
	}
	if !reflect.DeepEqual(env, want) {
		t.Fatalf("proxy env = %#v, want %#v", env, want)
	}
}

func TestApplyServiceProxyEnvGatesProvidersByTeamPlugins(t *testing.T) {
	db := connectCompileTestDB(t)
	org := model.Org{ID: uuid.New(), Name: "proxygate-" + uuid.NewString()[:8], RateLimit: 1000, Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	railwayPlugin := model.Plugin{ID: uuid.New(), Slug: "railway-" + uuid.NewString()[:8], Name: "railway", Status: model.PluginStatusActive, Manifest: model.RawJSON(`{}`)}
	if err := db.Create(&railwayPlugin).Error; err != nil {
		t.Fatalf("create plugin: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", railwayPlugin.ID).Delete(&model.Plugin{}) })
	integ := model.PluginIntegration{PluginID: railwayPlugin.ID, Provider: "railway", Kind: model.PluginIntegrationKindIntegration, Required: true}
	if err := db.Create(&integ).Error; err != nil {
		t.Fatalf("create plugin integration: %v", err)
	}
	t.Cleanup(func() { db.Where("plugin_id = ?", railwayPlugin.ID).Delete(&model.PluginIntegration{}) })
	install := model.OrgPluginInstall{ID: uuid.New(), OrgID: org.ID, PluginID: railwayPlugin.ID}
	if err := db.Create(&install).Error; err != nil {
		t.Fatalf("install org plugin: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", install.ID).Delete(&model.OrgPluginInstall{}) })

	teamWith := seedCompileTeam(t, db, org.ID)
	teamWithout := seedCompileTeam(t, db, org.ID)
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		db.Where("id IN ?", []uuid.UUID{teamWith.ID, teamWithout.ID}).Delete(&model.Team{})
	})
	grant := model.TeamPlugin{OrgID: org.ID, TeamID: teamWith.ID, PluginID: railwayPlugin.ID}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatalf("grant team plugin: %v", err)
	}
	t.Cleanup(func() {
		db.Where("team_id = ? AND plugin_id = ?", teamWith.ID, railwayPlugin.ID).Delete(&model.TeamPlugin{})
	})

	seedAgent := func(teamID uuid.UUID) model.Agent {
		agent := model.Agent{
			ID:            uuid.New(),
			OrgID:         &org.ID,
			TeamID:        teamID,
			Name:          "proxygate-agent-" + uuid.NewString()[:8],
			Model:         DefaultAgentModel,
			Status:        "active",
			Tools:         model.JSON{},
			McpServers:    model.RawJSON("[]"),
			Skills:        model.JSON{},
			Integrations:  model.JSON{},
			Resources:     model.JSON{},
			RuntimeConfig: model.JSON{},
			Permissions:   model.JSON{},
		}
		if err := db.Create(&agent).Error; err != nil {
			t.Fatalf("create agent: %v", err)
		}
		return agent
	}

	agentWith := seedAgent(teamWith.ID)
	agentWithout := seedAgent(teamWithout.ID)

	applyFor := func(agent model.Agent) map[string]string {
		allowed, err := AllowedServiceProxyProviders(context.Background(), db, agent)
		if err != nil {
			t.Fatalf("allowed providers: %v", err)
		}
		env := map[string]string{}
		ApplyServiceProxyEnv(env, "https://api.example.test/", agent.ID, "runtime-secret", allowed)
		return env
	}

	withEnv := applyFor(agentWith)
	if withEnv[AgentEnvRailwayAPIURL] == "" || withEnv[AgentEnvRailwayAPIKey] != "runtime-secret" {
		t.Fatalf("agent WITH railway plugin should get railway proxy env: %#v", withEnv)
	}

	withoutEnv := applyFor(agentWithout)
	if _, ok := withoutEnv[AgentEnvRailwayAPIURL]; ok {
		t.Fatalf("agent WITHOUT railway plugin must not get %s", AgentEnvRailwayAPIURL)
	}
	if _, ok := withoutEnv[AgentEnvRailwayAPIKey]; ok {
		t.Fatalf("agent WITHOUT railway plugin must not get %s", AgentEnvRailwayAPIKey)
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
