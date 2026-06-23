package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// @Summary Archive a brand
// @Description Archives an active brand for the current organization. Admin-only.
// @Tags brands
// @Produce json
// @Param id path string true "Brand ID"
// @Success 200 {object} brandMutationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/brands/{id} [delete]
func (h *BrandHandler) Archive(w http.ResponseWriter, r *http.Request) {
	brand, ok := h.loadBrandForRequest(w, r)
	if !ok {
		return
	}
	now := time.Now()
	if err := h.db.WithContext(r.Context()).Model(&model.Brand{}).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", brand.ID, brand.OrgID).
		Updates(map[string]any{"archived_at": &now, "is_default": false}).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to archive brand"})
		return
	}
	brand.ArchivedAt = &now
	brand.IsDefault = false
	writeJSON(w, http.StatusOK, brandMutationResponse{Brand: brandToResponse(brand)})
}

// @Summary Set default brand
// @Description Makes a brand the current organization's default brand. Admin-only.
// @Tags brands
// @Produce json
// @Param id path string true "Brand ID"
// @Success 200 {object} brandMutationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/brands/{id}/default [post]
func (h *BrandHandler) SetDefault(w http.ResponseWriter, r *http.Request) {
	brand, ok := h.loadBrandForRequest(w, r)
	if !ok {
		return
	}
	err := h.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Brand{}).
			Where("org_id = ? AND archived_at IS NULL", brand.OrgID).
			Update("is_default", false).Error; err != nil {
			return err
		}
		return tx.Model(&model.Brand{}).
			Where("id = ? AND org_id = ? AND archived_at IS NULL", brand.ID, brand.OrgID).
			Update("is_default", true).Error
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to set default brand"})
		return
	}
	h.writeReloadedBrand(w, r, brand.ID, brand.OrgID)
}

func (h *BrandHandler) loadBrandForRequest(w http.ResponseWriter, r *http.Request) (model.Brand, bool) {
	org, ok := orgForBrandRequest(w, r)
	if !ok {
		return model.Brand{}, false
	}
	brandID, ok := brandIDFromRequest(w, r)
	if !ok {
		return model.Brand{}, false
	}
	var brand model.Brand
	err := h.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", brandID, org.ID).
		First(&brand).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "brand not found"})
		return model.Brand{}, false
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load brand"})
		return model.Brand{}, false
	}
	return brand, true
}

func (h *BrandHandler) brandFromCreateRequest(r *http.Request, orgID uuid.UUID, req createBrandRequest) (model.Brand, error) {
	name := normalizeBrandName(req.Name)
	if name == "" {
		return model.Brand{}, fmt.Errorf("name is required")
	}
	brandID := uuid.New()
	logos, colors, typography, voice, source, rawImport, err := brandSectionsFromCreate(req)
	if err != nil {
		return model.Brand{}, err
	}
	if err := validateLogoAssetReferences(r.Context(), h.db, orgID, brandID, logos); err != nil {
		return model.Brand{}, err
	}
	userID, _ := currentRequestUserID(r.Context())
	return model.Brand{
		ID:          brandID,
		OrgID:       orgID,
		Name:        name,
		Slug:        normalizeBrandSlug(req.Slug, name),
		Description: strings.TrimSpace(req.Description),
		IsDefault:   req.IsDefault,
		Logos:       logos,
		Colors:      colors,
		Typography:  typography,
		Voice:       voice,
		Source:      source,
		RawImport:   rawImport,
		CreatedBy:   userID,
	}, nil
}

func brandSectionsFromCreate(req createBrandRequest) (model.RawJSON, model.RawJSON, model.RawJSON, model.RawJSON, model.RawJSON, *model.RawJSON, error) {
	logos, err := brandSectionFromRaw("logos", req.Logos, defaultBrandSectionJSON)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	colors, err := brandSectionFromRaw("colors", req.Colors, defaultBrandSectionJSON)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	typography, err := brandSectionFromRaw("typography", req.Typography, defaultBrandSectionJSON)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	voice, err := brandSectionFromRaw("voice", req.Voice, defaultBrandSectionJSON)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	source, err := brandSectionFromRaw("source", req.Source, defaultBrandSourceJSON)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	rawImport, err := brandRawImportFromRaw(req.RawImport)
	return logos, colors, typography, voice, source, rawImport, err
}

func orgForBrandRequest(w http.ResponseWriter, r *http.Request) (*model.Org, bool) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return nil, false
	}
	return org, true
}

func brandIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid brand id"})
		return uuid.Nil, false
	}
	return id, true
}

func decodeBrandPatchFields(w http.ResponseWriter, r *http.Request) (map[string]json.RawMessage, bool) {
	var fields map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&fields); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return nil, false
	}
	if len(fields) == 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "no fields to update"})
		return nil, false
	}
	return fields, true
}

func normalizeBrandName(raw string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
}

func normalizeBrandSlug(raw, name string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = name
	}
	return model.GenerateSlug(raw)
}
