package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/logging"
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

func (h *UploadsHandler) GenerateAgentImage(w http.ResponseWriter, r *http.Request) {
	if h.streamer == nil || h.imageKMS == nil {
		writeJSON(w, http.StatusServiceUnavailable, imageGenerationError{Error: "image generation is not configured", ErrorCode: "not_configured"})
		return
	}
	agent, sandbox, ok := h.authAgent(w, r)
	if !ok {
		return
	}

	req, err := decodeImageGenerationRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, imageGenerationError{Error: err.Error(), ErrorCode: "invalid_request"})
		return
	}
	vector := req.mode() == "vector"
	prompt := req.normalizedPrompt()
	if prompt == "" {
		writeJSON(w, http.StatusBadRequest, imageGenerationError{Error: "prompt or description is required", ErrorCode: "prompt_required"})
		return
	}

	modelID, err := h.resolveGenerationModel(r.Context(), agent, vector, req.sessionUUID())
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "image generation model resolution failed", "agent_id", agent.ID, "vector", vector, "error", err)
		writeJSON(w, http.StatusBadRequest, imageGenerationError{Error: "failed to resolve image model", ErrorCode: "model_resolution_failed"})
		return
	}
	route, ok := h.generationRegistry().ResolveModel(imageGenerationProviderID, modelID)
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, imageGenerationError{Error: "image model route unavailable", ErrorCode: "model_route_unavailable"})
		return
	}
	cred, err := credentials.ResolveForModel(r.Context(), h.db, h.generationRegistry(), *agent.OrgID, modelID)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "image generation credential resolution failed", "agent_id", agent.ID, "model", modelID, "error", err)
		writeJSON(w, http.StatusServiceUnavailable, imageGenerationError{Error: "image generation credential unavailable", ErrorCode: "credential_unavailable"})
		return
	}
	apiKey, err := decryptCredentialKey(r.Context(), h.imageKMS, cred)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "image generation credential decrypt failed", "agent_id", agent.ID, "credential_id", cred.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, imageGenerationError{Error: "failed to decrypt image generation credential", ErrorCode: "internal_error"})
		return
	}

	references, err := h.imageReferenceInputs(r.Context(), agent, req.ReferenceAssetIDs)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, imageGenerationError{Error: err.Error(), ErrorCode: "reference_asset_error"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), imageGenerationTimeout)
	defer cancel()

	images, err := h.callOpenRouterImages(ctx, cred, string(apiKey), route.UpstreamID, prompt, req.aspectRatio(), req.count(), references)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "image generation upstream failed", "agent_id", agent.ID, "model", modelID, "upstream_model", route.UpstreamID, "credential_id", cred.ID, "error", err)
		writeJSON(w, http.StatusBadGateway, imageGenerationError{Error: "image generation failed", ErrorCode: "upstream_failed"})
		return
	}
	results, err := h.storeGeneratedImages(r.Context(), agent, sandbox, vector, modelID, route.UpstreamID, prompt, req.ReferenceAssetIDs, images)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "image generation asset storage failed", "agent_id", agent.ID, "model", modelID, "error", err)
		writeJSON(w, http.StatusInternalServerError, imageGenerationError{Error: "failed to store generated image", ErrorCode: "asset_storage_failed"})
		return
	}
	writeJSON(w, http.StatusOK, results)
}

func decodeImageGenerationRequest(r *http.Request) (imageGenerationRequest, error) {
	var req imageGenerationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return req, errors.New("invalid request body")
	}
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
