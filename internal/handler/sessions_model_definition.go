package handler

import (
	"fmt"
	"strings"

	"github.com/usehivy/hivy/internal/model"
)

func createSessionModelID(req createSessionRequest) string {
	if req.ModelDefinition != nil {
		return strings.TrimSpace(req.ModelDefinition.ModelID)
	}
	return ""
}

func createSessionReasoningEffort(req createSessionRequest) string {
	if req.ModelDefinition != nil {
		return strings.TrimSpace(req.ModelDefinition.ReasoningEffort)
	}
	return ""
}

func normalizeSessionReasoningEffort(value string) (string, error) {
	normalized, ok := model.NormalizeReasoningEffort(value)
	if !ok {
		return "", fmt.Errorf("reasoning_effort must be low, medium, or high")
	}
	return normalized, nil
}

// resolveSessionReasoningEffort picks the reasoning effort for a new session:
// an explicit per-session value always wins, otherwise the agent's configured
// default is used, otherwise empty (the session applies its own default).
func resolveSessionReasoningEffort(req createSessionRequest, agent model.Agent) string {
	explicit, _ := normalizeSessionReasoningEffort(createSessionReasoningEffort(req))
	if explicit != "" {
		return explicit
	}
	return strings.TrimSpace(agent.DefaultReasoningEffort)
}
