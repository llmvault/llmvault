package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/usehivy/hivy/internal/model"
)

const (
	defaultBrandSectionJSON = `{"version":1}`
	defaultBrandSourceJSON  = `{"version":1,"origin":"manual"}`
	maxBrandSectionBytes    = 256 * 1024
	maxBrandRawImportBytes  = 1024 * 1024
)

var (
	brandJSONIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,80}$`)
	brandHexColor      = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)
	brandColorFunc     = regexp.MustCompile(`(?i)^(rgb|rgba|hsl|hsla|oklch|oklab|lab|lch|color)\(.+\)$`)
)

func brandSectionFromRaw(name string, raw json.RawMessage, fallback string) (model.RawJSON, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		trimmed = []byte(fallback)
	}
	if len(trimmed) > maxBrandSectionBytes {
		return nil, fmt.Errorf("%s exceeds maximum size", name)
	}
	var obj map[string]any
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return nil, fmt.Errorf("%s must be valid JSON", name)
	}
	if obj == nil {
		return nil, fmt.Errorf("%s must be a JSON object", name)
	}
	if err := validateBrandVersion(name, obj); err != nil {
		return nil, err
	}
	if err := validateBrandSection(name, obj); err != nil {
		return nil, err
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("marshal %s: %w", name, err)
	}
	return model.RawJSON(out), nil
}

func brandRawImportFromRaw(raw json.RawMessage) (*model.RawJSON, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	if len(trimmed) > maxBrandRawImportBytes {
		return nil, fmt.Errorf("raw_import exceeds maximum size")
	}
	var value any
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return nil, fmt.Errorf("raw_import must be valid JSON")
	}
	copied := append([]byte(nil), trimmed...)
	out := model.RawJSON(copied)
	return &out, nil
}

func validateBrandVersion(name string, obj map[string]any) error {
	raw, ok := obj["version"]
	if !ok {
		obj["version"] = float64(1)
		return nil
	}
	version, ok := raw.(float64)
	if !ok || version < 1 || version != float64(int(version)) {
		return fmt.Errorf("%s.version must be a positive integer", name)
	}
	return nil
}

func validateBrandSection(name string, obj map[string]any) error {
	switch name {
	case "colors":
		return validateBrandColors(obj)
	case "typography":
		return validateBrandTypography(obj)
	case "logos":
		return validateBrandLogos(obj)
	case "voice":
		return validateBrandVoice(obj)
	case "source":
		return validateBrandSource(obj)
	default:
		return nil
	}
}

func validateBrandColors(obj map[string]any) error {
	if err := validateIDObjectArray(obj, "tokens", 200, false, validateColorToken); err != nil {
		return err
	}
	if err := validateIDObjectArray(obj, "palettes", 100, false, validateColorPalette); err != nil {
		return err
	}
	if semantic, ok, err := objectField(obj, "semantic"); err != nil {
		return err
	} else if ok {
		for key, value := range semantic {
			text, ok := value.(string)
			if !ok || !validColorRef(text) {
				return fmt.Errorf("colors.semantic.%s must be a color value or token id", key)
			}
		}
	}
	return objectFieldOnly(obj, "rules")
}

func validateColorToken(token map[string]any) error {
	value, ok := token["value"]
	if !ok {
		return nil
	}
	text, ok := value.(string)
	if !ok || !isBrandColorValue(text) {
		return fmt.Errorf("colors.tokens.value must be a valid color")
	}
	return validateStringArray(token, "roles", 30)
}

func validateColorPalette(palette map[string]any) error {
	if err := validateColorEntryArray(palette, "colors", 200); err != nil {
		return err
	}
	return validateColorEntryArray(palette, "stops", 200)
}

func validateColorEntryArray(obj map[string]any, field string, max int) error {
	values, ok, err := arrayField(obj, field, max)
	if err != nil || !ok {
		return err
	}
	for _, value := range values {
		entry, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s entries must be objects", field)
		}
		if raw, ok := entry["value"]; ok {
			text, ok := raw.(string)
			if !ok || !isBrandColorValue(text) {
				return fmt.Errorf("%s.value must be a valid color", field)
			}
		}
		if raw, ok := entry["token"]; ok {
			text, ok := raw.(string)
			if !ok || !validBrandJSONID(text) {
				return fmt.Errorf("%s.token must be a token id", field)
			}
		}
	}
	return nil
}

func validateBrandTypography(obj map[string]any) error {
	if err := validateIDObjectArray(obj, "font_families", 50, false, nil); err != nil {
		return err
	}
	scale, ok, err := objectField(obj, "type_scale")
	if err != nil || !ok {
		return err
	}
	for name, raw := range scale {
		item, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("typography.type_scale.%s must be an object", name)
		}
		if err := validateTypographyScaleItem(name, item); err != nil {
			return err
		}
	}
	return objectFieldOnly(obj, "rules")
}

func validateTypographyScaleItem(name string, item map[string]any) error {
	if raw, ok := item["font_family"]; ok {
		text, ok := raw.(string)
		if !ok || !validBrandJSONID(text) {
			return fmt.Errorf("typography.type_scale.%s.font_family must be a font id", name)
		}
	}
	if raw, ok := item["font_size"]; ok {
		size, ok := raw.(float64)
		if !ok || size <= 0 || size > 512 {
			return fmt.Errorf("typography.type_scale.%s.font_size must be between 1 and 512", name)
		}
	}
	return nil
}

func validateBrandLogos(obj map[string]any) error {
	if raw, ok := obj["primary_asset_id"]; ok {
		if _, err := parseLogoAssetID(raw); err != nil {
			return fmt.Errorf("logos.primary_asset_id must be a valid asset id")
		}
	}
	values, ok, err := arrayField(obj, "variants", 100)
	if err != nil || !ok {
		return err
	}
	for _, value := range values {
		variant, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("logos.variants entries must be objects")
		}
		if raw, ok := variant["id"]; ok {
			text, ok := raw.(string)
			if !ok || !validBrandJSONID(text) {
				return fmt.Errorf("logos.variants.id must be a stable id")
			}
		}
		if _, err := parseLogoAssetID(variant["asset_id"]); err != nil {
			return fmt.Errorf("logos.variants.asset_id must be a valid asset id")
		}
	}
	return objectFieldOnly(obj, "rules")
}

func validateBrandVoice(obj map[string]any) error {
	for _, field := range []string{"personality", "dos", "donts", "banned_terms"} {
		if err := validateStringArray(obj, field, 100); err != nil {
			return err
		}
	}
	if err := validateObjectArray(obj, "preferred_terms", 100); err != nil {
		return err
	}
	if err := validateObjectArray(obj, "examples", 50); err != nil {
		return err
	}
	return objectFieldOnly(obj, "tone", "writing_style")
}

func validateBrandSource(obj map[string]any) error {
	raw, ok := obj["origin"]
	if !ok {
		obj["origin"] = "manual"
		return nil
	}
	origin, ok := raw.(string)
	if !ok {
		return fmt.Errorf("source.origin must be a string")
	}
	switch strings.TrimSpace(origin) {
	case "manual", "import", "system":
		return nil
	default:
		return fmt.Errorf("source.origin must be manual, import, or system")
	}
}
