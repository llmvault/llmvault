package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func validateLogoAssetReferences(ctx context.Context, db *gorm.DB, orgID, brandID uuid.UUID, logos model.RawJSON) error {
	ids, err := collectLogoAssetIDs(logos)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	var count int64
	if err := db.WithContext(ctx).Model(&model.BrandAsset{}).
		Where("org_id = ? AND brand_id = ? AND id IN ?", orgID, brandID, ids).
		Count(&count).Error; err != nil {
		return fmt.Errorf("validate logo assets: %w", err)
	}
	if count != int64(len(ids)) {
		return fmt.Errorf("logos reference assets that do not belong to this brand")
	}
	return nil
}

func collectLogoAssetIDs(logos model.RawJSON) ([]uuid.UUID, error) {
	var obj map[string]any
	if len(logos) == 0 {
		return nil, nil
	}
	if err := json.Unmarshal(logos, &obj); err != nil {
		return nil, fmt.Errorf("logos must be valid JSON")
	}
	seen := map[uuid.UUID]bool{}
	add := func(raw any) error {
		id, err := parseLogoAssetID(raw)
		if err != nil {
			return err
		}
		seen[id] = true
		return nil
	}
	if raw, ok := obj["primary_asset_id"]; ok {
		if err := add(raw); err != nil {
			return nil, fmt.Errorf("logos.primary_asset_id must be a valid asset id")
		}
	}
	if values, ok, err := arrayField(obj, "variants", 100); err != nil {
		return nil, err
	} else if ok {
		for _, value := range values {
			variant, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("logos.variants entries must be objects")
			}
			if raw, ok := variant["asset_id"]; ok {
				if err := add(raw); err != nil {
					return nil, fmt.Errorf("logos.variants.asset_id must be a valid asset id")
				}
			}
		}
	}
	out := make([]uuid.UUID, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	return out, nil
}

func parseLogoAssetID(raw any) (uuid.UUID, error) {
	text, ok := raw.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return uuid.Nil, fmt.Errorf("asset id is required")
	}
	id, err := uuid.Parse(text)
	if err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func validateIDObjectArray(obj map[string]any, field string, max int, requireID bool, validate func(map[string]any) error) error {
	values, ok, err := arrayField(obj, field, max)
	if err != nil || !ok {
		return err
	}
	seen := map[string]bool{}
	for _, value := range values {
		item, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s entries must be objects", field)
		}
		raw, hasID := item["id"]
		if requireID || hasID {
			id, ok := raw.(string)
			if !ok || !validBrandJSONID(id) {
				return fmt.Errorf("%s.id must be a stable id", field)
			}
			if seen[id] {
				return fmt.Errorf("%s contains duplicate id %q", field, id)
			}
			seen[id] = true
		}
		if validate != nil {
			if err := validate(item); err != nil {
				return err
			}
		}
	}
	return nil
}
