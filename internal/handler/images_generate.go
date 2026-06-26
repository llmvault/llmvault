package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

const (
	imageGenerationProviderID = "openrouter"
	imageGenerationTimeout    = 3 * time.Minute
	imageGenerationMaxCount   = 4
)

type imageGenerationRequest struct {
	Mode              string   `json:"mode,omitempty"`
	Prompt            string   `json:"prompt,omitempty"`
	Description       string   `json:"description,omitempty"`
	ReferenceAssetIDs []string `json:"reference_asset_ids,omitempty"`
	AspectRatio       string   `json:"aspect_ratio,omitempty"`
	Type              string   `json:"type,omitempty"`
	Count             int      `json:"count,omitempty"`
	N                 int      `json:"n,omitempty"`
	SessionID         string   `json:"session_id,omitempty"`
	HivySessionID     string   `json:"_hivy_session_id,omitempty"`
}

type imageGenerationToolResult struct {
	DriveAssetID      string   `json:"drive_asset_id"`
	ContentType       string   `json:"content_type"`
	Bytes             int64    `json:"bytes"`
	PublicURL         string   `json:"public_url"`
	ReferenceAssetIDs []string `json:"reference_asset_ids"`
}

type imageGenerationError struct {
	Error     string `json:"error"`
	ErrorCode string `json:"error_code,omitempty"`
}

func (h *UploadsHandler) WithImageGeneration(kms *crypto.KeyWrapper, reg *registry.Registry, client *http.Client) *UploadsHandler {
	h.imageKMS = kms
	if reg == nil {
		reg = registry.Global()
	}
	h.imageRegistry = reg
	if client == nil {
		client = &http.Client{Timeout: imageGenerationTimeout}
	}
	h.imageHTTPClient = client
	return h
}

func (h *UploadsHandler) runImageGeneration(ctx context.Context, agent *model.Agent, sandbox *model.Sandbox, req imageGenerationRequest) ([]imageGenerationToolResult, int, *imageGenerationError) {
	if h.streamer == nil || h.imageKMS == nil {
		return nil, http.StatusServiceUnavailable, &imageGenerationError{Error: "image generation is not configured", ErrorCode: "not_configured"}
	}
	vector := req.mode() == "vector"
	prompt := req.normalizedPrompt()
	if prompt == "" {
		return nil, http.StatusBadRequest, &imageGenerationError{Error: "prompt or description is required", ErrorCode: "prompt_required"}
	}

	modelID, err := h.resolveGenerationModel(ctx, agent, vector, req.sessionUUID())
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "image generation model resolution failed", "agent_id", agent.ID, "vector", vector, "error", err)
		return nil, http.StatusBadRequest, &imageGenerationError{Error: "failed to resolve image model", ErrorCode: "model_resolution_failed"}
	}
	route, ok := h.generationRegistry().ResolveModel(imageGenerationProviderID, modelID)
	if !ok {
		return nil, http.StatusServiceUnavailable, &imageGenerationError{Error: "image model route unavailable", ErrorCode: "model_route_unavailable"}
	}
	cred, err := credentials.ResolveForModel(ctx, h.db, h.generationRegistry(), *agent.OrgID, modelID)
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "image generation credential resolution failed", "agent_id", agent.ID, "model", modelID, "error", err)
		return nil, http.StatusServiceUnavailable, &imageGenerationError{Error: "image generation credential unavailable", ErrorCode: "credential_unavailable"}
	}
	apiKey, err := decryptCredentialKey(ctx, h.imageKMS, cred)
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "image generation credential decrypt failed", "agent_id", agent.ID, "credential_id", cred.ID, "error", err)
		return nil, http.StatusInternalServerError, &imageGenerationError{Error: "failed to decrypt image generation credential", ErrorCode: "internal_error"}
	}

	references, err := h.imageReferenceInputs(ctx, agent, req.ReferenceAssetIDs)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		return nil, status, &imageGenerationError{Error: err.Error(), ErrorCode: "reference_asset_error"}
	}

	callCtx, cancel := context.WithTimeout(ctx, imageGenerationTimeout)
	defer cancel()

	images, usage, err := h.callOpenRouterImages(callCtx, cred, string(apiKey), route.UpstreamID, prompt, req.aspectRatio(), req.count(), references)
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "image generation upstream failed", "agent_id", agent.ID, "model", modelID, "upstream_model", route.UpstreamID, "credential_id", cred.ID, "error", err)
		return nil, http.StatusBadGateway, &imageGenerationError{Error: "image generation failed", ErrorCode: "upstream_failed"}
	}
	results, err := h.storeGeneratedImages(ctx, agent, sandbox, vector, modelID, route.UpstreamID, prompt, req.ReferenceAssetIDs, images)
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "image generation asset storage failed", "agent_id", agent.ID, "model", modelID, "error", err)
		return nil, http.StatusInternalServerError, &imageGenerationError{Error: "failed to store generated image", ErrorCode: "asset_storage_failed"}
	}
	h.trackImageGenerationUsage(ctx, agent, sandbox, req, h.imageGenerationUsageSession(ctx, agent, req.sessionUUID()), cred, modelID, route.UpstreamID, usage, results)
	return results, http.StatusOK, nil
}

func normalizeImageGenerationRequest(req imageGenerationRequest) (imageGenerationRequest, error) {
	req.Mode = strings.ToLower(strings.TrimSpace(req.Mode))
	if req.Mode == "" {
		req.Mode = "raster"
	}
	if req.Mode != "raster" && req.Mode != "vector" {
		return req, errors.New("mode must be raster or vector")
	}
	return req, nil
}

func (r imageGenerationRequest) mode() string {
	return strings.ToLower(strings.TrimSpace(r.Mode))
}

func (r imageGenerationRequest) normalizedPrompt() string {
	prompt := strings.TrimSpace(r.Prompt)
	if prompt == "" {
		prompt = strings.TrimSpace(r.Description)
	}
	imageType := strings.TrimSpace(r.Type)
	if imageType != "" {
		return "Create a " + imageType + " image: " + prompt
	}
	return prompt
}

func (r imageGenerationRequest) aspectRatio() string {
	return strings.TrimSpace(r.AspectRatio)
}

func (r imageGenerationRequest) count() int {
	n := r.Count
	if n == 0 {
		n = r.N
	}
	if n <= 0 {
		return 1
	}
	if n > imageGenerationMaxCount {
		return imageGenerationMaxCount
	}
	return n
}

func (r imageGenerationRequest) sessionUUID() *uuid.UUID {
	raw := strings.TrimSpace(r.SessionID)
	if raw == "" {
		raw = strings.TrimSpace(r.HivySessionID)
	}
	if raw == "" {
		return nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil
	}
	return &id
}
