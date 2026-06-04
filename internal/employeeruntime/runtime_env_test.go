package employeeruntime

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

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
			APIWebhookBaseURL: "https://api.example.test",
			ProxyHost:         "https://proxy.example.test",
		},
	}
	token := &ProxyTokenResult{Token: "ptok_test", JTI: "jti_test"}

	env, err := BuildRuntimeEnvWithProxyToken(context.Background(), deps, agent, sandbox, "runtime-secret", token)
	if err != nil {
		t.Fatalf("build env: %v", err)
	}

	want := map[string]string{
		EmployeeEnvDriveUploadURL: "https://api.example.test/internal/employees/" + employeeID.String() + "/drive",
		EmployeeEnvBugsinkURL:     "https://api.example.test/internal/bugsink-proxy/" + employeeID.String(),
		EmployeeEnvBugsinkToken:   "runtime-secret",
		EmployeeEnvLinearURL:      "https://api.example.test/internal/linear-proxy/" + employeeID.String(),
		EmployeeEnvLinearToken:    "runtime-secret",
		EmployeeEnvNotionAPIURL:   "https://api.example.test/internal/notion-proxy/" + employeeID.String(),
		EmployeeEnvNotionToken:    "runtime-secret",
		EmployeeEnvRailwayAPIURL:  "https://api.example.test/internal/railway-proxy/" + employeeID.String(),
		EmployeeEnvRailwayAPIKey:  "runtime-secret",
		EmployeeEnvVercelAPIURL:   "https://api.example.test/internal/vercel-proxy/" + employeeID.String(),
		EmployeeEnvVercelAPIKey:   "runtime-secret",
		EmployeeEnvSlackAPIURL:    "https://api.example.test/internal/slack-proxy/" + employeeID.String(),
		EmployeeEnvSlackToken:     "runtime-secret",
		EmployeeEnvPostgresURL:    "https://api.example.test/internal/database-proxy/postgres/" + employeeID.String(),
		EmployeeEnvPostgresToken:  "runtime-secret",
		EmployeeEnvMySQLURL:       "https://api.example.test/internal/database-proxy/mysql/" + employeeID.String(),
		EmployeeEnvMySQLToken:     "runtime-secret",
		EmployeeEnvMongoDBURL:     "https://api.example.test/internal/database-proxy/mongodb/" + employeeID.String(),
		EmployeeEnvMongoDBToken:   "runtime-secret",
	}
	for key, value := range want {
		if env[key] != value {
			t.Fatalf("%s = %q, want %q", key, env[key], value)
		}
	}
}
