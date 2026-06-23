package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func (h *BrandHandler) brandUpdatesFromPatch(r *http.Request, brand model.Brand, fields map[string]json.RawMessage) (map[string]any, bool, error) {
	updates := map[string]any{}
	if raw, ok := fields["name"]; ok {
		value, err := stringPatchValue(raw, "name")
		if err != nil {
			return nil, false, err
		}
		name := normalizeBrandName(value)
		if name == "" {
			return nil, false, fmt.Errorf("name cannot be empty")
		}
		updates["name"] = name
	}
	if raw, ok := fields["slug"]; ok {
		value, err := stringPatchValue(raw, "slug")
		if err != nil {
			return nil, false, err
		}
		if strings.TrimSpace(value) == "" {
			return nil, false, fmt.Errorf("slug cannot be empty")
		}
		slug := normalizeBrandSlug(value, "")
		updates["slug"] = slug
	}
	if raw, ok := fields["description"]; ok {
		value, err := stringPatchValue(raw, "description")
		if err != nil {
			return nil, false, err
		}
		updates["description"] = strings.TrimSpace(value)
	}
	setDefault, err := applyDefaultPatch(updates, fields)
	if err != nil {
		return nil, false, err
	}
	if err := h.applyJSONPatch(r, brand, fields, updates); err != nil {
		return nil, false, err
	}
	return updates, setDefault, nil
}

func (h *BrandHandler) applyJSONPatch(r *http.Request, brand model.Brand, fields map[string]json.RawMessage, updates map[string]any) error {
	specs := []struct {
		key      string
		fallback string
	}{
		{"logos", defaultBrandSectionJSON},
		{"colors", defaultBrandSectionJSON},
		{"typography", defaultBrandSectionJSON},
		{"voice", defaultBrandSectionJSON},
		{"source", defaultBrandSourceJSON},
	}
	for _, spec := range specs {
		raw, ok := fields[spec.key]
		if !ok {
			continue
		}
		value, err := brandSectionFromRaw(spec.key, raw, spec.fallback)
		if err != nil {
			return err
		}
		if spec.key == "logos" {
			if err := validateLogoAssetReferences(r.Context(), h.db, brand.OrgID, brand.ID, value); err != nil {
				return err
			}
		}
		updates[spec.key] = value
	}
	if raw, ok := fields["raw_import"]; ok {
		value, err := brandRawImportFromRaw(raw)
		if err != nil {
			return err
		}
		if value == nil {
			updates["raw_import"] = nil
		} else {
			updates["raw_import"] = value
		}
	}
	return nil
}

func applyDefaultPatch(updates map[string]any, fields map[string]json.RawMessage) (bool, error) {
	raw, ok := fields["is_default"]
	if !ok {
		return false, nil
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, fmt.Errorf("is_default must be a boolean")
	}
	updates["is_default"] = value
	return value, nil
}

func stringPatchValue(raw json.RawMessage, field string) (string, error) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("%s must be a string", field)
	}
	return value, nil
}

func (h *BrandHandler) writeReloadedBrand(w http.ResponseWriter, r *http.Request, brandID, orgID uuid.UUID) {
	var reloaded model.Brand
	if err := h.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ?", brandID, orgID).
		First(&reloaded).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to reload brand"})
		return
	}
	writeJSON(w, http.StatusOK, brandMutationResponse{Brand: brandToResponse(reloaded)})
}

func brandToResponse(brand model.Brand) brandResponse {
	return brandResponse{
		ID:          brand.ID.String(),
		OrgID:       brand.OrgID.String(),
		Name:        brand.Name,
		Slug:        brand.Slug,
		Description: brand.Description,
		IsDefault:   brand.IsDefault,
		Logos:       brand.Logos,
		Colors:      brand.Colors,
		Typography:  brand.Typography,
		Voice:       brand.Voice,
		Source:      brand.Source,
		RawImport:   brand.RawImport,
		ArchivedAt:  formatTimePtr(brand.ArchivedAt),
		CreatedBy:   formatUUIDPtr(brand.CreatedBy),
		CreatedAt:   brand.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   brand.UpdatedAt.Format(time.RFC3339),
	}
}
