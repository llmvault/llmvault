package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/system"
)

func (h *ImageDescribeHandler) handleDescribeUpstreamError(w http.ResponseWriter, r *http.Request, err error, logAttrs []any) {
	logger := logging.FromContext(r.Context())
	var upErr *system.UpstreamError
	if errors.As(err, &upErr) {
		logger.ErrorContext(r.Context(), "image describe upstream rejected request",
			appendLogAttrs(logAttrs,
				"error_code", "upstream_error",
				"http_status", http.StatusBadGateway,
				"upstream_status", upErr.StatusCode,
				"upstream_body", truncateForLog(upErr.Body, 512),
				"error_type", fmt.Sprintf("%T", err),
				"error", err,
			)...,
		)
		writeJSON(w, http.StatusBadGateway, imageDescribeError{Error: "upstream error", ErrorCode: "upstream_error"})
		return
	}
	logger.ErrorContext(r.Context(), "image describe upstream failed",
		appendLogAttrs(logAttrs,
			"error_code", "upstream_error",
			"http_status", http.StatusBadGateway,
			"context_deadline_exceeded", errors.Is(err, context.DeadlineExceeded),
			"context_canceled", errors.Is(err, context.Canceled),
			"error_type", fmt.Sprintf("%T", err),
			"error", err,
		)...,
	)
	writeJSON(w, http.StatusBadGateway, imageDescribeError{Error: "upstream error", ErrorCode: "upstream_error"})
}

func imageDescribeLogAttrs(startedAt time.Time, orgID uuid.UUID, userID string, asset model.AgentAsset, detailLevel string) []any {
	return []any{
		"operation", "images.describe",
		"request_path", "/v1/images/describe",
		"org_id", orgID,
		"user_id", userID,
		"drive_asset_id", asset.ID,
		"agent_id", asset.AgentID,
		"sandbox_id", asset.SandboxID,
		"asset_path", asset.Path,
		"asset_key", asset.Key,
		"filename", asset.Filename,
		"content_type", asset.ContentType,
		"bytes", asset.Bytes,
		"detail_level", detailLevel,
		"timeout_ms", imageDescribeTimeout.Milliseconds(),
		"max_output_tokens", imageDescribeMaxTokens,
		"duration_ms", time.Since(startedAt).Milliseconds(),
	}
}

func imageDescribeGatewayLogAttrs(base []any, cred *model.Credential, route registry.ResolvedModelRoute, assetURL string, imageMetadata map[string]any) []any {
	attrs := appendLogAttrs(base,
		"provider_id", imageDescribeProviderID,
		"canonical_model", imageDescribeCanonicalModel,
		"route_provider_id", route.ProviderID,
		"upstream_model", route.UpstreamID,
		"credential_id", cred.ID,
		"credential_base_url_host", urlHost(cred.BaseURL),
		"auth_scheme", cred.AuthScheme,
		"asset_url_host", urlHost(assetURL),
		"response_format", "json_object",
		"stream", false,
		"temperature", 0,
		"metadata_extracted", len(imageMetadata) > 0,
	)
	if len(imageMetadata) == 0 {
		return attrs
	}
	for _, key := range []string{
		"format",
		"width",
		"height",
		"pixel_count",
		"has_alpha",
		"has_transparency",
		"transparent_pixel_ratio",
		"visible_nontransparent_ratio",
		"pixel_analysis_skipped",
		"pixel_analysis_skip_reason",
	} {
		if value, ok := imageMetadata[key]; ok {
			attrs = append(attrs, "metadata_"+key, value)
		}
	}
	return attrs
}

func appendLogAttrs(base []any, extra ...any) []any {
	out := make([]any, 0, len(base)+len(extra))
	out = append(out, base...)
	out = append(out, extra...)
	return out
}

func truncateForLog(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}

func urlHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}
