package sandbox

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/config"
)

const (
	RuntimePort = 25434

	runtimeURLRefreshBuffer = 5 * time.Minute
	runtimeURLTTL           = 55 * time.Minute
)

func agentDriveUploadURL(cfg *config.Config, agentID, sandboxID uuid.UUID) string {
	return agentruntime.AgentDriveUploadURL(cfg.RuntimeControlPlaneBaseURL(), agentID, sandboxID)
}

func setSandboxSentryEnvVars(envVars map[string]string, cfg *config.Config, dsn string) {
	agentruntime.ApplySandboxSentryEnv(envVars, cfg, dsn)
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
