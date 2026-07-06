package notion

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

// ---- fixtureSource ----

type fixtureSource struct {
	cfg json.RawMessage
}

func (s *fixtureSource) SourceID() string        { return "src-fixture" }
func (s *fixtureSource) OrgID() string           { return "org-fixture" }
func (s *fixtureSource) SourceKind() string      { return Kind }
func (s *fixtureSource) Config() json.RawMessage { return s.cfg }

// ---- fakeProxy: records calls, serves fixtures keyed by "METHOD path" ----

type fakeProxy struct {
	mu     sync.Mutex
	bodies map[string][]byte
	status map[string]int
	calls  []string
	notFnd []byte
}

func newFakeProxy() *fakeProxy {
	return &fakeProxy{
		bodies: map[string][]byte{},
		status: map[string]int{},
		notFnd: []byte(`{"object":"error","status":404}`),
	}
}

func (f *fakeProxy) set(method, path string, status int, body []byte) {
	key := method + " " + path
	f.bodies[key] = body
	f.status[key] = status
}

func (f *fakeProxy) Do(
	_ context.Context, method, path, rawQuery string, _ []byte,
) (int, http.Header, []byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	call := method + " " + path
	if rawQuery != "" {
		call += "?" + rawQuery
	}
	f.calls = append(f.calls, call)

	key := method + " " + path
	if body, ok := f.bodies[key]; ok {
		return f.status[key], http.Header{}, body, nil
	}
	return http.StatusNotFound, http.Header{}, f.notFnd, nil
}

func (f *fakeProxy) calledWith(substr string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if strings.Contains(c, substr) {
			return true
		}
	}
	return false
}

// ---- fakeBlockClient: drives the walker with in-memory block trees ----

type fakeBlockClient struct {
	children        map[string][]map[string]any
	unshared        map[string]bool
	dataSourcesByDB map[string][]NotionDataSource
	dsQueries       map[string][]dataSourceQueryResult
	dsIdx           map[string]int
	dsCalls         []string
	fetchFn         func(blockID, cursor string) (searchResult, bool, error)
}

func newFakeBlockClient() *fakeBlockClient {
	return &fakeBlockClient{
		children:        map[string][]map[string]any{},
		unshared:        map[string]bool{},
		dataSourcesByDB: map[string][]NotionDataSource{},
		dsQueries:       map[string][]dataSourceQueryResult{},
		dsIdx:           map[string]int{},
	}
}

func (f *fakeBlockClient) blockChildren(_ context.Context, blockID, cursor string) (searchResult, bool, error) {
	if f.fetchFn != nil {
		return f.fetchFn(blockID, cursor)
	}
	if f.unshared[blockID] {
		return searchResult{}, false, nil
	}
	return searchResult{Results: f.children[blockID]}, true, nil
}

func (f *fakeBlockClient) dataSources(_ context.Context, databaseID string) ([]NotionDataSource, error) {
	return f.dataSourcesByDB[databaseID], nil
}

func (f *fakeBlockClient) dataSourceQuery(_ context.Context, dataSourceID, cursor string) (dataSourceQueryResult, error) {
	f.dsCalls = append(f.dsCalls, dataSourceID+"|"+cursor)
	pages := f.dsQueries[dataSourceID]
	i := f.dsIdx[dataSourceID]
	if i >= len(pages) {
		return dataSourceQueryResult{}, nil
	}
	f.dsIdx[dataSourceID] = i + 1
	return pages[i], nil
}

// ---- block builders (bool/number types are native, not JSON-decoded) ----

func paraBlock(id, content string, hasChildren bool) map[string]any {
	return map[string]any{
		"id":           id,
		"type":         "paragraph",
		"has_children": hasChildren,
		"paragraph": map[string]any{
			"rich_text": []any{
				map[string]any{"text": map[string]any{"content": content}},
			},
		},
	}
}

func childPageBlock(id, title string) map[string]any {
	return map[string]any{
		"id":           id,
		"type":         "child_page",
		"has_children": true,
		"child_page":   map[string]any{"title": title},
	}
}

func syncedBlock(id string) map[string]any {
	return map[string]any{
		"id":           id,
		"type":         "synced_block",
		"has_children": true,
		"synced_block": map[string]any{},
	}
}

func tableBlock(id string) map[string]any {
	return map[string]any{
		"id":           id,
		"type":         "table",
		"has_children": true,
		"table":        map[string]any{"table_width": float64(3)},
	}
}

func tableRow(id string, cells ...[]string) map[string]any {
	cellArr := make([]any, 0, len(cells))
	for _, cell := range cells {
		rts := make([]any, 0, len(cell))
		for _, w := range cell {
			rts = append(rts, map[string]any{
				"type":       "text",
				"plain_text": w,
				"text":       map[string]any{"content": w},
			})
		}
		cellArr = append(cellArr, rts)
	}
	return map[string]any{
		"id":           id,
		"type":         "table_row",
		"has_children": false,
		"table_row":    map[string]any{"cells": cellArr},
	}
}

func dbPageRow(id string, props map[string]any) map[string]any {
	return map[string]any{"id": id, "object": "page", "properties": props}
}

func ptr(s string) *string { return &s }

// ---- misc ----

func makePageJSON(t *testing.T, id, title string) []byte {
	t.Helper()
	page := map[string]any{
		"object":           "page",
		"id":               id,
		"last_edited_time": "2026-04-01T12:00:00.000Z",
		"url":              "https://notion.so/" + strings.ReplaceAll(id, "-", ""),
		"properties": map[string]any{
			"Name": map[string]any{
				"type":  "title",
				"title": []any{map[string]any{"plain_text": title}},
			},
		},
	}
	b, err := json.Marshal(page)
	if err != nil {
		t.Fatalf("marshal page: %v", err)
	}
	return b
}

func drainIngest(t *testing.T, ch <-chan interfaces.DocumentOrFailure) (
	docs []*interfaces.Document, fails []*interfaces.ConnectorFailure,
) {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if ev.Doc != nil {
				docs = append(docs, ev.Doc)
			} else if ev.Failure != nil {
				fails = append(fails, ev.Failure)
			}
		case <-timeout:
			t.Fatal("drainIngest: timeout waiting for connector output")
		}
	}
}
