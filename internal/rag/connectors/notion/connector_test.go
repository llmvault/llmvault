package notion

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

const emptyChildren = `{"results":[],"next_cursor":null}`

// SUBTREE mode must seed the frontier from the configured roots and never
// touch the search API.
func TestSubtreeScoping_SeedsFromRootsAndSkipsSearch(t *testing.T) {
	const rootID = "1429989f-e8ac-4eff-bc8f-57f56486db54"

	fp := newFakeProxy()
	fp.set(http.MethodGet, "/v1/pages/"+rootID, http.StatusOK, makePageJSON(t, rootID, "Root Page"))
	fp.set(http.MethodGet, "/v1/blocks/"+rootID+"/children", http.StatusOK, []byte(emptyChildren))

	cfg := NotionConfig{RootPageIDs: []string{rootID}, IncludeDatabases: true}
	c := NewConnector(cfg, fp)

	docs, fails := runConnector(t, c)
	if len(fails) != 0 {
		t.Fatalf("unexpected failures: %+v", fails)
	}
	if len(docs) != 1 {
		t.Fatalf("expected exactly 1 document from the single root, got %d", len(docs))
	}
	if docs[0].DocID != "notion_page_"+rootID {
		t.Fatalf("doc id = %q, want %q", docs[0].DocID, "notion_page_"+rootID)
	}
	if fp.calledWith("/v1/search") {
		t.Fatalf("SUBTREE mode must not call search; calls=%v", fp.calls)
	}
}

// SUBTREE mode walks child pages discovered in blocks, recursively.
func TestSubtreeScoping_FollowsChildPages(t *testing.T) {
	const rootID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	const childID = "child-page-block"

	rootChildren, _ := json.Marshal(searchResult{
		Results: []map[string]any{childPageBlock(childID, "Child")},
	})

	fp := newFakeProxy()
	fp.set(http.MethodGet, "/v1/pages/"+rootID, http.StatusOK, makePageJSON(t, rootID, "Root"))
	fp.set(http.MethodGet, "/v1/blocks/"+rootID+"/children", http.StatusOK, rootChildren)
	fp.set(http.MethodGet, "/v1/pages/"+childID, http.StatusOK, makePageJSON(t, childID, "Child"))
	fp.set(http.MethodGet, "/v1/blocks/"+childID+"/children", http.StatusOK, []byte(emptyChildren))

	c := NewConnector(NotionConfig{RootPageIDs: []string{rootID}, IncludeDatabases: true}, fp)
	docs, fails := runConnector(t, c)
	if len(fails) != 0 {
		t.Fatalf("unexpected failures: %+v", fails)
	}
	if len(docs) != 2 {
		t.Fatalf("expected root + child = 2 docs, got %d", len(docs))
	}
}

// An unshared root (404) is skipped without failing the run.
func TestSubtreeScoping_UnsharedRootSkipped(t *testing.T) {
	const rootID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	fp := newFakeProxy() // no fixtures => 404 for page and database
	c := NewConnector(NotionConfig{RootPageIDs: []string{rootID}, IncludeDatabases: true}, fp)

	docs, fails := runConnector(t, c)
	if len(docs) != 0 || len(fails) != 0 {
		t.Fatalf("unshared root should yield nothing, got docs=%d fails=%d", len(docs), len(fails))
	}
}

// With no configured roots the source ingests nothing (deny-by-default) and
// never calls the search API.
func TestNoScope_IngestsNothing(t *testing.T) {
	fp := newFakeProxy()
	c := NewConnector(NotionConfig{IncludeDatabases: true}, fp) // no roots
	docs, fails := runConnector(t, c)
	if len(fails) != 0 {
		t.Fatalf("unexpected failures: %+v", fails)
	}
	if len(docs) != 0 {
		t.Fatalf("expected 0 docs with no scope, got %d", len(docs))
	}
	if fp.calledWith("/v1/search") {
		t.Fatal("no-scope source must not enumerate via search")
	}
}

func TestRegistration_NotionKindResolves(t *testing.T) {
	factory, err := interfaces.Lookup(Kind)
	if err != nil {
		t.Fatalf("interfaces.Lookup(%q): %v", Kind, err)
	}
	src := &fixtureSource{cfg: json.RawMessage(`{"root_page_ids":["1429989fe8ac4effbc8f57f56486db54"]}`)}
	conn, err := factory(src, interfaces.BuildDeps{})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	if conn.Kind() != Kind {
		t.Fatalf("Kind() = %q, want %q", conn.Kind(), Kind)
	}
	if _, ok := conn.(*NotionConnector); !ok {
		t.Fatalf("factory returned %T, want *NotionConnector", conn)
	}
	// Confirms the connector satisfies the non-generic runnable surface
	// the ingest handler drives.
	var _ interface {
		Run(context.Context, interfaces.Source, json.RawMessage, time.Time, time.Time) (<-chan interfaces.DocumentOrFailure, error)
		FinalCheckpoint() (json.RawMessage, error)
	} = conn.(*NotionConnector)
}

// runConnector drives a full run through the checkpointed surface and
// drains its output.
func runConnector(t *testing.T, c *NotionConnector) ([]*interfaces.Document, []*interfaces.ConnectorFailure) {
	t.Helper()
	src := &fixtureSource{cfg: json.RawMessage(`{}`)}
	ch, err := c.LoadFromCheckpoint(context.Background(), src, c.DummyCheckpoint(), time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("LoadFromCheckpoint: %v", err)
	}
	return drainIngest(t, ch)
}
