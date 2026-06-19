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

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/storage"
	"github.com/usehivy/hivy/internal/system"
)

const (
	imageDescribeCanonicalModel = "gemini-3.5-flash"
	imageDescribeProviderID     = "openrouter"
	imageDescribeTimeout        = time.Minute
	imageDescribeMaxTokens      = 6000
)

type ImageDescribeHandler struct {
	db                  *gorm.DB
	kms                 *crypto.KeyWrapper
	registry            *registry.Registry
	gateway             system.Gateway
	assetReader         storage.Reader
	assetPreviewBaseURL string
}

func NewImageDescribeHandler(db *gorm.DB, kms *crypto.KeyWrapper, reg *registry.Registry, gateway system.Gateway, assetPreviewBaseURL string) *ImageDescribeHandler {
	if reg == nil {
		reg = registry.Global()
	}
	return &ImageDescribeHandler{
		db:                  db,
		kms:                 kms,
		registry:            reg,
		gateway:             gateway,
		assetPreviewBaseURL: strings.TrimRight(strings.TrimSpace(assetPreviewBaseURL), "/"),
	}
}

func (h *ImageDescribeHandler) WithAssetReader(reader storage.Reader) *ImageDescribeHandler {
	h.assetReader = reader
	return h
}

type imageDescribeRequest struct {
	DriveAssetID string `json:"drive_asset_id"`
	DetailLevel  string `json:"detail_level,omitempty"`
}

type imageDescribeResponse struct {
	DriveAssetID        string         `json:"drive_asset_id"`
	AssetURL            string         `json:"asset_url"`
	Filename            string         `json:"filename"`
	ContentType         string         `json:"content_type"`
	Category            string         `json:"category"`
	Confidence          float64        `json:"confidence"`
	Analysis            map[string]any `json:"analysis"`
	RenderedDescription string         `json:"rendered_description"`
}

type imageDescribeError struct {
	Error     string `json:"error"`
	ErrorCode string `json:"error_code,omitempty"`
}

// Describe handles POST /v1/images/describe.
func (h *ImageDescribeHandler) Describe(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	userID := currentUserID(r)

	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusUnauthorized, imageDescribeError{Error: "missing org context", ErrorCode: "unauthorized"})
		return
	}
	if h.gateway == nil {
		writeJSON(w, http.StatusServiceUnavailable, imageDescribeError{Error: "image description unavailable", ErrorCode: "system_model_unavailable"})
		return
	}

	var req imageDescribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, imageDescribeError{Error: "invalid request body", ErrorCode: "invalid_request_body"})
		return
	}
	assetID, err := uuid.Parse(strings.TrimSpace(req.DriveAssetID))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, imageDescribeError{Error: "invalid drive_asset_id", ErrorCode: "invalid_drive_asset_id"})
		return
	}
	detailLevel := strings.ToLower(strings.TrimSpace(req.DetailLevel))
	if detailLevel == "" {
		detailLevel = "high"
	}
	if detailLevel != "low" && detailLevel != "medium" && detailLevel != "high" {
		writeJSON(w, http.StatusBadRequest, imageDescribeError{Error: "invalid detail_level", ErrorCode: "invalid_detail_level"})
		return
	}

	var asset model.AgentAsset
	if err := h.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ?", assetID, org.ID).
		First(&asset).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, imageDescribeError{Error: "asset not found", ErrorCode: "asset_not_found"})
			return
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "image describe asset lookup failed",
			"operation", "images.describe",
			"request_path", "/v1/images/describe",
			"org_id", org.ID,
			"user_id", userID,
			"drive_asset_id", assetID,
			"error_code", "internal_error",
			"http_status", http.StatusInternalServerError,
			"duration_ms", time.Since(startedAt).Milliseconds(),
			"error", err,
		)
		writeJSON(w, http.StatusInternalServerError, imageDescribeError{Error: "failed to load asset", ErrorCode: "internal_error"})
		return
	}
	if !isImageContentType(asset.ContentType) {
		writeJSON(w, http.StatusUnprocessableEntity, imageDescribeError{Error: "asset is not an image", ErrorCode: "unsupported_content_type"})
		return
	}
	baseLogAttrs := func() []any {
		return imageDescribeLogAttrs(startedAt, org.ID, userID, asset, detailLevel)
	}

	cred, err := h.openRouterSystemCredential(r.Context())
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logging.FromContext(r.Context()).ErrorContext(r.Context(), "image describe system credential unavailable",
				appendLogAttrs(baseLogAttrs(),
					"error_code", "system_credential_unavailable",
					"http_status", http.StatusServiceUnavailable,
					"provider_id", imageDescribeProviderID,
					"error", err,
				)...,
			)
			writeJSON(w, http.StatusServiceUnavailable, imageDescribeError{Error: "OpenRouter system credential unavailable", ErrorCode: "system_credential_unavailable"})
			return
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "image describe system credential lookup failed",
			appendLogAttrs(baseLogAttrs(),
				"error_code", "internal_error",
				"http_status", http.StatusInternalServerError,
				"provider_id", imageDescribeProviderID,
				"error", err,
			)...,
		)
		writeJSON(w, http.StatusInternalServerError, imageDescribeError{Error: "failed to load system credential", ErrorCode: "internal_error"})
		return
	}
	if strings.TrimSpace(cred.BaseURL) == "" {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "image describe system credential endpoint unavailable",
			appendLogAttrs(baseLogAttrs(),
				"error_code", "system_credential_unavailable",
				"http_status", http.StatusServiceUnavailable,
				"provider_id", imageDescribeProviderID,
				"credential_id", cred.ID,
			)...,
		)
		writeJSON(w, http.StatusServiceUnavailable, imageDescribeError{Error: "OpenRouter system credential endpoint unavailable", ErrorCode: "system_credential_unavailable"})
		return
	}

	route, ok := h.registry.ResolveModel(imageDescribeProviderID, imageDescribeCanonicalModel)
	if !ok {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "image describe model route unavailable",
			appendLogAttrs(baseLogAttrs(),
				"error_code", "system_model_unavailable",
				"http_status", http.StatusServiceUnavailable,
				"provider_id", imageDescribeProviderID,
				"canonical_model", imageDescribeCanonicalModel,
				"credential_id", cred.ID,
				"credential_base_url_host", urlHost(cred.BaseURL),
			)...,
		)
		writeJSON(w, http.StatusServiceUnavailable, imageDescribeError{Error: "image description model unavailable", ErrorCode: "system_model_unavailable"})
		return
	}

	assetURL := h.assetURL(asset)
	if assetURL == "" {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "image describe asset preview unavailable",
			appendLogAttrs(baseLogAttrs(),
				"error_code", "asset_preview_unavailable",
				"http_status", http.StatusServiceUnavailable,
				"provider_id", imageDescribeProviderID,
				"canonical_model", imageDescribeCanonicalModel,
				"upstream_model", route.UpstreamID,
				"credential_id", cred.ID,
				"credential_base_url_host", urlHost(cred.BaseURL),
				"asset_preview_base_url_host", urlHost(h.assetPreviewBaseURL),
			)...,
		)
		writeJSON(w, http.StatusServiceUnavailable, imageDescribeError{Error: "asset preview unavailable", ErrorCode: "asset_preview_unavailable"})
		return
	}
	imageMetadata := h.loadImageMetadata(r.Context(), asset)
	describeLogAttrs := func() []any {
		return imageDescribeGatewayLogAttrs(baseLogAttrs(), cred, route, assetURL, imageMetadata)
	}

	apiKey, err := decryptCredentialKey(r.Context(), h.kms, cred)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "image describe credential decrypt failed",
			appendLogAttrs(describeLogAttrs(),
				"error_code", "internal_error",
				"http_status", http.StatusInternalServerError,
				"error", err,
			)...,
		)
		writeJSON(w, http.StatusInternalServerError, imageDescribeError{Error: "failed to decrypt system credential", ErrorCode: "internal_error"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), imageDescribeTimeout)
	defer cancel()

	llmReq := buildImageDescribeLLMRequest(route.UpstreamID, asset, assetURL, detailLevel, imageMetadata)
	res, err := h.gateway.Complete(ctx, system.ForwardCall{
		ProviderID: imageDescribeProviderID,
		BaseURL:    cred.BaseURL,
		APIKey:     string(apiKey),
		AuthScheme: cred.AuthScheme,
		Request:    llmReq,
		Stream:     false,
	})
	if err != nil {
		h.handleDescribeUpstreamError(w, r, err, describeLogAttrs())
		return
	}

	analysis, category, confidence, err := parseImageAnalysis(res.Text)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "image describe model returned invalid output",
			appendLogAttrs(describeLogAttrs(),
				"error_code", "invalid_model_output",
				"http_status", http.StatusBadGateway,
				"completion_model", res.Model,
				"model_output_bytes", len(res.Text),
				"input_tokens", res.Usage.InputTokens,
				"output_tokens", res.Usage.OutputTokens,
				"cached_tokens", res.Usage.CachedTokens,
				"reasoning_tokens", res.Usage.ReasoningTokens,
				"error", err,
			)...,
		)
		writeJSON(w, http.StatusBadGateway, imageDescribeError{Error: "invalid image analysis output", ErrorCode: "invalid_model_output"})
		return
	}
	if imageMetadata != nil {
		analysis["auto_extracted_image_metadata"] = imageMetadata
	}
	rendered := renderImageDescription(category, confidence, analysis)
	rendered = stripBackendModelNames(rendered)

	h.writeImageDescribeGeneration(r.Context(), cred, org.ID, userID, res)

	writeJSON(w, http.StatusOK, imageDescribeResponse{
		DriveAssetID:        asset.ID.String(),
		AssetURL:            assetURL,
		Filename:            asset.Filename,
		ContentType:         asset.ContentType,
		Category:            category,
		Confidence:          confidence,
		Analysis:            analysis,
		RenderedDescription: rendered,
	})
}
