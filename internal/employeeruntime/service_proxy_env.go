package employeeruntime

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
	{Provider: "bugsink", SkillName: "bugsink", BaseURLEnv: EmployeeEnvBugsinkURL, AuthEnv: EmployeeEnvBugsinkToken, Path: "/internal/bugsink-proxy/%s"},
	{Provider: "linear", SkillName: "linear", BaseURLEnv: EmployeeEnvLinearURL, AuthEnv: EmployeeEnvLinearToken, Path: "/internal/linear-proxy/%s"},
	{Provider: "notion", SkillName: "notion", BaseURLEnv: EmployeeEnvNotionAPIURL, AuthEnv: EmployeeEnvNotionToken, Path: "/internal/notion-proxy/%s"},
	{Provider: "railway", SkillName: "railway", BaseURLEnv: EmployeeEnvRailwayAPIURL, AuthEnv: EmployeeEnvRailwayAPIKey, Path: "/internal/railway-proxy/%s"},
	{Provider: "vercel", SkillName: "vercel", BaseURLEnv: EmployeeEnvVercelAPIURL, AuthEnv: EmployeeEnvVercelAPIKey, Path: "/internal/vercel-proxy/%s"},
	{Provider: "slack", SkillName: "slack", BaseURLEnv: EmployeeEnvSlackAPIURL, AuthEnv: EmployeeEnvSlackToken, Path: "/internal/slack-proxy/%s"},
}

func ServiceProxyEnvSpecs() []ServiceProxyEnvSpec {
	out := make([]ServiceProxyEnvSpec, len(serviceProxyEnvSpecs))
	copy(out, serviceProxyEnvSpecs)
	return out
}

func ApplyServiceProxyEnv(env map[string]string, controlPlaneBaseURL string, employeeID uuid.UUID, runtimeSecret string) {
	if env == nil || employeeID == uuid.Nil || strings.TrimSpace(controlPlaneBaseURL) == "" || runtimeSecret == "" {
		return
	}
	base := strings.TrimRight(controlPlaneBaseURL, "/")
	for _, spec := range serviceProxyEnvSpecs {
		env[spec.BaseURLEnv] = base + fmt.Sprintf(spec.Path, employeeID)
		env[spec.AuthEnv] = runtimeSecret
	}
}

func EmployeeDriveUploadURL(controlPlaneBaseURL string, employeeID uuid.UUID) string {
	return fmt.Sprintf("%s/internal/employees/%s/drive", strings.TrimRight(controlPlaneBaseURL, "/"), employeeID)
}
