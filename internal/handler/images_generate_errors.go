package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/usehivy/hivy/internal/quiver"
	"github.com/usehivy/hivy/internal/reve"
)

func imageGenerationUpstreamError(providerID string, err error) *imageGenerationError {
	if providerID == imageGenerationReveProviderID {
		var statusErr *reve.StatusError
		if errors.As(err, &statusErr) {
			return &imageGenerationError{
				Error:     reveStatusToolMessage(statusErr),
				ErrorCode: upstreamErrorCode(statusErr.ErrorCode),
			}
		}
	}
	if providerID == imageGenerationQuiverProviderID {
		var statusErr *quiver.StatusError
		if errors.As(err, &statusErr) {
			return &imageGenerationError{
				Error:     quiverStatusToolMessage(statusErr),
				ErrorCode: upstreamErrorCode(statusErr.Code),
			}
		}
	}
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		msg = "image generation provider failed"
	}
	return &imageGenerationError{
		Error:     imageGenerationProviderName(providerID) + " image generation failed: " + msg,
		ErrorCode: "upstream_failed",
	}
}

func reveStatusToolMessage(err *reve.StatusError) string {
	parts := []string{fmt.Sprintf("Reve rejected the image generation request with status %d", err.StatusCode)}
	if strings.TrimSpace(err.ErrorCode) != "" {
		parts = append(parts, "code "+strings.TrimSpace(err.ErrorCode))
	}
	if strings.TrimSpace(err.Message) != "" {
		parts = append(parts, strings.TrimSpace(err.Message))
	} else if strings.TrimSpace(err.Body) != "" {
		parts = append(parts, strings.TrimSpace(err.Body))
	}
	if len(err.Params) > 0 {
		if data, marshalErr := json.Marshal(err.Params); marshalErr == nil {
			parts = append(parts, "details "+string(data))
		}
	}
	if err.StatusCode == http.StatusBadRequest {
		parts = append(parts, "retry with corrected tool arguments; aspect_ratio must be one of 16:9, 9:16, 3:2, 2:3, 4:3, 3:4, or 1:1, or omitted")
	}
	if strings.TrimSpace(err.RequestID) != "" {
		parts = append(parts, "request_id "+strings.TrimSpace(err.RequestID))
	}
	return strings.Join(parts, ": ")
}

func quiverStatusToolMessage(err *quiver.StatusError) string {
	parts := []string{fmt.Sprintf("Quiver rejected the SVG generation request with status %d", err.StatusCode)}
	if strings.TrimSpace(err.Code) != "" {
		parts = append(parts, "code "+strings.TrimSpace(err.Code))
	}
	if strings.TrimSpace(err.Message) != "" {
		parts = append(parts, strings.TrimSpace(err.Message))
	} else if strings.TrimSpace(err.Body) != "" {
		parts = append(parts, strings.TrimSpace(err.Body))
	}
	if err.StatusCode == http.StatusTooManyRequests && strings.TrimSpace(err.RetryAfter) != "" {
		parts = append(parts, "retry after "+strings.TrimSpace(err.RetryAfter)+"s")
	}
	if strings.TrimSpace(err.RequestID) != "" {
		parts = append(parts, "request_id "+strings.TrimSpace(err.RequestID))
	}
	return strings.Join(parts, ": ")
}

func imageGenerationProviderName(providerID string) string {
	switch providerID {
	case imageGenerationReveProviderID:
		return "Reve"
	case imageGenerationQuiverProviderID:
		return "Quiver"
	case imageGenerationOpenRouterProviderID:
		return "OpenRouter"
	default:
		return strings.TrimSpace(providerID)
	}
}

func upstreamErrorCode(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	if code == "" {
		return "upstream_failed"
	}
	code = strings.NewReplacer("-", "_", " ", "_", ".", "_").Replace(code)
	return "upstream_" + code
}

func imageGenerationProviderIDs() []string {
	return []string{imageGenerationReveProviderID, imageGenerationQuiverProviderID, imageGenerationOpenRouterProviderID}
}
