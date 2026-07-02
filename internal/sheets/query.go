package sheets

import (
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// Filter is one node of the filter AST: either a group (exactly one of And /
// Or set) or a leaf condition (Field + Op + optional Value).
type Filter struct {
	And   []Filter `json:"and,omitempty"`
	Or    []Filter `json:"or,omitempty"`
	Field string   `json:"field,omitempty"`
	Op    string   `json:"op,omitempty"`
	Value any      `json:"value,omitempty"`
}

// Sort orders results by a special column ("position", "created_at") or a
// field ID. Field sorts disable cursor pagination.
type Sort struct {
	Field string `json:"field"`
	Desc  bool   `json:"desc,omitempty"`
}

// Special sort keys (everything else must be a known field ID).
const (
	SortPosition  = "position"
	SortCreatedAt = "created_at"
)

// Query is the full rows-query input: filter AST, free-text search, sorts,
// keyset cursor, and limit.
type Query struct {
	Filter           *Filter `json:"filter,omitempty"`
	Search           string  `json:"search,omitempty"`
	Sorts            []Sort  `json:"sorts,omitempty"`
	Cursor           string  `json:"cursor,omitempty"`
	Limit            int     `json:"limit,omitempty"`
	ResolveRelations bool    `json:"resolve_relations,omitempty"`
}

// Compiled is a single parameterized SQL fragment set ready to run against
// sheet_rows. Field IDs are the only interpolated identifiers and are
// re-validated against the canonical fld_ pattern before splicing; every
// value travels as a bind parameter.
type Compiled struct {
	Where   string
	Args    []any
	OrderBy string
	Limit   int
	// KeyField is "position" or "created_at" when keyset pagination applies;
	// empty when field sorts are in play (first-page only).
	KeyField string
	KeyDesc  bool
}

// FieldMap indexes active field definitions by ID for the compiler.
func FieldMap(fields []model.SheetField) map[string]*model.SheetField {
	out := make(map[string]*model.SheetField, len(fields))
	for i := range fields {
		out[fields[i].ID] = &fields[i]
	}
	return out
}

// Compile turns a Query into one parameterized WHERE/ORDER BY/LIMIT set.
// Every referenced field is resolved against the page's loaded definitions
// (unknown IDs error out), ops come from the per-type whitelist, and the
// query is always scoped to (page_id, org_id, archived_at IS NULL).
func Compile(pageID, orgID uuid.UUID, fields map[string]*model.SheetField, q Query, maxLimit int) (*Compiled, error) {
	var sb strings.Builder
	sb.WriteString("page_id = ? AND org_id = ? AND archived_at IS NULL")
	args := []any{pageID, orgID}

	if q.Filter != nil {
		count := 0
		cond, condArgs, err := compileFilter(fields, q.Filter, 0, &count)
		if err != nil {
			return nil, err
		}
		if cond != "" {
			sb.WriteString(" AND (")
			sb.WriteString(cond)
			sb.WriteString(")")
			args = append(args, condArgs...)
		}
	}

	if search := strings.TrimSpace(q.Search); search != "" {
		sb.WriteString(" AND data::text ILIKE ?")
		args = append(args, "%"+escapeLike(search)+"%")
	}

	orderBy, keyField, keyDesc, err := compileOrder(fields, q.Sorts)
	if err != nil {
		return nil, err
	}

	if q.Cursor != "" {
		if keyField == "" {
			return nil, ErrCursorWithFieldSorts
		}
		cond, cursorArgs, err := compileCursor(q.Cursor, keyField, keyDesc)
		if err != nil {
			return nil, err
		}
		sb.WriteString(" AND ")
		sb.WriteString(cond)
		args = append(args, cursorArgs...)
	}

	return &Compiled{
		Where:    sb.String(),
		Args:     args,
		OrderBy:  orderBy,
		Limit:    ClampLimit(q.Limit, maxLimit),
		KeyField: keyField,
		KeyDesc:  keyDesc,
	}, nil
}

func compileFilter(fields map[string]*model.SheetField, f *Filter, depth int, count *int) (string, []any, error) {
	if depth > maxFilterDepth {
		return "", nil, fmt.Errorf("sheets: filter nesting exceeds max depth %d", maxFilterDepth)
	}
	isGroup := len(f.And) > 0 || len(f.Or) > 0
	isLeaf := f.Field != "" || f.Op != ""
	switch {
	case isGroup && isLeaf:
		return "", nil, fmt.Errorf("sheets: filter node cannot combine and/or with a field condition")
	case len(f.And) > 0 && len(f.Or) > 0:
		return "", nil, fmt.Errorf("sheets: filter node cannot combine and with or")
	case len(f.And) > 0:
		return compileGroup(fields, f.And, " AND ", depth, count)
	case len(f.Or) > 0:
		return compileGroup(fields, f.Or, " OR ", depth, count)
	case !isLeaf:
		return "", nil, fmt.Errorf("sheets: empty filter node")
	}

	*count++
	if *count > maxFilterConditions {
		return "", nil, fmt.Errorf("sheets: filter exceeds max of %d conditions", maxFilterConditions)
	}
	def, ok := fields[f.Field]
	if !ok {
		return "", nil, &UnknownFieldError{FieldID: f.Field}
	}
	// Defense in depth: never splice an identifier that does not match the
	// canonical generated shape, even if a corrupted definition loaded.
	if !ValidFieldID(def.ID) {
		return "", nil, &UnknownFieldError{FieldID: def.ID}
	}
	spec, known := fieldTypeSpecs[def.Type]
	if !known {
		return "", nil, &UnknownFieldTypeError{Type: def.Type}
	}
	if !spec.ops[f.Op] {
		return "", nil, &UnsupportedOpError{FieldID: def.ID, FieldType: def.Type, Op: f.Op}
	}
	return compileCondition(def, spec, f.Op, f.Value)
}

func compileGroup(fields map[string]*model.SheetField, nodes []Filter, joiner string, depth int, count *int) (string, []any, error) {
	parts := make([]string, 0, len(nodes))
	var args []any
	for i := range nodes {
		cond, condArgs, err := compileFilter(fields, &nodes[i], depth+1, count)
		if err != nil {
			return "", nil, err
		}
		parts = append(parts, cond)
		args = append(args, condArgs...)
	}
	return "(" + strings.Join(parts, joiner) + ")", args, nil
}
