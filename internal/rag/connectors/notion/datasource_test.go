package notion

import (
	"context"
	"testing"
)

// A database with multiple data sources is fully queried and every row
// page is collected as a child page for later indexing.
func TestReadPagesFromDatabase_CollectsPagesFromAllSources(t *testing.T) {
	fake := newFakeBlockClient()
	fake.dataSourcesByDB["db-1"] = []NotionDataSource{
		{ID: "ds-1", Name: "Source A"},
		{ID: "ds-2", Name: "Source B"},
	}
	fake.dsQueries["ds-1"] = []dataSourceQueryResult{
		{Results: []map[string]any{dbPageRow("page-from-ds1", nil)}},
	}
	fake.dsQueries["ds-2"] = []dataSourceQueryResult{
		{Results: []map[string]any{dbPageRow("page-from-ds2", nil)}},
	}

	w := newWalker(fake, true)
	out, err := w.readPagesFromDatabase(context.Background(), "db-1")
	if err != nil {
		t.Fatalf("readPagesFromDatabase: %v", err)
	}

	got := map[string]bool{}
	for _, id := range out.ChildPageIDs {
		got[id] = true
	}
	if !got["page-from-ds1"] || !got["page-from-ds2"] {
		t.Fatalf("expected pages from both sources, got %v", out.ChildPageIDs)
	}
	if fake.dataSourceToDatabaseMapMissing(w, "ds-1", "db-1") || fake.dataSourceToDatabaseMapMissing(w, "ds-2", "db-1") {
		t.Fatalf("data-source->database mapping not recorded: %v", w.dataSourceToDatabaseMap)
	}
}

// A data source paginates via next_cursor; every page is queried and the
// cursor is threaded through subsequent calls.
func TestReadPagesFromDatabase_PaginatesCursor(t *testing.T) {
	fake := newFakeBlockClient()
	fake.dataSourcesByDB["db-1"] = []NotionDataSource{{ID: "ds-1"}}
	fake.dsQueries["ds-1"] = []dataSourceQueryResult{
		{Results: []map[string]any{dbPageRow("page-1", nil)}, NextCursor: ptr("cursor-abc")},
		{Results: []map[string]any{dbPageRow("page-2", nil)}},
	}

	w := newWalker(fake, true)
	out, err := w.readPagesFromDatabase(context.Background(), "db-1")
	if err != nil {
		t.Fatalf("readPagesFromDatabase: %v", err)
	}

	if len(fake.dsCalls) != 2 {
		t.Fatalf("expected 2 data-source queries, got %v", fake.dsCalls)
	}
	if fake.dsCalls[0] != "ds-1|" || fake.dsCalls[1] != "ds-1|cursor-abc" {
		t.Fatalf("cursor not threaded across pages: %v", fake.dsCalls)
	}
	got := map[string]bool{}
	for _, id := range out.ChildPageIDs {
		got[id] = true
	}
	if !got["page-1"] || !got["page-2"] {
		t.Fatalf("expected page-1 and page-2, got %v", out.ChildPageIDs)
	}
}

// Row properties are rendered to text so database content is searchable
// even before the row pages are individually indexed.
func TestReadPagesFromDatabase_RendersRowProperties(t *testing.T) {
	fake := newFakeBlockClient()
	fake.dataSourcesByDB["db-1"] = []NotionDataSource{{ID: "ds-1"}}
	fake.dsQueries["ds-1"] = []dataSourceQueryResult{
		{Results: []map[string]any{
			dbPageRow("page-1", map[string]any{
				"Status": map[string]any{"type": "status", "status": map[string]any{"name": "In Progress"}},
			}),
		}},
	}

	w := newWalker(fake, true)
	out, err := w.readPagesFromDatabase(context.Background(), "db-1")
	if err != nil {
		t.Fatalf("readPagesFromDatabase: %v", err)
	}
	if len(out.Blocks) != 1 {
		t.Fatalf("expected 1 rendered row block, got %d", len(out.Blocks))
	}
	if got := out.Blocks[0].Text; got != "Status: In Progress\t" {
		t.Fatalf("row text = %q, want %q", got, "Status: In Progress\t")
	}
}

func (f *fakeBlockClient) dataSourceToDatabaseMapMissing(w *walker, ds, db string) bool {
	return w.dataSourceToDatabaseMap[ds] != db
}
