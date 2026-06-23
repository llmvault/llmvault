package handler

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func (h *BrandHandler) loadBrandAssetForRequest(w http.ResponseWriter, r *http.Request, brand model.Brand) (model.BrandAsset, bool) {
	assetID, ok := brandAssetIDFromRequest(w, r)
	if !ok {
		return model.BrandAsset{}, false
	}
	var asset model.BrandAsset
	err := h.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ? AND brand_id = ?", assetID, brand.OrgID, brand.ID).
		First(&asset).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "brand asset not found"})
		return model.BrandAsset{}, false
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load brand asset"})
		return model.BrandAsset{}, false
	}
	return asset, true
}

func brandAssetIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "assetID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid brand asset id"})
		return uuid.Nil, false
	}
	return id, true
}

func validateBrandAssetURL(raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fmt.Errorf("public_url is required")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("public_url must be an absolute URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("public_url must use http or https")
	}
	return nil
}

func brandLogosReferenceAsset(brand model.Brand, assetID uuid.UUID) bool {
	ids, err := collectLogoAssetIDs(brand.Logos)
	if err != nil {
		return false
	}
	for _, id := range ids {
		if id == assetID {
			return true
		}
	}
	return false
}

func brandAssetToResponse(asset model.BrandAsset) brandAssetResponse {
	return brandAssetResponse{
		ID:          asset.ID.String(),
		OrgID:       asset.OrgID.String(),
		BrandID:     asset.BrandID.String(),
		Kind:        asset.Kind,
		Role:        asset.Role,
		Name:        asset.Name,
		Key:         asset.Key,
		PublicURL:   asset.PublicURL,
		ContentType: asset.ContentType,
		Bytes:       asset.Bytes,
		Width:       asset.Width,
		Height:      asset.Height,
		Metadata:    normalizeJSONPtr(&asset.Metadata),
		CreatedBy:   formatUUIDPtr(asset.CreatedBy),
		CreatedAt:   asset.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   asset.UpdatedAt.Format(time.RFC3339),
	}
}
