package sandbox

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

const (
	RuntimePort = 25434

	runtimeHealthTimeout    = 90 * time.Second
	runtimeHealthInterval   = 2 * time.Second
	runtimeURLRefreshBuffer = 5 * time.Minute
	runtimeURLTTL           = 55 * time.Minute
)

func baseEnvVars(cfg *config.Config, runtimeSecret string, sandboxID uuid.UUID, webhookURL string) map[string]string {
	controlPlaneBaseURL := cfg.RuntimeControlPlaneBaseURL()
	envVars := map[string]string{
		agentruntime.AgentEnvRuntimeSecret:     runtimeSecret,
		agentruntime.AgentEnvDriveUploadBearer: runtimeSecret,
		agentruntime.AgentEnvRuntimeBindAddr:   fmt.Sprintf("0.0.0.0:%d", RuntimePort),
		"HIVY_WEB_URL":                         controlPlaneBaseURL + "/spider",
		agentruntime.AgentEnvSandboxID:         sandboxID.String(),
		agentruntime.AgentEnvHome:              "/work",
		"OPENCODE_CONFIG_DIR":                  "/work/.opencode",
		"NO_BROWSER":                           "1",
		"HIVY_STORAGE_PATH":                    "/work/runtime.db",
	}
	if webhookURL != "" {
		envVars["HIVY_WEBHOOK_URL"] = webhookURL
	}
	agentSentryDSN := ""
	if cfg != nil {
		agentSentryDSN = cfg.AgentSandboxSentryDSN
	}
	setSandboxSentryEnvVars(envVars, cfg, agentSentryDSN)
	return envVars
}

func setSandboxSentryEnvVars(envVars map[string]string, cfg *config.Config, dsn string) {
	agentruntime.ApplySandboxSentryEnv(envVars, cfg, dsn)
}

func setOrgEnvVars(envVars map[string]string, orgID uuid.UUID) {
	envVars[agentruntime.AgentEnvOrgID] = orgID.String()
}

func setAgentEnvVars(envVars map[string]string, agent *model.Agent, cfg *config.Config) {
	if agent == nil {
		return
	}
	envVars[agentruntime.AgentEnvAgentID] = agent.ID.String()
	agentruntime.ApplyControlPlaneRuntimeEnv(envVars, cfg, agent, envVars[agentruntime.AgentEnvRuntimeSecret], agentruntime.ControlPlaneRuntimeEnvOptions{})
}

func setDriveEndpoint(envVars map[string]string, sandboxID uuid.UUID, cfg *config.Config) {
	envVars["HIVY_DRIVE_ENDPOINT"] = fmt.Sprintf("%s/internal/sandbox-drive/%s", cfg.RuntimeControlPlaneBaseURL(), sandboxID)
}

// setAssetsUploadURL exposes the conversation-asset endpoint base. The
// runtime appends the per-session conversation_id and the agent's chosen
// "<folder>/<filename>" tail so the final PUT URL is:
//
//	$HIVY_ASSETS_UPLOAD_URL/{conversationID}/assets/{folder}/{filename}
//
// Auth uses the same runtime secret already exported as HIVY_RUNTIME_SECRET.
func setAssetsUploadURL(envVars map[string]string, cfg *config.Config) {
	controlPlaneBaseURL := cfg.RuntimeControlPlaneBaseURL()
	envVars["HIVY_ASSETS_UPLOAD_URL"] = controlPlaneBaseURL + "/internal/conversations"
	envVars["HIVY_AGENT_ASSETS_UPLOAD_URL"] = controlPlaneBaseURL + "/internal/agents"
}

func agentDriveUploadURL(cfg *config.Config, agentID uuid.UUID) string {
	return agentruntime.AgentDriveUploadURL(cfg.RuntimeControlPlaneBaseURL(), agentID)
}

func setAgentDriveUploadURL(envVars map[string]string, cfg *config.Config, agentID uuid.UUID) {
	envVars[agentruntime.AgentEnvDriveUploadURL] = agentDriveUploadURL(cfg, agentID)
}

func setDriveUploadBearer(envVars map[string]string, bearer string) {
	envVars[agentruntime.AgentEnvDriveUploadBearer] = bearer
}

type repoResource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func generateRandomHex(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func shortID(id uuid.UUID) string {
	return strings.ReplaceAll(id.String(), "-", "")[:12]
}

func sanitizeName(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	s := b.String()
	if len(s) > 20 {
		s = s[:20]
	}
	return s
}
