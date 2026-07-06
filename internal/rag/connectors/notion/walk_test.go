package notion

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

// Locks the exact block emit order and child-page mapping so the
// iterative traversal stays faithful: descendants emit before their
// parent's own text, siblings in order, and child pages are collected
// (not inlined) mapped to their containing page.
func TestReadBlocks_OrderAndChildPageMapping(t *testing.T) {
	fake := newFakeBlockClient()
	fake.children["page-1"] = []map[string]any{
		paraBlock("toggle-1", "Toggle Title", true),
		childPageBlock("sub-page-1", "Sub"),
		paraBlock("para-1", "Sibling", false),
	}
	fake.children["toggle-1"] = []map[string]any{
		paraBlock("inner-1", "Inner", false),
	}

	w := newWalker(fake, true)
	out, err := w.readBlocks(context.Background(), "page-1", "page-1")
	if err != nil {
		t.Fatalf("readBlocks: %v", err)
	}

	var texts []string
	for _, b := range out.Blocks {
		texts = append(texts, b.Text)
	}
	want := []string{"Inner", "Toggle Title", "Sibling"}
	if strings.Join(texts, "|") != strings.Join(want, "|") {
		t.Fatalf("block order = %v, want %v", texts, want)
	}

	if len(out.ChildPageIDs) != 1 || out.ChildPageIDs[0] != "sub-page-1" {
		t.Fatalf("child page ids = %v, want [sub-page-1]", out.ChildPageIDs)
	}
	if w.childPageParentMap["sub-page-1"] != "page-1" {
		t.Fatalf("child->parent map = %v, want sub-page-1 -> page-1", w.childPageParentMap)
	}
}

// A genuinely deep (finite) nesting chain must be walked iteratively
// without overflowing the goroutine stack, emitting every block once.
func TestReadBlocks_DeepNestingDoesNotOverflow(t *testing.T) {
	const deep = 2000
	fake := newFakeBlockClient()
	fake.fetchFn = func(blockID, _ string) (searchResult, bool, error) {
		level, _ := strconv.Atoi(blockID[strings.LastIndex(blockID, "-")+1:])
		next := level + 1
		if next > deep {
			return searchResult{}, true, nil
		}
		return searchResult{Results: []map[string]any{
			paraBlock("block-"+strconv.Itoa(next), "level "+strconv.Itoa(next), next < deep),
		}}, true, nil
	}

	w := newWalker(fake, true)
	out, err := w.readBlocks(context.Background(), "block-0", "block-0")
	if err != nil {
		t.Fatalf("readBlocks: %v", err)
	}

	if len(out.Blocks) != deep {
		t.Fatalf("expected %d blocks, got %d", deep, len(out.Blocks))
	}
	seen := map[string]struct{}{}
	for _, b := range out.Blocks {
		seen[b.Text] = struct{}{}
	}
	if len(seen) != deep {
		t.Fatalf("expected %d unique texts, got %d", deep, len(seen))
	}
	if _, ok := seen["level 1"]; !ok {
		t.Fatal("missing first level")
	}
	if _, ok := seen["level "+strconv.Itoa(deep)]; !ok {
		t.Fatal("missing deepest level")
	}
}

// A synced-block cycle (two blocks each nesting the other) must be
// detected via the open-ancestor set so traversal terminates.
func TestReadBlocks_CycleTerminates(t *testing.T) {
	fake := newFakeBlockClient()
	calls := 0
	cycle := map[string][]map[string]any{
		"block-0": {syncedBlock("block-1")},
		"block-1": {syncedBlock("block-0")},
	}
	fake.fetchFn = func(blockID, _ string) (searchResult, bool, error) {
		calls++
		if calls > 1000 {
			t.Fatal("cycle not broken: runaway fetches")
		}
		return searchResult{Results: cycle[blockID]}, true, nil
	}

	w := newWalker(fake, true)
	if _, err := w.readBlocks(context.Background(), "block-0", "block-0"); err != nil {
		t.Fatalf("readBlocks: %v", err)
	}
}

// table_row blocks store text in a cells matrix (not rich_text); each row
// is tab-joined and must be indexed.
func TestReadBlocks_TableRowCellsExtracted(t *testing.T) {
	fake := newFakeBlockClient()
	fake.children["page-1"] = []map[string]any{tableBlock("table-1")}
	fake.children["table-1"] = []map[string]any{
		tableRow("row-1", []string{"Name"}, []string{"Role"}, []string{"Team"}),
		tableRow("row-2", []string{"Arturo", "Martinez"}, []string{"Engineer"}, []string{"Platform"}),
	}

	w := newWalker(fake, true)
	out, err := w.readBlocks(context.Background(), "page-1", "page-1")
	if err != nil {
		t.Fatalf("readBlocks: %v", err)
	}

	var all string
	for _, b := range out.Blocks {
		all += b.Text + " "
	}
	for _, want := range []string{"Arturo Martinez", "Engineer", "Platform", "Name\tRole\tTeam"} {
		if !strings.Contains(all, want) {
			t.Fatalf("expected %q in table text, got %q", want, all)
		}
	}
}
