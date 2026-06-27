package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func (h *UploadsHandler) imageGenerationUsageSession(ctx context.Context, agent *model.Agent, sessionID *uuid.UUID) *model.Session {
	if h == nil || h.db == nil || agent == nil || agent.OrgID == nil || sessionID == nil {
		return nil
	}
	var session model.Session
	if err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND agent_id = ?", *sessionID, *agent.OrgID, agent.ID).
		First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		logging.Capture(ctx, err)
		return nil
	}
	return &session
}

func (h *UploadsHandler) trackImageGenerationUsage(ctx context.Context, agent *model.Agent, sandbox *model.Sandbox, req imageGenerationRequest, session *model.Session, cred *model.Credential, providerID, modelID, upstreamModel string, usage imageGenerationUsage, results []imageGenerationToolResult) {
	if agent == nil || agent.OrgID == nil || sandbox == nil || cred == nil {
		return
	}
	vector := req.mode() == "vector"
	toolName := imageGenerationRasterToolName
	if vector {
		toolName = imageGenerationVectorToolName
	}
	metadata := model.JSON{
		"mode":                req.mode(),
		"tool_name":           toolName,
		"aspect_ratio":        strings.TrimSpace(req.AspectRatio),
		"count":               len(results),
		"generated_asset_ids": imageGenerationResultAssetIDs(results),
		"reference_asset_ids": cleanReferenceAssetIDs(req.ReferenceAssetIDs),
		"upstream_model":      upstreamModel,
	}
	if usage.CreditsUsed > 0 {
		metadata["credits_used"] = usage.CreditsUsed
	}
	if usage.CreditsRemaining > 0 {
		metadata["credits_remaining"] = usage.CreditsRemaining
	}
	if len(usage.RequestIDs) > 0 {
		metadata["request_ids"] = usage.RequestIDs
	}
	if usage.ContentViolation {
		metadata["content_violation"] = true
	}

	payload := buildModelUsagePayload(modelUsageInput{
		Operation:       "image_generation",
		OrgID:           *agent.OrgID,
		ProviderID:      providerID,
		Model:           modelID,
		RequestPath:     "/mcp/tools/" + toolName,
		TokenJTI:        "system:images.generate",
		UserID:          "agent:" + agent.ID.String(),
		Session:         session,
		SandboxID:       &sandbox.ID,
		Credential:      cred,
		InputTokens:     usage.PromptTokens,
		OutputTokens:    usage.CompletionTokens,
		CachedTokens:    usage.CachedTokens,
		ReasoningTokens: usage.ReasoningTokens,
		TotalTokens:     usage.TotalTokens,
		Cost:            usage.Cost,
		UpstreamStatus:  http.StatusOK,
		Metadata:        metadata,
	})
	dispatchModelUsage(ctx, h.db, h.usageEnqueuer, payload)
}

func imageGenerationResultAssetIDs(results []imageGenerationToolResult) []string {
	out := make([]string, 0, len(results))
	for _, result := range results {
		if value := strings.TrimSpace(result.DriveAssetID); value != "" {
			out = append(out, value)
		}
	}
	return out
}
