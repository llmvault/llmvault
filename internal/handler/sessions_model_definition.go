package handler

import (
	"fmt"
	"strings"
)

func createSessionModelID(req createSessionRequest) string {
	if req.ModelDefinition != nil {
		if modelID := strings.TrimSpace(req.ModelDefinition.ModelID); modelID != "" {
			return modelID
		}
	}
	return strings.TrimSpace(req.Model)
}

func createSessionReasoningEffort(req createSessionRequest) string {
	if req.ModelDefinition != nil {
		if effort := strings.TrimSpace(req.ModelDefinition.ReasoningEffort); effort != "" {
			return effort
		}
	}
	return strings.TrimSpace(req.ReasoningEffort)
}

func normalizeSessionReasoningEffort(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "high", nil
	}
	switch value {
	case "low", "medium", "high":
		return value, nil
	default:
		return "", fmt.Errorf("reasoning_effort must be low, medium, or high")
	}
}
