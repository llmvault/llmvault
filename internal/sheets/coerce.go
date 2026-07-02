package sheets

import (
	"encoding/json"
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// CoerceValue validates and normalizes an incoming cell value for a field.
// nil is always accepted and means "clear the cell". The returned value is
// JSON-storable (string, float64, bool, or []string).
func CoerceValue(field *model.SheetField, value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	switch field.Type {
	case FieldTypeText, FieldTypeLongText:
		return coerceString(field, value)
	case FieldTypeNumber:
		return coerceNumber(field, value)
	case FieldTypeCheckbox:
		return coerceCheckbox(field, value)
	case FieldTypeSelect:
		return coerceSelect(field, value)
	case FieldTypeMultiSelect:
		return coerceMultiSelect(field, value)
	case FieldTypeDate:
		return coerceDate(field, value)
	case FieldTypeURL:
		return coerceURL(field, value)
	case FieldTypeEmail:
		return coerceEmail(field, value)
	case FieldTypePhone:
		return coercePhone(field, value)
	case FieldTypeAttachment:
		return coerceAttachment(field, value)
	case FieldTypeRelation:
		return coerceRelation(field, value)
	default:
		return nil, &UnknownFieldTypeError{Type: field.Type}
	}
}

func coerceString(field *model.SheetField, value any) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case json.Number:
		return v.String(), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case int:
		return strconv.Itoa(v), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case bool:
		return strconv.FormatBool(v), nil
	default:
		return "", &ValueError{FieldID: field.ID, FieldType: field.Type, Message: fmt.Sprintf("expected a string, got %T", value)}
	}
}

func coerceNumber(field *model.SheetField, value any) (float64, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return 0, &ValueError{FieldID: field.ID, FieldType: field.Type, Message: fmt.Sprintf("%q is not a number", v.String())}
		}
		return f, nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0, &ValueError{FieldID: field.ID, FieldType: field.Type, Message: fmt.Sprintf("%q is not a number", v)}
		}
		return f, nil
	default:
		return 0, &ValueError{FieldID: field.ID, FieldType: field.Type, Message: fmt.Sprintf("expected a number, got %T", value)}
	}
}

func coerceCheckbox(field *model.SheetField, value any) (bool, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "yes", "1":
			return true, nil
		case "false", "no", "0", "":
			return false, nil
		}
	case float64:
		if v == 0 || v == 1 {
			return v == 1, nil
		}
	}
	return false, &ValueError{FieldID: field.ID, FieldType: field.Type, Message: "expected a boolean"}
}

func coerceSelect(field *model.SheetField, value any) (string, error) {
	s, err := coerceString(field, value)
	if err != nil {
		return "", err
	}
	if err := checkChoice(field, s); err != nil {
		return "", err
	}
	return s, nil
}

func coerceMultiSelect(field *model.SheetField, value any) ([]string, error) {
	items, err := coerceStringSlice(field, value)
	if err != nil {
		return nil, err
	}
	out := dedupeStrings(items)
	for _, item := range out {
		if err := checkChoice(field, item); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func checkChoice(field *model.SheetField, value string) error {
	choices := SelectChoices(field.Options)
	if len(choices) == 0 {
		return nil
	}
	for _, choice := range choices {
		if choice == value {
			return nil
		}
	}
	return &ValueError{FieldID: field.ID, FieldType: field.Type, Message: fmt.Sprintf("%q is not one of the configured choices", value)}
}

var dateLayouts = []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"}

func coerceDate(field *model.SheetField, value any) (string, error) {
	s, ok := value.(string)
	if !ok {
		return "", &ValueError{FieldID: field.ID, FieldType: field.Type, Message: fmt.Sprintf("expected a date string, got %T", value)}
	}
	s = strings.TrimSpace(s)
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC().Format(time.RFC3339), nil
		}
	}
	return "", &ValueError{FieldID: field.ID, FieldType: field.Type, Message: fmt.Sprintf("%q is not a recognized date (use RFC3339 or YYYY-MM-DD)", s)}
}

func coerceURL(field *model.SheetField, value any) (string, error) {
	s, err := coerceString(field, value)
	if err != nil {
		return "", err
	}
	s = strings.TrimSpace(s)
	parsed, parseErr := url.Parse(s)
	if parseErr != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", &ValueError{FieldID: field.ID, FieldType: field.Type, Message: fmt.Sprintf("%q is not a valid http(s) URL", s)}
	}
	return s, nil
}

func coerceEmail(field *model.SheetField, value any) (string, error) {
	s, err := coerceString(field, value)
	if err != nil {
		return "", err
	}
	s = strings.TrimSpace(s)
	addr, parseErr := mail.ParseAddress(s)
	if parseErr != nil || addr.Name != "" || addr.Address != s {
		return "", &ValueError{FieldID: field.ID, FieldType: field.Type, Message: fmt.Sprintf("%q is not a valid email address", s)}
	}
	return addr.Address, nil
}

var phonePattern = regexp.MustCompile(`^\+?[0-9 ().\-]{3,32}$`)

func coercePhone(field *model.SheetField, value any) (string, error) {
	s, err := coerceString(field, value)
	if err != nil {
		return "", err
	}
	s = strings.TrimSpace(s)
	if !phonePattern.MatchString(s) || countDigits(s) < 3 {
		return "", &ValueError{FieldID: field.ID, FieldType: field.Type, Message: fmt.Sprintf("%q is not a valid phone number", s)}
	}
	return s, nil
}

func coerceAttachment(field *model.SheetField, value any) ([]string, error) {
	keys, err := coerceStringSlice(field, value)
	if err != nil {
		return nil, err
	}
	keys = dedupeStrings(keys)
	if len(keys) > MaxAttachmentsPerCell {
		return nil, &LimitError{Limit: "attachments per cell", Max: MaxAttachmentsPerCell, Actual: len(keys)}
	}
	for _, key := range keys {
		if key == "" || strings.Contains(key, "..") || strings.HasPrefix(key, "/") {
			return nil, &AttachmentError{FieldID: field.ID, Key: key, Message: "malformed object key"}
		}
	}
	return keys, nil
}

func coerceRelation(field *model.SheetField, value any) ([]string, error) {
	raw, err := coerceStringSlice(field, value)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		id, parseErr := uuid.Parse(strings.TrimSpace(item))
		if parseErr != nil {
			return nil, &ValueError{FieldID: field.ID, FieldType: field.Type, Message: fmt.Sprintf("%q is not a valid row id", item)}
		}
		out = append(out, id.String())
	}
	out = dedupeStrings(out)
	if len(out) > MaxRelationLinksPerCell {
		return nil, &LimitError{Limit: "relation links per cell", Max: MaxRelationLinksPerCell, Actual: len(out)}
	}
	return out, nil
}

func coerceStringSlice(field *model.SheetField, value any) ([]string, error) {
	switch v := value.(type) {
	case string:
		return []string{v}, nil
	case []string:
		return append([]string(nil), v...), nil
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, &ValueError{FieldID: field.ID, FieldType: field.Type, Message: fmt.Sprintf("expected an array of strings, got element %T", item)}
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, &ValueError{FieldID: field.ID, FieldType: field.Type, Message: fmt.Sprintf("expected an array of strings, got %T", value)}
	}
}

func dedupeStrings(items []string) []string {
	seen := make(map[string]bool, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		if seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func countDigits(s string) int {
	n := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n++
		}
	}
	return n
}
