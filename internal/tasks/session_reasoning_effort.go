package tasks

import (
	"strings"

	"github.com/usehivy/hivy/internal/model"
)

// sessionReasoningEffort honors the agent's configured default_reasoning_effort
// for autonomously-created sessions (Slack, triggers, schedules), mirroring the
// web create path. It falls back to "high" only when the agent sets no default.
func sessionReasoningEffort(agent model.Agent) string {
	if effort := strings.TrimSpace(agent.DefaultReasoningEffort); effort != "" {
		return effort
	}
	return "high"
}
