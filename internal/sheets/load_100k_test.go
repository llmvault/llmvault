package sheets

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestSheets100kRowLoadSanity is the Phase 7 load sanity check: it seeds one
// page with 100k rows and measures the hot query shapes plus the two Phase 4
// watchpoints (per-chunk ActiveRowCount COUNT and nextPosition MAX). Ceilings
// are intentionally generous — the point is to log actuals and catch
// order-of-magnitude regressions, not to be a benchmark.
//
// Skipped unless SHEETS_LOAD_TEST=1 (needs a real Postgres and ~10s of
// seeding).
func TestSheets100kRowLoadSanity(t *testing.T) {
	if os.Getenv("SHEETS_LOAD_TEST") != "1" {
		t.Skip("set SHEETS_LOAD_TEST=1 to run the 100k-row load sanity test")
	}
	ctx := context.Background()
	db := connectSheetsTestDB(t)
	f := seedSheetsFixture(t, db)
	pageID := f.leads.Page.ID
	orgID := f.org.ID

	nameID := f.fieldByName(t, f.leads, "Name").ID
	scoreID := f.fieldByName(t, f.leads, "Score").ID
	statusID := f.fieldByName(t, f.leads, "Status").ID
	tagsID := f.fieldByName(t, f.leads, "Tags").ID
	mailID := f.fieldByName(t, f.leads, "Mail").ID

	const totalRows = 100_000
	seedStart := time.Now()
	// Batch raw seed: one INSERT..SELECT over generate_series. Every 100th row
	// carries tag "a" (containment target); one sentinel row carries the
	// search needle.
	err := db.Exec(`
		INSERT INTO sheet_rows (page_id, org_id, data, position, created_at, updated_at)
		SELECT ?, ?, jsonb_build_object(
			?::text, CASE WHEN i = 54321 THEN 'Zephyr Needle Corp' ELSE 'Lead ' || i END,
			?::text, (i % 1000),
			?::text, CASE WHEN i % 2 = 0 THEN 'new' ELSE 'qualified' END,
			?::text, CASE WHEN i % 100 = 0 THEN '["a"]'::jsonb ELSE '["b"]'::jsonb END,
			?::text, 'lead' || i || '@example.com'
		), i * 1024, now(), now()
		FROM generate_series(1, ?) AS i`,
		pageID, orgID, nameID, scoreID, statusID, tagsID, mailID, totalRows,
	).Error
	if err != nil {
		t.Fatalf("seed 100k rows: %v", err)
	}
	t.Logf("seeded %d rows in %s", totalRows, time.Since(seedStart).Round(time.Millisecond))

	count, err := f.svc.ActiveRowCount(ctx, orgID, pageID)
	if err != nil || count < totalRows {
		t.Fatalf("seeded row count=%d err=%v", count, err)
	}

	measure := func(name string, ceiling time.Duration, fn func() (int, error)) {
		t.Helper()
		start := time.Now()
		n, err := fn()
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		t.Logf("%-36s %9.1fms  (rows=%d, ceiling=%s)", name, float64(elapsed.Microseconds())/1000, n, ceiling)
		if elapsed > ceiling {
			t.Errorf("%s took %s, ceiling %s", name, elapsed, ceiling)
		}
	}

	query := func(q Query) (int, error) {
		result, err := f.svc.QueryRows(ctx, orgID, pageID, q, QueryLimitREST)
		if err != nil {
			return 0, err
		}
		return len(result.Rows), nil
	}

	// 1. Indexed-shape containment filter (multi_select contains → data->'fld' @> '["a"]').
	containment := Query{
		Filter: &Filter{Field: tagsID, Op: OpContains, Value: "a"},
		Limit:  QueryLimitREST,
	}
	measure("containment filter (tags @> a)", 2*time.Second, func() (int, error) {
		n, err := query(containment)
		if err == nil && n != QueryLimitREST {
			return n, fmt.Errorf("expected a full page of tag-a rows, got %d", n)
		}
		return n, err
	})
	logQueryPlan(t, f, pageID, orgID, containment, "containment")

	// 2. Numeric-cast filter + sort.
	numeric := Query{
		Filter: &Filter{Field: scoreID, Op: OpGte, Value: 990},
		Sorts:  []Sort{{Field: scoreID, Desc: true}},
		Limit:  QueryLimitREST,
	}
	measure("numeric filter+sort (score>=990)", 2*time.Second, func() (int, error) {
		return query(numeric)
	})
	logQueryPlan(t, f, pageID, orgID, numeric, "numeric-cast")

	// 3. Free-text search (data::text ILIKE).
	measure("search ILIKE (needle)", 2*time.Second, func() (int, error) {
		n, err := query(Query{Search: "zephyr needle", Limit: QueryLimitREST})
		if err == nil && n != 1 {
			return n, fmt.Errorf("search expected exactly the sentinel row, got %d", n)
		}
		return n, err
	})

	// 4. Keyset pagination: walk 5 pages of 200 on the default (position, id)
	// order.
	var pageIDs []uuid.UUID
	measure("keyset walk 5x200", 5*time.Second, func() (int, error) {
		cursor := ""
		total := 0
		for page := 0; page < 5; page++ {
			result, err := f.svc.QueryRows(ctx, orgID, pageID, Query{Limit: QueryLimitREST, Cursor: cursor}, QueryLimitREST)
			if err != nil {
				return total, err
			}
			total += len(result.Rows)
			if page == 0 {
				for i := range result.Rows {
					if len(pageIDs) < MaxRowsPerWriteMCP {
						pageIDs = append(pageIDs, result.Rows[i].ID)
					}
				}
			}
			if result.NextCursor == "" {
				return total, fmt.Errorf("keyset walk ended early at page %d", page)
			}
			cursor = result.NextCursor
		}
		if total != 5*QueryLimitREST {
			return total, fmt.Errorf("keyset walk fetched %d rows, want %d", total, 5*QueryLimitREST)
		}
		return total, nil
	})

	// 5. 100-row batch update through the full service path (coercion,
	// operation capture with before-image, prune).
	updates := make([]RowUpdate, 0, len(pageIDs))
	for _, id := range pageIDs {
		updates = append(updates, RowUpdate{ID: id, Data: map[string]any{scoreID: 5}})
	}
	measure("batch update 100 rows", 2*time.Second, func() (int, error) {
		rows, err := f.svc.UpdateRows(ctx, orgID, pageID, updates, MaxRowsPerWriteMCP, f.actor)
		return len(rows), err
	})

	// Watchpoint A: per-chunk ActiveRowCount COUNT at 100k (runs before every
	// import chunk).
	measure("watchpoint ActiveRowCount", 2*time.Second, func() (int, error) {
		n, err := f.svc.ActiveRowCount(ctx, orgID, pageID)
		return int(n), err
	})

	// Watchpoint B: nextPosition MAX(position) at 100k (runs on every insert
	// batch and import chunk).
	measure("watchpoint nextPosition MAX", 2*time.Second, func() (int, error) {
		_, err := f.svc.nextPosition(ctx, &modelSheetRowAlias{}, "page_id = ? AND org_id = ?", pageID, orgID)
		return 0, err
	})

	// 6. Operations prune behavior: hammer the op log past the retention
	// window and confirm it stays capped, with single-row writes staying fast
	// at 100k rows (each records + prunes inside its transaction).
	measure("30 single-row inserts (op prune)", 30*time.Second, func() (int, error) {
		for i := 0; i < 30; i++ {
			_, err := f.svc.InsertRows(ctx, orgID, pageID, []RowInsert{
				{Data: map[string]any{nameID: fmt.Sprintf("prune probe %d", i)}},
			}, MaxRowsPerWriteMCP, f.actor)
			if err != nil {
				return i, err
			}
		}
		return 30, nil
	})
	var opCount int64
	if err := db.Table("sheet_operations").
		Where("page_id = ? AND org_id = ?", pageID, orgID).
		Count(&opCount).Error; err != nil {
		t.Fatalf("count operations: %v", err)
	}
	t.Logf("operations retained after 30+ writes: %d (cap %d)", opCount, operationRetentionPerPage)
	if opCount > operationRetentionPerPage {
		t.Errorf("operations log not pruned: %d > %d", opCount, operationRetentionPerPage)
	}
}

// modelSheetRowAlias avoids importing model twice in the measure closure; it
// maps to the sheet_rows table for nextPosition.
type modelSheetRowAlias struct{}

func (modelSheetRowAlias) TableName() string { return "sheet_rows" }

// logQueryPlan logs EXPLAIN ANALYZE for a compiled query so the load run
// records whether indexes are used. Failures only log — plans are
// informational.
func logQueryPlan(t *testing.T, f *sheetsFixture, pageID, orgID uuid.UUID, q Query, label string) {
	t.Helper()
	compiled, err := Compile(pageID, orgID, FieldMap(f.leads.Fields), q, QueryLimitREST)
	if err != nil {
		t.Logf("explain %s: compile failed: %v", label, err)
		return
	}
	sql := fmt.Sprintf(
		"EXPLAIN (ANALYZE, BUFFERS) SELECT id FROM sheet_rows WHERE %s ORDER BY %s LIMIT %d",
		compiled.Where, compiled.OrderBy, compiled.Limit,
	)
	rows, err := f.db.Raw(sql, compiled.Args...).Rows()
	if err != nil {
		t.Logf("explain %s failed: %v", label, err)
		return
	}
	defer rows.Close()
	var lines []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err == nil {
			lines = append(lines, line)
		}
	}
	t.Logf("plan %s:\n%s", label, strings.Join(lines, "\n"))
}
