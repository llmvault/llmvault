package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/system"
)

const (
	imageDescribeCanonicalModel = "gemini-3.5-flash"
	imageDescribeProviderID     = "openrouter"
	imageDescribeTimeout        = 12 * time.Second
	imageDescribeMaxTokens      = 1800
)

type ImageDescribeHandler struct {
	db                  *gorm.DB
	kms                 *crypto.KeyWrapper
	registry            *registry.Registry
	gateway             system.Gateway
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
		writeJSON(w, http.StatusInternalServerError, imageDescribeError{Error: "failed to load asset", ErrorCode: "internal_error"})
		return
	}
	if !isImageContentType(asset.ContentType) {
		writeJSON(w, http.StatusUnprocessableEntity, imageDescribeError{Error: "asset is not an image", ErrorCode: "unsupported_content_type"})
		return
	}

	cred, err := h.openRouterSystemCredential(r.Context())
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusServiceUnavailable, imageDescribeError{Error: "OpenRouter system credential unavailable", ErrorCode: "system_credential_unavailable"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, imageDescribeError{Error: "failed to load system credential", ErrorCode: "internal_error"})
		return
	}
	if strings.TrimSpace(cred.BaseURL) == "" {
		writeJSON(w, http.StatusServiceUnavailable, imageDescribeError{Error: "OpenRouter system credential endpoint unavailable", ErrorCode: "system_credential_unavailable"})
		return
	}

	route, ok := h.registry.ResolveModel(imageDescribeProviderID, imageDescribeCanonicalModel)
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, imageDescribeError{Error: "image description model unavailable", ErrorCode: "system_model_unavailable"})
		return
	}

	apiKey, err := decryptCredentialKey(r.Context(), h.kms, cred)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "image describe credential decrypt failed", "error", err, "credential_id", cred.ID)
		writeJSON(w, http.StatusInternalServerError, imageDescribeError{Error: "failed to decrypt system credential", ErrorCode: "internal_error"})
		return
	}

	assetURL := h.assetURL(asset)
	if assetURL == "" {
		writeJSON(w, http.StatusServiceUnavailable, imageDescribeError{Error: "asset preview unavailable", ErrorCode: "asset_preview_unavailable"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), imageDescribeTimeout)
	defer cancel()

	llmReq := buildImageDescribeLLMRequest(route.UpstreamID, asset, assetURL, detailLevel)
	res, err := h.gateway.Complete(ctx, system.ForwardCall{
		ProviderID: imageDescribeProviderID,
		BaseURL:    cred.BaseURL,
		APIKey:     string(apiKey),
		AuthScheme: cred.AuthScheme,
		Request:    llmReq,
		Stream:     false,
	})
	if err != nil {
		h.handleDescribeUpstreamError(w, r, err)
		return
	}

	analysis, category, confidence, err := parseImageAnalysis(res.Text)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, imageDescribeError{Error: "invalid image analysis output", ErrorCode: "invalid_model_output"})
		return
	}
	rendered := renderImageDescription(category, confidence, analysis)
	rendered = stripBackendModelNames(rendered)

	h.writeImageDescribeGeneration(r.Context(), cred, org.ID, currentUserID(r), res)

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

func (h *ImageDescribeHandler) openRouterSystemCredential(ctx context.Context) (*model.Credential, error) {
	var cred model.Credential
	if err := h.db.WithContext(ctx).
		Where("org_id IS NULL AND revoked_at IS NULL AND provider_id = ?", imageDescribeProviderID).
		Order("created_at DESC").
		First(&cred).Error; err != nil {
		return nil, err
	}
	return &cred, nil
}

func (h *ImageDescribeHandler) assetURL(asset model.AgentAsset) string {
	if h.assetPreviewBaseURL != "" && asset.Key != "" {
		return buildAssetPreviewURL(h.assetPreviewBaseURL, asset.Key)
	}
	return asset.PublicURL
}

func buildImageDescribeLLMRequest(modelID string, asset model.AgentAsset, assetURL, detailLevel string) *system.LLMRequest {
	userPrompt := fmt.Sprintf(`Analyze the uploaded image as structured attachment context.

Filename: %s
Content type: %s
Detail level: %s

Return only the JSON object required by the system instructions.`, asset.Filename, asset.ContentType, detailLevel)
	temperature := float32(0)
	return &system.LLMRequest{
		Model: modelID,
		Messages: []system.LLMMessage{
			{Role: "system", Content: imageDescriptionSystemPrompt},
			{
				Role: "user",
				Parts: []system.LLMPart{
					{Kind: system.LLMPartText, Text: userPrompt},
					{Kind: system.LLMPartMedia, ContentType: asset.ContentType, Text: assetURL},
				},
			},
		},
		MaxTokens:      imageDescribeMaxTokens,
		Temperature:    &temperature,
		ResponseFormat: system.JSONResponseSpec(),
	}
}

func (h *ImageDescribeHandler) handleDescribeUpstreamError(w http.ResponseWriter, r *http.Request, err error) {
	logger := logging.FromContext(r.Context())
	var upErr *system.UpstreamError
	if errors.As(err, &upErr) {
		logger.ErrorContext(r.Context(), "image describe upstream rejected request",
			"status", upErr.StatusCode,
			"body", truncateForLog(upErr.Body, 512),
			"error", err,
		)
		writeJSON(w, http.StatusBadGateway, imageDescribeError{Error: "upstream error", ErrorCode: "upstream_error"})
		return
	}
	logger.ErrorContext(r.Context(), "image describe upstream failed", "error", err)
	writeJSON(w, http.StatusBadGateway, imageDescribeError{Error: "upstream error", ErrorCode: "upstream_error"})
}

func (h *ImageDescribeHandler) writeImageDescribeGeneration(ctx context.Context, cred *model.Credential, orgID uuid.UUID, userID string, res *system.CompletionResult) {
	if res == nil {
		return
	}
	gen := model.Generation{
		ID:             "gen_" + ulid.Make().String(),
		OrgID:          orgID,
		CredentialID:   cred.ID,
		TokenJTI:       "system:images.describe",
		ProviderID:     imageDescribeProviderID,
		Model:          imageDescribeCanonicalModel,
		RequestPath:    "/v1/images/describe",
		IsStreaming:    false,
		InputTokens:    res.Usage.InputTokens,
		OutputTokens:   res.Usage.OutputTokens,
		CachedTokens:   res.Usage.CachedTokens,
		UpstreamStatus: http.StatusOK,
		UserID:         userID,
		CreatedAt:      time.Now(),
	}
	if err := h.db.WithContext(ctx).Create(&gen).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "image describe generation row write failed", "error", err, "generation_id", gen.ID)
	}
}

func parseImageAnalysis(raw string) (map[string]any, string, float64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, "", 0, errors.New("empty image analysis")
	}
	if !strings.HasPrefix(raw, "{") {
		start := strings.Index(raw, "{")
		end := strings.LastIndex(raw, "}")
		if start < 0 || end <= start {
			return nil, "", 0, errors.New("analysis is not JSON")
		}
		raw = raw[start : end+1]
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var analysis map[string]any
	if err := dec.Decode(&analysis); err != nil {
		return nil, "", 0, err
	}
	category, _ := analysis["category"].(string)
	category = strings.TrimSpace(category)
	if category == "" {
		return nil, "", 0, errors.New("missing category")
	}
	confidence, ok := numberValue(analysis["confidence"])
	if !ok {
		return nil, "", 0, errors.New("missing confidence")
	}
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}
	return analysis, category, confidence, nil
}

func numberValue(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

func renderImageDescription(category string, confidence float64, analysis map[string]any) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Primary category: %s\n", humanCategory(category))
	fmt.Fprintf(&b, "Confidence: %.2f\n", confidence)
	if summary := stringField(analysis, "summary"); summary != "" {
		fmt.Fprintf(&b, "\nSummary:\n%s\n", summary)
	}
	writeAnalysisSection(&b, "Visible text", analysis["visible_text"])
	writeAnalysisSection(&b, "Layout", analysis["layout"])
	writeAnalysisSection(&b, "Objects and UI elements", analysis["objects"])
	writeAnalysisSection(&b, "Approximate colors", analysis["colors"])
	writeAnalysisSection(&b, "Visible states", analysis["states"])
	writeAnalysisSection(&b, "Relationships", analysis["relationships"])
	writeAnalysisSection(&b, "Important details", analysis["important_details"])
	writeAnalysisSection(&b, "Limitations", analysis["limitations"])
	writeAnalysisSection(&b, "Untrusted image instructions", analysis["untrusted_image_instructions"])
	writeAnalysisSection(&b, "Category-specific details", analysis["category_specific"])
	return strings.TrimSpace(b.String())
}

func writeAnalysisSection(b *strings.Builder, title string, value any) {
	text := compactAnalysisValue(value, 0)
	if strings.TrimSpace(text) == "" || strings.TrimSpace(text) == "[]" || strings.TrimSpace(text) == "{}" {
		return
	}
	fmt.Fprintf(b, "\n%s:\n%s\n", title, text)
}

func compactAnalysisValue(value any, indent int) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case json.Number:
		return v.String()
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case []any:
		lines := make([]string, 0, len(v))
		for _, item := range v {
			text := compactAnalysisValue(item, indent+2)
			if text == "" {
				continue
			}
			lines = append(lines, strings.Repeat(" ", indent)+"- "+indentMultiline(text, indent+2))
		}
		return strings.Join(lines, "\n")
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		lines := make([]string, 0, len(keys))
		for _, k := range keys {
			text := compactAnalysisValue(v[k], indent+2)
			if text == "" {
				continue
			}
			lines = append(lines, strings.Repeat(" ", indent)+k+": "+indentMultiline(text, indent+2))
		}
		return strings.Join(lines, "\n")
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(raw)
	}
}

func indentMultiline(text string, indent int) string {
	text = strings.TrimSpace(text)
	if !strings.Contains(text, "\n") {
		return text
	}
	prefix := "\n" + strings.Repeat(" ", indent)
	return strings.ReplaceAll(text, "\n", prefix)
}

func stringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return strings.TrimSpace(v)
}

func humanCategory(category string) string {
	parts := strings.Split(strings.ReplaceAll(category, "_", " "), " ")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func isImageContentType(contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	return strings.HasPrefix(contentType, "image/")
}

func currentUserID(r *http.Request) string {
	if claims, ok := middleware.AuthClaimsFromContext(r.Context()); ok && claims != nil {
		return claims.UserID
	}
	return ""
}

func truncateForLog(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}

var backendModelNamePattern = regexp.MustCompile(`(?i)\b(openrouter|google/gemini-3\.5-flash|gemini-3\.5-flash)\b`)

func stripBackendModelNames(raw string) string {
	return strings.TrimSpace(backendModelNamePattern.ReplaceAllString(raw, "[redacted backend model]"))
}
