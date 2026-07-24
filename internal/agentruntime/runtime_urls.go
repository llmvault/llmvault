package agentruntime

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

func RuntimeEventWebSocketURL(cfg *config.Config, sandboxID uuid.UUID) string {
	if cfg == nil || sandboxID == uuid.Nil {
		return ""
	}
	base := strings.TrimRight(cfg.RuntimeControlPlaneBaseURL(), "/")
	parsed, err := url.Parse(base)
	if err == nil && parsed.Scheme != "" {
		switch parsed.Scheme {
		case "http":
			parsed.Scheme = "ws"
		case "https":
			parsed.Scheme = "wss"
		}
		base = strings.TrimRight(parsed.String(), "/")
	} else if strings.HasPrefix(base, "http://") {
		base = "ws://" + strings.TrimPrefix(base, "http://")
	} else if strings.HasPrefix(base, "https://") {
		base = "wss://" + strings.TrimPrefix(base, "https://")
	}
	return fmt.Sprintf("%s/internal/runtime-events/sandboxes/%s/sessions/{session_id}/ws", base, sandboxID)
}

func RuntimeTurnStateWebhookURL(cfg *config.Config, sandboxID uuid.UUID) string {
	if cfg == nil || sandboxID == uuid.Nil {
		return ""
	}
	base := strings.TrimRight(cfg.RuntimeControlPlaneBaseURL(), "/")
	return fmt.Sprintf("%s/internal/runtime-events/sandboxes/%s/turn-state", base, sandboxID)
}

func AgentGitUsername(agent *model.Agent) string {
	if agent == nil {
		return "agent"
	}
	if username := sanitizeGitIdentity(agent.Name); username != "" {
		return username
	}
	return "hivy"
}

func AgentGitEmail(agent *model.Agent) string {
	return AgentGitUsername(agent) + "@users.noreply.github.com"
}

func sanitizeGitIdentity(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
