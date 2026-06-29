package agentruntime

import (
	"context"
	"encoding/base64"
	"encoding/json"
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

func testEncryptJSON(t *testing.T, key *crypto.SymmetricKey, obj map[string]string) []byte {
	t.Helper()
	data, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("marshal env vars: %v", err)
	}
	enc, err := key.EncryptString(string(data))
	if err != nil {
		t.Fatalf("encrypt env vars: %v", err)
	}
	return enc
}

func TestBuildRuntimeEnvWithProxyTokenIncludesSkillProxyEnv(t *testing.T) {
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

	env, err := BuildRuntimeEnvWithProxyToken(context.Background(), deps, agent, sandbox, "runtime-secret", token)
	if err != nil {
		t.Fatalf("build env: %v", err)
	}

	want := map[string]string{
		AgentEnvDriveUploadURL:         "https://api.example.test/internal/agents/" + agentID.String() + "/sandboxes/" + sandbox.ID.String() + "/drive",
		AgentEnvGitUsername:            "hivy",
		AgentEnvGitEmail:               "hivy@users.noreply.github.com",
		AgentEnvGitCredentialsURL:      "https://api.example.test/internal/git-credentials/" + agentID.String(),
		AgentEnvGitHubNoKeyring:        "1",
		AgentEnvBugsinkURL:             "https://api.example.test/internal/bugsink-proxy/" + agentID.String(),
		AgentEnvBugsinkToken:           "runtime-secret",
		AgentEnvGlitchTipURL:           "https://api.example.test/internal/glitchtip-proxy/" + agentID.String(),
		AgentEnvGlitchTipToken:         "runtime-secret",
		AgentEnvLinearURL:              "https://api.example.test/internal/linear-proxy/" + agentID.String(),
		AgentEnvLinearToken:            "runtime-secret",
		AgentEnvNotionAPIURL:           "https://api.example.test/internal/notion-proxy/" + agentID.String(),
		AgentEnvNotionToken:            "runtime-secret",
		AgentEnvRailwayAPIURL:          "https://api.example.test/internal/railway-proxy/" + agentID.String(),
		AgentEnvRailwayAPIKey:          "runtime-secret",
		AgentEnvVercelAPIURL:           "https://api.example.test/internal/vercel-proxy/" + agentID.String(),
		AgentEnvVercelAPIKey:           "runtime-secret",
		AgentEnvSlackAPIURL:            "https://api.example.test/internal/slack-proxy/" + agentID.String(),
		AgentEnvSlackToken:             "runtime-secret",
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

	env, err := BuildRuntimeEnvWithProxyToken(context.Background(), deps, agent, sandbox, "runtime-secret", token)
	if err != nil {
		t.Fatalf("build env: %v", err)
	}

	if env[AgentEnvTunnelPassword] != "runtime-secret" {
		t.Fatalf("%s = %q, want %q (tunnel must fail closed)", AgentEnvTunnelPassword, env[AgentEnvTunnelPassword], "runtime-secret")
	}
}

// Reserved HIVY_ runtime keys (proxy key, runtime secret, drive bearer) must not
// be clobbered by org-supplied env vars that share the same name.
func TestBuildRuntimeEnvWithProxyToken_ReservedKeysNotClobbedByUserEnv(t *testing.T) {
	encKey := testSymmetricKey(t)

	orgID := uuid.New()
	agent := &model.Agent{
		ID:    uuid.New(),
		OrgID: &orgID,
		// Org env vars smuggling reserved HIVY_ keys.
		EncryptedEnvVars: testEncryptJSON(t, encKey, map[string]string{
			AgentEnvRuntimeSecret:     "user-controlled-secret",
			AgentEnvProxyAPIKey:       "user-controlled-proxy-key",
			AgentEnvDriveUploadBearer: "user-controlled-bearer",
			"MY_CUSTOM_VAR":           "custom-value",
		}),
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

	env, err := BuildRuntimeEnvWithProxyToken(context.Background(), deps, agent, sb, runtimeSecret, proxyToken)
	if err != nil {
		t.Fatalf("BuildRuntimeEnvWithProxyToken: %v", err)
	}

	// Control-plane values must always win over any user-supplied values.
	if got := env[AgentEnvRuntimeSecret]; got != runtimeSecret {
		t.Errorf("%s = %q, want control-plane value %q; user env must not clobber reserved key",
			AgentEnvRuntimeSecret, got, runtimeSecret)
	}
	if got := env[AgentEnvProxyAPIKey]; got != proxyToken.Token {
		t.Errorf("%s = %q, want control-plane value %q; user env must not clobber reserved key",
			AgentEnvProxyAPIKey, got, proxyToken.Token)
	}
	if got := env[AgentEnvDriveUploadBearer]; got != runtimeSecret {
		t.Errorf("%s = %q, want control-plane value %q; user env must not clobber reserved key",
			AgentEnvDriveUploadBearer, got, runtimeSecret)
	}

	if got := env["MY_CUSTOM_VAR"]; got != "custom-value" {
		t.Errorf("MY_CUSTOM_VAR = %q, want custom-value; non-reserved user env must be preserved", got)
	}
}
