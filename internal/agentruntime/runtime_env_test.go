package agentruntime

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/model"
)

func testSymmetricKey(t *testing.T) *crypto.SymmetricKey {
	t.Helper()
	key, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatalf("create enc key: %v", err)
	}
	return key
}

func TestBuildRuntimeEnvWithProxyTokenIncludesControlPlaneEnv(t *testing.T) {
	orgID := uuid.New()
	agentID := uuid.New()
	agent := &model.Agent{
		ID:     agentID,
		OrgID:  &orgID,
		Name:   "Hivy",
		Status: "active",
	}
	sandbox := &model.Sandbox{ID: uuid.New()}
	deps := CompileDeps{
		Cfg: &config.Config{
			APIWebhookBaseURL:      "https://api.example.test",
			ProxyHost:              "https://proxy.example.test",
			Environment:            "production",
			SentryTracesSampleRate: 0.25,
			AgentSandboxSentryDSN:  "https://agent@example.test/1",
		},
	}
	token := &ProxyTokenResult{Token: "ptok_test", JTI: "jti_test"}

	env, err := BuildRuntimeEnvWithProxyToken(context.Background(), deps, agent, sandbox, "runtime-secret", token, uuid.Nil)
	if err != nil {
		t.Fatalf("build env: %v", err)
	}

	want := map[string]string{
		AgentEnvDriveUploadURL:         "https://api.example.test/internal/agents/" + agentID.String() + "/sandboxes/" + sandbox.ID.String() + "/drive",
		AgentEnvGitUsername:            "hivy",
		AgentEnvGitEmail:               "hivy@users.noreply.github.com",
		AgentEnvGitCredentialsURL:      "https://api.example.test/internal/git-credentials/" + agentID.String(),
		AgentEnvGitHubNoKeyring:        "1",
		AgentEnvPostgresURL:            "https://api.example.test/internal/database-proxy/postgres/" + agentID.String(),
		AgentEnvPostgresToken:          "runtime-secret",
		AgentEnvMySQLURL:               "https://api.example.test/internal/database-proxy/mysql/" + agentID.String(),
		AgentEnvMySQLToken:             "runtime-secret",
		AgentEnvMongoDBURL:             "https://api.example.test/internal/database-proxy/mongodb/" + agentID.String(),
		AgentEnvMongoDBToken:           "runtime-secret",
		AgentEnvRedisURL:               "https://api.example.test/internal/database-proxy/redis/" + agentID.String(),
		AgentEnvRedisToken:             "runtime-secret",
		AgentEnvSentryDSN:              "https://agent@example.test/1",
		AgentEnvSentryEnvironment:      "production",
		AgentEnvSentrySampleRate:       "1",
		AgentEnvSentryTracesSampleRate: "0.25",
		AgentEnvSentryEnableLogs:       "true",
		AgentEnvWorkspaceRoot:          "/workspace",
		AgentEnvDBPath:                 AgentRuntimeDBPath,
	}
	for key, value := range want {
		if env[key] != value {
			t.Fatalf("%s = %q, want %q", key, env[key], value)
		}
	}
}

// The runtime tunnel proxy is an unauthenticated open proxy when
// HIVY_TUNNEL_PASSWORD is unset, so the control plane must always provision it.
func TestBuildRuntimeEnvProvisionsTunnelPassword(t *testing.T) {
	orgID := uuid.New()
	agent := &model.Agent{
		ID:     uuid.New(),
		OrgID:  &orgID,
		Name:   "Hivy",
		Status: "active",
	}
	sandbox := &model.Sandbox{ID: uuid.New()}
	deps := CompileDeps{
		Cfg: &config.Config{
			APIWebhookBaseURL: "https://api.example.test",
			ProxyHost:         "https://proxy.example.test",
			Environment:       "production",
		},
	}
	token := &ProxyTokenResult{Token: "ptok_test", JTI: "jti_test"}

	env, err := BuildRuntimeEnvWithProxyToken(context.Background(), deps, agent, sandbox, "runtime-secret", token, uuid.Nil)
	if err != nil {
		t.Fatalf("build env: %v", err)
	}

	if env[AgentEnvTunnelPassword] != "runtime-secret" {
		t.Fatalf("%s = %q, want %q (tunnel must fail closed)", AgentEnvTunnelPassword, env[AgentEnvTunnelPassword], "runtime-secret")
	}
}

// Team env vars are injected as __ENV__<NAME>, both when the team is passed
// explicitly (session-create path) and when it must be resolved from the
// sandbox session's channel (lifecycle / token-refresh paths).
func TestBuildRuntimeEnvWithProxyToken_InjectsTeamEnvVars(t *testing.T) {
	db := connectCompileTestDB(t)
	encKey := testSymmetricKey(t)

	orgID := uuid.New()
	if err := db.Create(&model.Org{ID: orgID, Name: "env-org-" + uuid.NewString()[:8], Active: true}).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	team := seedCompileTeam(t, db, orgID)
	agent := &model.Agent{
		ID: uuid.New(), OrgID: &orgID, TeamID: team.ID, Name: "Hivy", Model: "deepseek-v4-flash",
		Tools: model.JSON{}, McpServers: model.RawJSON("[]"), Skills: model.JSON{},
		RuntimeConfig: model.JSON{}, Permissions: model.JSON{}, Resources: model.JSON{}, Status: "active",
	}
	if err := db.Create(agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	channel := &model.Channel{ID: uuid.New(), OrgID: orgID, TeamID: team.ID, Name: "engineering", DefaultAgentID: agent.ID}
	if err := db.Create(channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	sbID := uuid.New()
	if err := db.Create(&model.Sandbox{
		ID: sbID, OrgID: &orgID, AgentID: &agent.ID,
		ExternalID: "ext-" + sbID.String(), RuntimeURL: "https://runtime.test",
		EncryptedRuntimeSecret: []byte("x"), Status: "running",
	}).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	if err := db.Create(&model.Session{
		OrgID: orgID, ChannelID: channel.ID, AgentID: agent.ID, SandboxID: &sbID, Status: "active",
	}).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	encValue, err := encKey.EncryptString("postgres://secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if err := db.Create(&model.TeamEnvVar{
		OrgID: orgID, TeamID: team.ID, Name: "DATABASE_URL", EncryptedValue: encValue,
	}).Error; err != nil {
		t.Fatalf("create team env var: %v", err)
	}
	t.Cleanup(func() {
		db.Where("team_id = ?", team.ID).Delete(&model.TeamEnvVar{})
		db.Where("org_id = ?", orgID).Delete(&model.Session{})
		db.Where("id = ?", sbID).Delete(&model.Sandbox{})
		db.Where("org_id = ?", orgID).Delete(&model.Channel{})
		db.Where("org_id = ?", orgID).Delete(&model.Agent{})
		db.Where("id = ?", orgID).Delete(&model.Org{})
	})

	deps := CompileDeps{DB: db, EncKey: encKey, Cfg: &config.Config{APIWebhookBaseURL: "https://api.example.test"}}
	sb := &model.Sandbox{ID: sbID, OrgID: &orgID, AgentID: &agent.ID}
	token := &ProxyTokenResult{Token: "ptok_test", JTI: "jti_test"}

	// Explicit team (session-create path).
	env, err := BuildRuntimeEnvWithProxyToken(context.Background(), deps, agent, sb, "runtime-secret", token, team.ID)
	if err != nil {
		t.Fatalf("build env (explicit team): %v", err)
	}
	if got := env["__ENV__DATABASE_URL"]; got != "postgres://secret" {
		t.Fatalf("__ENV__DATABASE_URL = %q, want postgres://secret", got)
	}
	if env[AgentEnvRuntimeSecret] != "runtime-secret" {
		t.Fatalf("reserved key clobbered: %s = %q", AgentEnvRuntimeSecret, env[AgentEnvRuntimeSecret])
	}

	// Nil team resolves from the sandbox session's channel.
	env, err = BuildRuntimeEnvWithProxyToken(context.Background(), deps, agent, sb, "runtime-secret", token, uuid.Nil)
	if err != nil {
		t.Fatalf("build env (resolved team): %v", err)
	}
	if got := env["__ENV__DATABASE_URL"]; got != "postgres://secret" {
		t.Fatalf("resolved __ENV__DATABASE_URL = %q, want postgres://secret", got)
	}
}

// A nil team injects no user env, and the reserved HIVY_ control-plane keys
// are always populated with the authoritative values.
func TestBuildRuntimeEnvWithProxyToken_ReservedKeysPopulated(t *testing.T) {
	encKey := testSymmetricKey(t)

	orgID := uuid.New()
	agent := &model.Agent{
		ID:    uuid.New(),
		OrgID: &orgID,
	}

	sb := &model.Sandbox{ID: uuid.New()}
	runtimeSecret := "real-runtime-secret"
	proxyToken := &ProxyTokenResult{
		Token: "ptok_real-proxy-token",
		JTI:   uuid.New().String(),
	}

	deps := CompileDeps{
		EncKey: encKey,
		Cfg: &config.Config{
			Environment:       "test",
			APIWebhookBaseURL: "https://api.example.test",
		},
	}

	// Nil team and nil DB: mergeTeamEnvVars is a no-op.
	env, err := BuildRuntimeEnvWithProxyToken(context.Background(), deps, agent, sb, runtimeSecret, proxyToken, uuid.Nil)
	if err != nil {
		t.Fatalf("BuildRuntimeEnvWithProxyToken: %v", err)
	}

	if got := env[AgentEnvRuntimeSecret]; got != runtimeSecret {
		t.Errorf("%s = %q, want control-plane value %q", AgentEnvRuntimeSecret, got, runtimeSecret)
	}
	if got := env[AgentEnvProxyAPIKey]; got != proxyToken.Token {
		t.Errorf("%s = %q, want control-plane value %q", AgentEnvProxyAPIKey, got, proxyToken.Token)
	}
	if got := env[AgentEnvDriveUploadBearer]; got != runtimeSecret {
		t.Errorf("%s = %q, want control-plane value %q", AgentEnvDriveUploadBearer, got, runtimeSecret)
	}
}
