package handler

import (
	"fmt"
	"strings"
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
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", nil
	}
	switch value {
	case "low", "medium", "high":
		return value, nil
	default:
		return "", fmt.Errorf("reasoning_effort must be low, medium, or high")
	}
}
