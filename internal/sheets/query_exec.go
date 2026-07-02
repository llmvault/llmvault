package sheets

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// RowsResult is one page of query results. Relations maps linked row IDs to
// hydrated {id,label} chips when the query asked for resolve_relations.
type RowsResult struct {
	Rows       []model.SheetRow
	Fields     []model.SheetField
	NextCursor string
	Relations  map[string]RelationRef
}

// QueryRows compiles and runs a filter-AST query against one page. All
// predicates run in Postgres; the page is never loaded into app memory.
// maxLimit is the caller-surface clamp (QueryLimitMCP or QueryLimitREST).
func (s *Service) QueryRows(ctx context.Context, orgID, pageID uuid.UUID, q Query, maxLimit int) (*RowsResult, error) {
	page, err := s.loadPage(ctx, orgID, pageID)
	if err != nil {
		return nil, err
	}
	fields, err := s.loadPageFields(ctx, orgID, page.ID)
	if err != nil {
		return nil, err
	}
	compiled, err := Compile(page.ID, orgID, FieldMap(fields), q, maxLimit)
	if err != nil {
		return nil, err
	}

	var rows []model.SheetRow
	err = s.db.WithContext(ctx).
		Where(compiled.Where, compiled.Args...).
		Order(compiled.OrderBy).
		Limit(compiled.Limit + 1).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("query sheet rows: %w", err)
	}

	result := &RowsResult{Fields: fields}
	if len(rows) > compiled.Limit {
		rows = rows[:compiled.Limit]
		if compiled.KeyField != "" && len(rows) > 0 {
			cursor, err := EncodeCursor(compiled.KeyField, compiled.KeyDesc, &rows[len(rows)-1])
			if err != nil {
				return nil, err
			}
			result.NextCursor = cursor
		}
	}
	result.Rows = rows

	if q.ResolveRelations {
		refs, err := s.ResolveRelations(ctx, orgID, fields, rows)
		if err != nil {
			return nil, err
		}
		result.Relations = refs
	}
	return result, nil
}
