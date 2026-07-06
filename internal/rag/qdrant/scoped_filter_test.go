package qdrant

import "testing"

func TestBuildScopedFilter(t *testing.T) {
	// With no source IDs the scoped filter matches the plain org filter shape.
	base := BuildOrgFilter("org-1")
	none := BuildScopedFilter("org-1", nil)
	if len(none.Must) != len(base.Must) {
		t.Fatalf("empty sourceIDs: Must len = %d, want %d", len(none.Must), len(base.Must))
	}

	// With source IDs it adds exactly one extra Must clause matching rag_source_id.
	scoped := BuildScopedFilter("org-1", []string{"src-a", "src-b"})
	if len(scoped.Must) != len(base.Must)+1 {
		t.Fatalf("scoped Must len = %d, want %d", len(scoped.Must), len(base.Must)+1)
	}
	last := scoped.Must[len(scoped.Must)-1]
	field := last.GetField()
	if field == nil || field.Key != "rag_source_id" {
		t.Fatalf("last Must clause is not a rag_source_id match: %+v", last)
	}
	vals := field.GetMatch().GetKeywords()
	if vals == nil || len(vals.Strings) != 2 {
		t.Fatalf("rag_source_id match keywords = %+v, want 2 values", vals)
	}
}
