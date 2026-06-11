package employeeruntime

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
	employeeID := uuid.New()
	agent := &model.Employee{
		ID:     employeeID,
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
		EmployeeEnvDriveUploadURL:         "https://api.example.test/internal/employees/" + employeeID.String() + "/drive",
		EmployeeEnvGitUsername:            "hivy",
		EmployeeEnvGitEmail:               "hivy@users.noreply.github.com",
		EmployeeEnvGitCredentialsURL:      "https://api.example.test/internal/git-credentials/" + employeeID.String(),
		EmployeeEnvGitHubNoKeyring:        "1",
		EmployeeEnvBugsinkURL:             "https://api.example.test/internal/bugsink-proxy/" + employeeID.String(),
		EmployeeEnvBugsinkToken:           "runtime-secret",
		EmployeeEnvGlitchTipURL:           "https://api.example.test/internal/glitchtip-proxy/" + employeeID.String(),
		EmployeeEnvGlitchTipToken:         "runtime-secret",
		EmployeeEnvLinearURL:              "https://api.example.test/internal/linear-proxy/" + employeeID.String(),
		EmployeeEnvLinearToken:            "runtime-secret",
		EmployeeEnvNotionAPIURL:           "https://api.example.test/internal/notion-proxy/" + employeeID.String(),
		EmployeeEnvNotionToken:            "runtime-secret",
		EmployeeEnvRailwayAPIURL:          "https://api.example.test/internal/railway-proxy/" + employeeID.String(),
		EmployeeEnvRailwayAPIKey:          "runtime-secret",
		EmployeeEnvVercelAPIURL:           "https://api.example.test/internal/vercel-proxy/" + employeeID.String(),
		EmployeeEnvVercelAPIKey:           "runtime-secret",
		EmployeeEnvSlackAPIURL:            "https://api.example.test/internal/slack-proxy/" + employeeID.String(),
		EmployeeEnvSlackToken:             "runtime-secret",
		EmployeeEnvPostgresURL:            "https://api.example.test/internal/database-proxy/postgres/" + employeeID.String(),
		EmployeeEnvPostgresToken:          "runtime-secret",
		EmployeeEnvMySQLURL:               "https://api.example.test/internal/database-proxy/mysql/" + employeeID.String(),
		EmployeeEnvMySQLToken:             "runtime-secret",
		EmployeeEnvMongoDBURL:             "https://api.example.test/internal/database-proxy/mongodb/" + employeeID.String(),
		EmployeeEnvMongoDBToken:           "runtime-secret",
		EmployeeEnvSentryDSN:              "https://agent@example.test/1",
		EmployeeEnvSentryEnvironment:      "production",
		EmployeeEnvSentrySampleRate:       "1",
		EmployeeEnvSentryTracesSampleRate: "0.25",
		EmployeeEnvSentryEnableLogs:       "true",
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
	agent := &model.Employee{
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

	if env[EmployeeEnvTunnelPassword] != "runtime-secret" {
		t.Fatalf("%s = %q, want %q (tunnel must fail closed)", EmployeeEnvTunnelPassword, env[EmployeeEnvTunnelPassword], "runtime-secret")
	}
}

// Reserved HIVY_ runtime keys (proxy key, runtime secret, drive bearer) must not
// be clobbered by org-supplied env vars that share the same name.
func TestBuildRuntimeEnvWithProxyToken_ReservedKeysNotClobbedByUserEnv(t *testing.T) {
	encKey := testSymmetricKey(t)

	orgID := uuid.New()
	agent := &model.Employee{
		ID:    uuid.New(),
		OrgID: &orgID,
		// Org env vars smuggling reserved HIVY_ keys.
		EncryptedEnvVars: testEncryptJSON(t, encKey, map[string]string{
			EmployeeEnvRuntimeSecret:     "user-controlled-secret",
			EmployeeEnvProxyAPIKey:       "user-controlled-proxy-key",
			EmployeeEnvDriveUploadBearer: "user-controlled-bearer",
			"MY_CUSTOM_VAR":              "custom-value",
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
	if got := env[EmployeeEnvRuntimeSecret]; got != runtimeSecret {
		t.Errorf("%s = %q, want control-plane value %q; user env must not clobber reserved key",
			EmployeeEnvRuntimeSecret, got, runtimeSecret)
	}
	if got := env[EmployeeEnvProxyAPIKey]; got != proxyToken.Token {
		t.Errorf("%s = %q, want control-plane value %q; user env must not clobber reserved key",
			EmployeeEnvProxyAPIKey, got, proxyToken.Token)
	}
	if got := env[EmployeeEnvDriveUploadBearer]; got != runtimeSecret {
		t.Errorf("%s = %q, want control-plane value %q; user env must not clobber reserved key",
			EmployeeEnvDriveUploadBearer, got, runtimeSecret)
	}

	if got := env["MY_CUSTOM_VAR"]; got != "custom-value" {
		t.Errorf("MY_CUSTOM_VAR = %q, want custom-value; non-reserved user env must be preserved", got)
	}
}
