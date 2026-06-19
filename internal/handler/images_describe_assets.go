package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

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

func (h *ImageDescribeHandler) loadImageMetadata(ctx context.Context, asset model.AgentAsset) map[string]any {
	if h.assetReader == nil || strings.TrimSpace(asset.Key) == "" {
		return nil
	}
	metadata, err := extractImageMetadataFromAsset(ctx, h.assetReader, asset)
	if err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "image metadata extraction failed",
			"operation", "images.describe",
			"org_id", asset.OrgID,
			"asset_id", asset.ID,
			"agent_id", asset.AgentID,
			"sandbox_id", asset.SandboxID,
			"key", asset.Key,
			"filename", asset.Filename,
			"content_type", asset.ContentType,
			"bytes", asset.Bytes,
			"error_code", "metadata_extraction_failed",
			"error", err,
		)
		return nil
	}
	return metadata
}

func (h *ImageDescribeHandler) storeImageDescription(ctx context.Context, assetID uuid.UUID, resp imageDescribeResponse) error {
	raw, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	description := model.RawJSON(raw)
	return h.db.WithContext(ctx).
		Model(&model.AgentAsset{}).
		Where("id = ?", assetID).
		Update("description", description).Error
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
