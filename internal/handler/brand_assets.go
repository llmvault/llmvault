package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

var validBrandAssetKinds = map[string]bool{
	"logo":     true,
	"mark":     true,
	"icon":     true,
	"image":    true,
	"font":     true,
	"document": true,
	"other":    true,
}

type createBrandAssetRequest struct {
	Kind        string     `json:"kind"`
	Role        string     `json:"role,omitempty"`
	Name        string     `json:"name"`
	Key         string     `json:"key"`
	PublicURL   string     `json:"public_url"`
	ContentType string     `json:"content_type"`
	Bytes       int64      `json:"bytes"`
	Width       *int       `json:"width,omitempty"`
	Height      *int       `json:"height,omitempty"`
	Metadata    model.JSON `json:"metadata,omitempty"`
}

type brandAssetResponse struct {
	ID          string     `json:"id"`
	OrgID       string     `json:"org_id"`
	BrandID     string     `json:"brand_id"`
	Kind        string     `json:"kind"`
	Role        string     `json:"role"`
	Name        string     `json:"name"`
	Key         string     `json:"key"`
	PublicURL   string     `json:"public_url"`
	ContentType string     `json:"content_type"`
	Bytes       int64      `json:"bytes"`
	Width       *int       `json:"width,omitempty"`
	Height      *int       `json:"height,omitempty"`
	Metadata    model.JSON `json:"metadata"`
	CreatedBy   *string    `json:"created_by,omitempty"`
	CreatedAt   string     `json:"created_at"`
	UpdatedAt   string     `json:"updated_at"`
}

type brandAssetMutationResponse struct {
	Asset brandAssetResponse `json:"asset"`
}

// @Summary List brand assets
// @Description Returns assets attached to one brand.
// @Tags brands
// @Produce json
// @Param id path string true "Brand ID"
// @Success 200 {object} paginatedResponse[brandAssetResponse]
// @Security BearerAuth
// @Router /v1/orgs/current/brands/{id}/assets [get]
func (h *BrandHandler) ListAssets(w http.ResponseWriter, r *http.Request) {
	brand, ok := h.loadBrandForRequest(w, r)
	if !ok {
		return
	}
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	q := h.db.WithContext(r.Context()).Where("org_id = ? AND brand_id = ?", brand.OrgID, brand.ID)
	q = applyPagination(q, cursor, limit)
	var assets []model.BrandAsset
	if err := q.Find(&assets).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list brand assets"})
		return
	}
	hasMore := len(assets) > limit
	if hasMore {
		assets = assets[:limit]
	}
	out := make([]brandAssetResponse, len(assets))
	for i, asset := range assets {
		out[i] = brandAssetToResponse(asset)
	}
	resp := paginatedResponse[brandAssetResponse]{Data: out, HasMore: hasMore}
	if hasMore {
		last := assets[len(assets)-1]
		next := encodeCursor(last.CreatedAt, last.ID)
		resp.NextCursor = &next
	}
	writeJSON(w, http.StatusOK, resp)
}

// @Summary Create brand asset
// @Description Stores metadata for an uploaded brand asset. Admin-only.
// @Tags brands
// @Accept json
// @Produce json
// @Param id path string true "Brand ID"
// @Param body body createBrandAssetRequest true "Brand asset metadata"
// @Success 201 {object} brandAssetMutationResponse
// @Security BearerAuth
// @Router /v1/orgs/current/brands/{id}/assets [post]
func (h *BrandHandler) CreateAsset(w http.ResponseWriter, r *http.Request) {
	brand, ok := h.loadBrandForRequest(w, r)
	if !ok {
		return
	}
	var req createBrandAssetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	asset, err := brandAssetFromRequest(r, brand, req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	if err := h.db.WithContext(r.Context()).Create(&asset).Error; err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "brand asset already exists"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to create brand asset"})
		return
	}
	writeJSON(w, http.StatusCreated, brandAssetMutationResponse{Asset: brandAssetToResponse(asset)})
}

// @Summary Delete brand asset
// @Description Deletes a brand asset metadata row if no logo references it. Admin-only.
// @Tags brands
// @Produce json
// @Param id path string true "Brand ID"
// @Param assetID path string true "Brand asset ID"
// @Success 200 {object} brandAssetMutationResponse
// @Security BearerAuth
// @Router /v1/orgs/current/brands/{id}/assets/{assetID} [delete]
func (h *BrandHandler) DeleteAsset(w http.ResponseWriter, r *http.Request) {
	brand, ok := h.loadBrandForRequest(w, r)
	if !ok {
		return
	}
	asset, ok := h.loadBrandAssetForRequest(w, r, brand)
	if !ok {
		return
	}
	if brandLogosReferenceAsset(brand, asset.ID) {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "remove logo references before deleting this asset"})
		return
	}
	if err := h.db.WithContext(r.Context()).Delete(&asset).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to delete brand asset"})
		return
	}
	writeJSON(w, http.StatusOK, brandAssetMutationResponse{Asset: brandAssetToResponse(asset)})
}

func brandAssetFromRequest(r *http.Request, brand model.Brand, req createBrandAssetRequest) (model.BrandAsset, error) {
	kind := strings.ToLower(strings.TrimSpace(req.Kind))
	if !validBrandAssetKinds[kind] {
		return model.BrandAsset{}, fmt.Errorf("kind must be logo, mark, icon, image, font, document, or other")
	}
	if strings.TrimSpace(req.Name) == "" {
		return model.BrandAsset{}, fmt.Errorf("name is required")
	}
	if strings.TrimSpace(req.Key) == "" {
		return model.BrandAsset{}, fmt.Errorf("key is required")
	}
	if err := validateBrandAssetURL(req.PublicURL); err != nil {
		return model.BrandAsset{}, err
	}
	if strings.TrimSpace(req.ContentType) == "" {
		return model.BrandAsset{}, fmt.Errorf("content_type is required")
	}
	if req.Bytes <= 0 {
		return model.BrandAsset{}, fmt.Errorf("bytes must be positive")
	}
	if req.Width != nil && *req.Width <= 0 {
		return model.BrandAsset{}, fmt.Errorf("width must be positive")
	}
	if req.Height != nil && *req.Height <= 0 {
		return model.BrandAsset{}, fmt.Errorf("height must be positive")
	}
	userID, _ := currentRequestUserID(r.Context())
	return model.BrandAsset{
		ID:          uuid.New(),
		OrgID:       brand.OrgID,
		BrandID:     brand.ID,
		Kind:        kind,
		Role:        strings.TrimSpace(req.Role),
		Name:        strings.TrimSpace(req.Name),
		Key:         strings.TrimSpace(req.Key),
		PublicURL:   strings.TrimSpace(req.PublicURL),
		ContentType: strings.TrimSpace(req.ContentType),
		Bytes:       req.Bytes,
		Width:       req.Width,
		Height:      req.Height,
		Metadata:    normalizeJSONPtr(&req.Metadata),
		CreatedBy:   userID,
	}, nil
}
