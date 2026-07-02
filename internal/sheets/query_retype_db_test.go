package sheets

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// TestQueryAfterFieldRetypeGuardsCasts is the regression for the field
// re-type 500: cells are lazily re-coerced by design, so a text column full
// of junk can become a number/date column. Range filters and sorts compile
// to ::numeric / ::timestamptz casts, which used to throw a Postgres error
// (→ 500) on the first invalid cell. The guarded casts must instead treat
// invalid cells as NULL: dropped from gt/gte/lt/lte filters, sorted NULLS
// LAST.
func TestQueryAfterFieldRetypeGuardsCasts(t *testing.T) {
	ctx := context.Background()
	db := connectSheetsTestDB(t)
	f := seedSheetsFixture(t, db)

	addTextField := func(t *testing.T, name string) *model.SheetField {
		t.Helper()
		field, err := f.svc.CreateField(ctx, f.org.ID, f.leads.Page.ID, FieldSpec{
			Name: name, Type: FieldTypeText,
		}, f.actor)
		if err != nil {
			t.Fatalf("create text field: %v", err)
		}
		return field
	}
	retype := func(t *testing.T, fieldID, newType string) {
		t.Helper()
		if _, err := f.svc.UpdateField(ctx, f.org.ID, fieldID, UpdateFieldRequest{Type: &newType}, f.actor); err != nil {
			t.Fatalf("retype field to %s: %v", newType, err)
		}
	}
	query := func(t *testing.T, q Query) []uuid.UUID {
		t.Helper()
		result, err := f.svc.QueryRows(ctx, f.org.ID, f.leads.Page.ID, q, QueryLimitMCP)
		if err != nil {
			t.Fatalf("query after retype must not error, got: %v", err)
		}
		ids := make([]uuid.UUID, 0, len(result.Rows))
		for _, row := range result.Rows {
			ids = append(ids, row.ID)
		}
		return ids
	}
	wantIDs := func(t *testing.T, got []uuid.UUID, want ...uuid.UUID) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("got %d rows %v, want %d rows %v", len(got), got, len(want), want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("row order mismatch at %d: got %v, want %v", i, got, want)
			}
		}
	}

	t.Run("text to number", func(t *testing.T) {
		field := addTextField(t, "Retype Number")
		rows := f.insertLeads(t,
			map[string]any{field.ID: "not a number"},
			map[string]any{field.ID: "42"},
			map[string]any{field.ID: "3.5"},
		)
		retype(t, field.ID, FieldTypeNumber)

		// Range filter: junk cell is NULL → dropped, valid cells compare.
		got := query(t, Query{Filter: &Filter{Field: field.ID, Op: OpGt, Value: 1}})
		wantIDs(t, got, rows[1].ID, rows[2].ID)
		got = query(t, Query{Filter: &Filter{Field: field.ID, Op: OpLte, Value: 4}})
		wantIDs(t, got, rows[2].ID)

		// Sort: numeric order for valid cells, junk cell NULLS LAST.
		got = query(t, Query{
			Filter: &Filter{Field: field.ID, Op: OpIsNotEmpty},
			Sorts:  []Sort{{Field: field.ID}},
		})
		wantIDs(t, got, rows[2].ID, rows[1].ID, rows[0].ID)
	})

	t.Run("text to date", func(t *testing.T) {
		field := addTextField(t, "Retype Date")
		rows := f.insertLeads(t,
			map[string]any{field.ID: "definitely not a date"},
			map[string]any{field.ID: "2026-01-02T00:00:00Z"},
			map[string]any{field.ID: "2025-06-01T12:30:00Z"},
		)
		retype(t, field.ID, FieldTypeDate)

		// Range filter: junk cell is NULL → dropped, normalized cells compare.
		got := query(t, Query{Filter: &Filter{Field: field.ID, Op: OpGte, Value: "2026-01-01"}})
		wantIDs(t, got, rows[1].ID)

		// Sort: chronological for valid cells, junk cell NULLS LAST.
		got = query(t, Query{
			Filter: &Filter{Field: field.ID, Op: OpIsNotEmpty},
			Sorts:  []Sort{{Field: field.ID}},
		})
		wantIDs(t, got, rows[2].ID, rows[1].ID, rows[0].ID)
	})
}
