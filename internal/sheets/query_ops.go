package sheets

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/usehivy/hivy/internal/model"
)

// compileCondition emits parameterized SQL for one leaf condition. def.ID has
// already been validated against the fld_ pattern; op has already been
// checked against the type's whitelist.
func compileCondition(def *model.SheetField, spec fieldTypeSpec, op string, value any) (string, []any, error) {
	textExpr := fmt.Sprintf("data->>'%s'", def.ID)
	jsonExpr := fmt.Sprintf("data->'%s'", def.ID)

	switch op {
	case OpIsEmpty:
		return emptyExpr(def, spec, false), nil, nil
	case OpIsNotEmpty:
		return emptyExpr(def, spec, true), nil, nil
	case OpEq, OpNeq:
		return compileEquality(def, spec, op, value, textExpr)
	case OpGt, OpGte, OpLt, OpLte:
		return compileRange(def, spec, op, value)
	case OpContains, OpNotContains:
		if spec.array {
			return compileContainment(def, op, value, jsonExpr)
		}
		return compileTextMatch(def, op, value, textExpr)
	case OpStartsWith:
		return compileTextMatch(def, op, value, textExpr)
	case OpIn:
		return compileIn(def, spec, value, textExpr)
	default:
		return "", nil, &UnsupportedOpError{FieldID: def.ID, FieldType: def.Type, Op: op}
	}
}

func emptyExpr(def *model.SheetField, spec fieldTypeSpec, negate bool) string {
	var expr string
	if spec.array {
		expr = fmt.Sprintf("(data->'%s' IS NULL OR data->'%s' = 'null'::jsonb OR data->'%s' = '[]'::jsonb)", def.ID, def.ID, def.ID)
	} else {
		expr = fmt.Sprintf("(data->'%s' IS NULL OR data->'%s' = 'null'::jsonb OR data->>'%s' = '')", def.ID, def.ID, def.ID)
	}
	if negate {
		return "NOT " + expr
	}
	return expr
}

func castExpr(def *model.SheetField, spec fieldTypeSpec, textExpr string) string {
	switch spec.cast {
	case castNumeric:
		return "(" + textExpr + ")::numeric"
	case castBoolean:
		return "COALESCE((" + textExpr + ")::boolean, false)"
	case castTimestamp:
		return "(" + textExpr + ")::timestamptz"
	default:
		return textExpr
	}
}

func compileEquality(def *model.SheetField, spec fieldTypeSpec, op string, value any, textExpr string) (string, []any, error) {
	coerced, err := CoerceValue(def, value)
	if err != nil {
		return "", nil, err
	}
	if coerced == nil {
		return "", nil, &ValueError{FieldID: def.ID, FieldType: def.Type, Message: fmt.Sprintf("op %q requires a value (use is_empty for empties)", op)}
	}
	lhs := castExpr(def, spec, textExpr)
	rhs := "?"
	if spec.cast == castTimestamp {
		rhs = "?::timestamptz"
	}
	if op == OpEq {
		return lhs + " = " + rhs, []any{coerced}, nil
	}
	return lhs + " IS DISTINCT FROM " + rhs, []any{coerced}, nil
}

func compileRange(def *model.SheetField, spec fieldTypeSpec, op string, value any) (string, []any, error) {
	coerced, err := CoerceValue(def, value)
	if err != nil {
		return "", nil, err
	}
	if coerced == nil {
		return "", nil, &ValueError{FieldID: def.ID, FieldType: def.Type, Message: fmt.Sprintf("op %q requires a value", op)}
	}
	cmp := map[string]string{OpGt: ">", OpGte: ">=", OpLt: "<", OpLte: "<="}[op]
	lhs := castExpr(def, spec, fmt.Sprintf("data->>'%s'", def.ID))
	rhs := "?"
	if spec.cast == castTimestamp {
		rhs = "?::timestamptz"
	}
	return lhs + " " + cmp + " " + rhs, []any{coerced}, nil
}

func compileTextMatch(def *model.SheetField, op string, value any, textExpr string) (string, []any, error) {
	s, err := coerceString(def, value)
	if err != nil {
		return "", nil, err
	}
	switch op {
	case OpContains:
		return textExpr + " ILIKE ?", []any{"%" + escapeLike(s) + "%"}, nil
	case OpNotContains:
		return "(" + textExpr + " IS NULL OR " + textExpr + " NOT ILIKE ?)", []any{"%" + escapeLike(s) + "%"}, nil
	case OpStartsWith:
		return textExpr + " ILIKE ?", []any{escapeLike(s) + "%"}, nil
	default:
		return "", nil, &UnsupportedOpError{FieldID: def.ID, FieldType: def.Type, Op: op}
	}
}

// compileContainment handles contains/not_contains on array-valued fields
// (multi_select, relation) via jsonb containment, which the GIN
// jsonb_path_ops index accelerates.
func compileContainment(def *model.SheetField, op string, value any, jsonExpr string) (string, []any, error) {
	element, err := containmentElement(def, value)
	if err != nil {
		return "", nil, err
	}
	payload, err := json.Marshal([]any{element})
	if err != nil {
		return "", nil, fmt.Errorf("sheets: marshal containment value: %w", err)
	}
	if op == OpContains {
		return jsonExpr + " @> ?::jsonb", []any{string(payload)}, nil
	}
	return "NOT (COALESCE(" + jsonExpr + ", '[]'::jsonb) @> ?::jsonb)", []any{string(payload)}, nil
}

func containmentElement(def *model.SheetField, value any) (any, error) {
	coerced, err := CoerceValue(def, value)
	if err != nil {
		return nil, err
	}
	switch v := coerced.(type) {
	case []string:
		if len(v) != 1 {
			return nil, &ValueError{FieldID: def.ID, FieldType: def.Type, Message: "contains expects a single value"}
		}
		return v[0], nil
	case string:
		return v, nil
	default:
		return nil, &ValueError{FieldID: def.ID, FieldType: def.Type, Message: "contains expects a single value"}
	}
}

func compileIn(def *model.SheetField, spec fieldTypeSpec, value any, textExpr string) (string, []any, error) {
	items, ok := anySlice(value)
	if !ok || len(items) == 0 {
		return "", nil, &ValueError{FieldID: def.ID, FieldType: def.Type, Message: "in expects a non-empty array of values"}
	}
	if len(items) > maxInValues {
		return "", nil, &LimitError{Limit: "in values", Max: maxInValues, Actual: len(items)}
	}
	args := make([]any, 0, len(items))
	placeholders := make([]string, 0, len(items))
	for _, item := range items {
		coerced, err := CoerceValue(def, item)
		if err != nil {
			return "", nil, err
		}
		if coerced == nil {
			return "", nil, &ValueError{FieldID: def.ID, FieldType: def.Type, Message: "in values cannot be null"}
		}
		args = append(args, coerced)
		placeholders = append(placeholders, "?")
	}
	lhs := castExpr(def, spec, textExpr)
	return lhs + " IN (" + strings.Join(placeholders, ", ") + ")", args, nil
}

func anySlice(value any) ([]any, bool) {
	switch v := value.(type) {
	case []any:
		return v, true
	case []string:
		out := make([]any, len(v))
		for i, s := range v {
			out[i] = s
		}
		return out, true
	default:
		return nil, false
	}
}

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// escapeLike neutralizes LIKE wildcards in user-supplied match strings.
func escapeLike(s string) string {
	return likeEscaper.Replace(s)
}

// compileOrder builds the ORDER BY clause. Returns the keyset key field
// ("position"/"created_at") when keyset pagination applies, or "" when field
// sorts make cursors unsupported.
func compileOrder(fields map[string]*model.SheetField, sorts []Sort) (orderBy, keyField string, keyDesc bool, err error) {
	if len(sorts) == 0 {
		return "position ASC, id ASC", SortPosition, false, nil
	}
	parts := make([]string, 0, len(sorts)+1)
	hasFieldSort := false
	for _, sort := range sorts {
		dir := "ASC"
		if sort.Desc {
			dir = "DESC"
		}
		switch sort.Field {
		case SortPosition:
			parts = append(parts, "position "+dir)
		case SortCreatedAt:
			parts = append(parts, "created_at "+dir)
		default:
			def, ok := fields[sort.Field]
			if !ok {
				return "", "", false, &UnknownFieldError{FieldID: sort.Field}
			}
			if !ValidFieldID(def.ID) {
				return "", "", false, &UnknownFieldError{FieldID: def.ID}
			}
			spec, known := fieldTypeSpecs[def.Type]
			if !known {
				return "", "", false, &UnknownFieldTypeError{Type: def.Type}
			}
			hasFieldSort = true
			parts = append(parts, castExpr(def, spec, fmt.Sprintf("data->>'%s'", def.ID))+" "+dir+" NULLS LAST")
		}
	}
	primary := sorts[0]
	idDir := "ASC"
	if primary.Desc {
		idDir = "DESC"
	}
	parts = append(parts, "id "+idDir)
	orderBy = strings.Join(parts, ", ")
	if hasFieldSort || len(sorts) > 1 {
		return orderBy, "", false, nil
	}
	return orderBy, primary.Field, primary.Desc, nil
}
