package handler

import (
	"fmt"
	"strings"
)

func validateObjectArray(obj map[string]any, field string, max int) error {
	values, ok, err := arrayField(obj, field, max)
	if err != nil || !ok {
		return err
	}
	for _, value := range values {
		if _, ok := value.(map[string]any); !ok {
			return fmt.Errorf("%s entries must be objects", field)
		}
	}
	return nil
}

func validateStringArray(obj map[string]any, field string, max int) error {
	values, ok, err := arrayField(obj, field, max)
	if err != nil || !ok {
		return err
	}
	for _, value := range values {
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return fmt.Errorf("%s entries must be non-empty strings", field)
		}
	}
	return nil
}

func arrayField(obj map[string]any, field string, max int) ([]any, bool, error) {
	raw, ok := obj[field]
	if !ok || raw == nil {
		return nil, false, nil
	}
	values, ok := raw.([]any)
	if !ok {
		return nil, false, fmt.Errorf("%s must be an array", field)
	}
	if len(values) > max {
		return nil, false, fmt.Errorf("%s has too many entries", field)
	}
	return values, true, nil
}

func objectField(obj map[string]any, field string) (map[string]any, bool, error) {
	raw, ok := obj[field]
	if !ok || raw == nil {
		return nil, false, nil
	}
	value, ok := raw.(map[string]any)
	if !ok {
		return nil, false, fmt.Errorf("%s must be an object", field)
	}
	return value, true, nil
}

func objectFieldOnly(obj map[string]any, fields ...string) error {
	for _, field := range fields {
		if _, _, err := objectField(obj, field); err != nil {
			return err
		}
	}
	return nil
}

func validBrandJSONID(value string) bool {
	return brandJSONIDPattern.MatchString(strings.TrimSpace(value))
}

func validColorRef(value string) bool {
	value = strings.TrimSpace(value)
	return isBrandColorValue(value) || validBrandJSONID(value)
}

func isBrandColorValue(value string) bool {
	value = strings.TrimSpace(value)
	switch value {
	case "transparent", "currentColor", "inherit":
		return true
	}
	if strings.HasPrefix(value, "var(") && strings.HasSuffix(value, ")") {
		return true
	}
	return brandHexColor.MatchString(value) || brandColorFunc.MatchString(value)
}
