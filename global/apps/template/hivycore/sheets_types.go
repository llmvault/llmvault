// DO NOT EDIT — hivycore is the Hivy platform core. See doc.go.

package hivycore

import (
	"fmt"
	"time"
)

// APIError is a non-2xx response from the internal app API, surfacing the
// platform's error body and status. Handlers can pass it to
// (*App).WriteError to relay validation failures to the SPA.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("sheets api: status %d: %s", e.Status, e.Message)
}

// Filter is one node of the rows-query filter AST: either a group (exactly
// one of And / Or set) or a leaf condition (Field + Op + optional Value).
// Field is always a field ID (fld_…), never a display name.
type Filter struct {
	And   []Filter `json:"and,omitempty"`
	Or    []Filter `json:"or,omitempty"`
	Field string   `json:"field,omitempty"`
	Op    string   `json:"op,omitempty"`
	Value any      `json:"value,omitempty"`
}

// Sort orders results by "position", "created_at", or a field ID.
type Sort struct {
	Field string `json:"field"`
	Desc  bool   `json:"desc,omitempty"`
}

// Query is the rows-query input: filter AST, free-text search, sorts, keyset
// cursor, and limit (server-clamped to 100).
type Query struct {
	Filter           *Filter `json:"filter,omitempty"`
	Search           string  `json:"search,omitempty"`
	Sorts            []Sort  `json:"sorts,omitempty"`
	Cursor           string  `json:"cursor,omitempty"`
	Limit            int     `json:"limit,omitempty"`
	ResolveRelations bool    `json:"resolve_relations,omitempty"`
}

// SheetInfo is the bound sheet's metadata.
type SheetInfo struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Icon        string    `json:"icon"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Field is a typed column. Row data keys by Field.ID, never by name.
type Field struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Type     string         `json:"type"`
	Options  map[string]any `json:"options"`
	Position float64        `json:"position"`
}

// Page is one tab of the bound sheet, with its field definitions and row
// count.
type Page struct {
	ID             string  `json:"id"`
	SheetID        string  `json:"sheet_id"`
	Name           string  `json:"name"`
	Position       float64 `json:"position"`
	DisplayFieldID *string `json:"display_field_id,omitempty"`
	Fields         []Field `json:"fields"`
	RowCount       int64   `json:"row_count"`
}

// SheetStructure is the full read-only structure of the bound sheet.
type SheetStructure struct {
	Sheet SheetInfo `json:"sheet"`
	Pages []Page    `json:"pages"`
}

// Row is a stored row; Data maps field IDs to cell values.
type Row struct {
	ID        string         `json:"id"`
	Data      map[string]any `json:"data"`
	Position  float64        `json:"position"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// RelationRef is a hydrated relation chip returned when a query resolves
// relations.
type RelationRef struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// QueryResult is one page of query results plus the field legend. Follow
// NextCursor until it is empty to read everything.
type QueryResult struct {
	Rows       []Row                  `json:"rows"`
	Fields     []Field                `json:"fields"`
	NextCursor string                 `json:"next_cursor"`
	Relations  map[string]RelationRef `json:"relations"`
}

// RowInsert is one row to create; Data keys are field IDs.
type RowInsert struct {
	Data     map[string]any `json:"data"`
	Position *float64       `json:"position,omitempty"`
}

// RowUpdate partial-merges Data into the row: only the keys sent change.
type RowUpdate struct {
	ID       string         `json:"id"`
	Data     map[string]any `json:"data"`
	Position *float64       `json:"position,omitempty"`
}
