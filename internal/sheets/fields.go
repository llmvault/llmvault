package sheets

import (
	"fmt"
	"sort"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// Field types (v1). `formula` is explicitly deferred to a later phase.
const (
	FieldTypeText        = "text"
	FieldTypeLongText    = "long_text"
	FieldTypeNumber      = "number"
	FieldTypeCheckbox    = "checkbox"
	FieldTypeSelect      = "select"
	FieldTypeMultiSelect = "multi_select"
	FieldTypeDate        = "date"
	FieldTypeURL         = "url"
	FieldTypeEmail       = "email"
	FieldTypePhone       = "phone"
	FieldTypeAttachment  = "attachment"
	FieldTypeRelation    = "relation"
)

// Filter ops (whitelist). Anything outside this set never reaches SQL.
const (
	OpEq          = "eq"
	OpNeq         = "neq"
	OpContains    = "contains"
	OpNotContains = "not_contains"
	OpStartsWith  = "starts_with"
	OpGt          = "gt"
	OpGte         = "gte"
	OpLt          = "lt"
	OpLte         = "lte"
	OpIsEmpty     = "is_empty"
	OpIsNotEmpty  = "is_not_empty"
	OpIn          = "in"
)

// castKind selects the SQL cast applied to `data->>'fld_x'` for a field type.
type castKind int

const (
	castText castKind = iota
	castNumeric
	castBoolean
	castTimestamp
)

type fieldTypeSpec struct {
	array bool
	cast  castKind
	ops   map[string]bool
}

func opSet(ops ...string) map[string]bool {
	set := make(map[string]bool, len(ops))
	for _, op := range ops {
		set[op] = true
	}
	return set
}

var (
	textLikeOps = []string{OpEq, OpNeq, OpContains, OpNotContains, OpStartsWith, OpIsEmpty, OpIsNotEmpty, OpIn}
	rangeOps    = []string{OpEq, OpNeq, OpGt, OpGte, OpLt, OpLte, OpIsEmpty, OpIsNotEmpty}
)

var fieldTypeSpecs = map[string]fieldTypeSpec{
	FieldTypeText:        {cast: castText, ops: opSet(textLikeOps...)},
	FieldTypeLongText:    {cast: castText, ops: opSet(textLikeOps...)},
	FieldTypeNumber:      {cast: castNumeric, ops: opSet(append(rangeOps, OpIn)...)},
	FieldTypeCheckbox:    {cast: castBoolean, ops: opSet(OpEq, OpNeq, OpIsEmpty, OpIsNotEmpty)},
	FieldTypeSelect:      {cast: castText, ops: opSet(OpEq, OpNeq, OpIsEmpty, OpIsNotEmpty, OpIn)},
	FieldTypeMultiSelect: {array: true, cast: castText, ops: opSet(OpContains, OpNotContains, OpIsEmpty, OpIsNotEmpty)},
	FieldTypeDate:        {cast: castTimestamp, ops: opSet(rangeOps...)},
	FieldTypeURL:         {cast: castText, ops: opSet(textLikeOps...)},
	FieldTypeEmail:       {cast: castText, ops: opSet(textLikeOps...)},
	FieldTypePhone:       {cast: castText, ops: opSet(textLikeOps...)},
	FieldTypeAttachment:  {array: true, cast: castText, ops: opSet(OpIsEmpty, OpIsNotEmpty)},
	FieldTypeRelation:    {array: true, cast: castText, ops: opSet(OpContains, OpIsEmpty, OpIsNotEmpty)},
}

// KnownFieldType reports whether t is one of the 12 v1 field types.
func KnownFieldType(t string) bool {
	_, ok := fieldTypeSpecs[t]
	return ok
}

// FieldTypes returns the sorted list of supported field types.
func FieldTypes() []string {
	out := make([]string, 0, len(fieldTypeSpecs))
	for t := range fieldTypeSpecs {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// ValidateFieldOptions validates the options payload for a field type:
// select/multi_select choices and the relation target page reference. Unknown
// option keys are allowed (display config such as number format lives here).
func ValidateFieldOptions(fieldType string, options model.JSON) error {
	if !KnownFieldType(fieldType) {
		return &UnknownFieldTypeError{Type: fieldType}
	}
	switch fieldType {
	case FieldTypeSelect, FieldTypeMultiSelect:
		return validateSelectChoices(fieldType, options)
	case FieldTypeRelation:
		if _, err := RelationTargetPageID(options); err != nil {
			return err
		}
	}
	return nil
}

func validateSelectChoices(fieldType string, options model.JSON) error {
	raw, ok := options["choices"]
	if !ok || raw == nil {
		return nil
	}
	list, ok := raw.([]any)
	if !ok {
		if typed, isTyped := raw.([]string); isTyped {
			list = make([]any, len(typed))
			for i, v := range typed {
				list[i] = v
			}
		} else {
			return &OptionsError{FieldType: fieldType, Message: "choices must be an array of strings"}
		}
	}
	if len(list) > MaxSelectOptionsPerField {
		return &LimitError{Limit: "select options per field", Max: MaxSelectOptionsPerField, Actual: len(list)}
	}
	seen := make(map[string]bool, len(list))
	for _, item := range list {
		choice, ok := item.(string)
		if !ok || choice == "" {
			return &OptionsError{FieldType: fieldType, Message: "choices must be non-empty strings"}
		}
		if seen[choice] {
			return &OptionsError{FieldType: fieldType, Message: fmt.Sprintf("duplicate choice %q", choice)}
		}
		seen[choice] = true
	}
	return nil
}

// SelectChoices returns the configured choices for a select/multi_select
// field, or nil when the field is unrestricted.
func SelectChoices(options model.JSON) []string {
	raw, ok := options["choices"]
	if !ok || raw == nil {
		return nil
	}
	switch list := raw.(type) {
	case []string:
		return list
	case []any:
		out := make([]string, 0, len(list))
		for _, item := range list {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// RelationTargetPageID extracts and validates options.target_page_id for a
// relation field.
func RelationTargetPageID(options model.JSON) (uuid.UUID, error) {
	raw, ok := options["target_page_id"]
	if !ok {
		return uuid.Nil, &OptionsError{FieldType: FieldTypeRelation, Message: "target_page_id is required"}
	}
	s, ok := raw.(string)
	if !ok {
		return uuid.Nil, &OptionsError{FieldType: FieldTypeRelation, Message: "target_page_id must be a UUID string"}
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, &OptionsError{FieldType: FieldTypeRelation, Message: "target_page_id must be a valid UUID"}
	}
	return id, nil
}
