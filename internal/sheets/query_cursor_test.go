package sheets

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestClampLimit(t *testing.T) {
	cases := []struct{ limit, max, want int }{
		{0, QueryLimitMCP, DefaultQueryLimit},
		{-5, QueryLimitMCP, DefaultQueryLimit},
		{20, QueryLimitMCP, 20},
		{500, QueryLimitMCP, QueryLimitMCP},
		{500, QueryLimitREST, QueryLimitREST},
		{5, 0, 5},
	}
	for _, tc := range cases {
		if got := ClampLimit(tc.limit, tc.max); got != tc.want {
			t.Fatalf("ClampLimit(%d, %d) = %d, want %d", tc.limit, tc.max, got, tc.want)
		}
	}
	compiled := mustCompile(t, Query{Limit: 10_000})
	if compiled.Limit != QueryLimitMCP {
		t.Fatalf("compiled limit = %d, want clamp to %d", compiled.Limit, QueryLimitMCP)
	}
}

func TestCompileOrdering(t *testing.T) {
	compiled := mustCompile(t, Query{})
	if compiled.OrderBy != "position ASC, id ASC" || compiled.KeyField != SortPosition || compiled.KeyDesc {
		t.Fatalf("default ordering = %q keyset (%q, %v)", compiled.OrderBy, compiled.KeyField, compiled.KeyDesc)
	}
	compiled = mustCompile(t, Query{Sorts: []Sort{{Field: SortCreatedAt, Desc: true}}})
	if compiled.OrderBy != "created_at DESC, id DESC" || compiled.KeyField != SortCreatedAt || !compiled.KeyDesc {
		t.Fatalf("created_at ordering = %q keyset (%q, %v)", compiled.OrderBy, compiled.KeyField, compiled.KeyDesc)
	}
	compiled = mustCompile(t, Query{Sorts: []Sort{{Field: "fld_num0000001", Desc: true}}})
	want := guardedNumeric("fld_num0000001") + " DESC NULLS LAST, id DESC"
	if compiled.OrderBy != want {
		t.Fatalf("field ordering = %q, want %q", compiled.OrderBy, want)
	}
	if compiled.KeyField != "" {
		t.Fatalf("field sorts must disable keyset pagination, got %q", compiled.KeyField)
	}
	compiled = mustCompile(t, Query{Sorts: []Sort{{Field: SortPosition}, {Field: SortCreatedAt}}})
	if compiled.KeyField != "" {
		t.Fatalf("multi-sorts must disable keyset pagination, got %q", compiled.KeyField)
	}
	var unknownField *UnknownFieldError
	if _, err := Compile(testPageID, testOrgID, testFieldDefs(), Query{Sorts: []Sort{{Field: "position; DROP TABLE sheet_rows"}}}, 100); !errors.As(err, &unknownField) {
		t.Fatalf("injected sort field error = %v, want UnknownFieldError", err)
	}
}

func TestCursorRoundTrip(t *testing.T) {
	row := &model.SheetRow{ID: uuid.New(), Position: 2048, CreatedAt: time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)}

	cursor, err := EncodeCursor(SortPosition, false, row)
	if err != nil {
		t.Fatalf("EncodeCursor: %v", err)
	}
	compiled := mustCompile(t, Query{Cursor: cursor})
	if !strings.HasSuffix(compiled.Where, " AND (position, id) > (?, ?)") {
		t.Fatalf("position cursor where = %q", compiled.Where)
	}
	args := compiled.Args[2:]
	if args[0] != 2048.0 || args[1] != row.ID {
		t.Fatalf("position cursor args = %#v", args)
	}

	cursor, err = EncodeCursor(SortPosition, true, row)
	if err != nil {
		t.Fatalf("EncodeCursor desc: %v", err)
	}
	compiled = mustCompile(t, Query{Sorts: []Sort{{Field: SortPosition, Desc: true}}, Cursor: cursor})
	if !strings.HasSuffix(compiled.Where, " AND (position, id) < (?, ?)") {
		t.Fatalf("desc cursor where = %q", compiled.Where)
	}

	cursor, err = EncodeCursor(SortCreatedAt, false, row)
	if err != nil {
		t.Fatalf("EncodeCursor created_at: %v", err)
	}
	compiled = mustCompile(t, Query{Sorts: []Sort{{Field: SortCreatedAt}}, Cursor: cursor})
	if !strings.HasSuffix(compiled.Where, " AND (created_at, id) > (?::timestamptz, ?)") {
		t.Fatalf("created_at cursor where = %q", compiled.Where)
	}
}

func TestCursorRejections(t *testing.T) {
	defs := testFieldDefs()
	row := &model.SheetRow{ID: uuid.New(), Position: 1024, CreatedAt: time.Now()}

	posCursor, err := EncodeCursor(SortPosition, false, row)
	if err != nil {
		t.Fatalf("EncodeCursor: %v", err)
	}
	// Cursor minted under position ordering replayed against created_at.
	if _, err := Compile(testPageID, testOrgID, defs, Query{Sorts: []Sort{{Field: SortCreatedAt}}, Cursor: posCursor}, 100); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("ordering-mismatch cursor error = %v, want ErrInvalidCursor", err)
	}
	// Direction mismatch.
	if _, err := Compile(testPageID, testOrgID, defs, Query{Sorts: []Sort{{Field: SortPosition, Desc: true}}, Cursor: posCursor}, 100); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("direction-mismatch cursor error = %v, want ErrInvalidCursor", err)
	}
	for _, garbage := range []string{"not-base64!!!", "eyJrIjoiZXZpbCJ9", "AAAA"} {
		if _, err := Compile(testPageID, testOrgID, defs, Query{Cursor: garbage}, 100); !errors.Is(err, ErrInvalidCursor) {
			t.Fatalf("garbage cursor %q error = %v, want ErrInvalidCursor", garbage, err)
		}
	}
	// Cursors cannot be combined with field sorts.
	if _, err := Compile(testPageID, testOrgID, defs, Query{Sorts: []Sort{{Field: "fld_num0000001"}}, Cursor: posCursor}, 100); !errors.Is(err, ErrCursorWithFieldSorts) {
		t.Fatalf("field-sort cursor error = %v, want ErrCursorWithFieldSorts", err)
	}
	if _, err := EncodeCursor("fld_num0000001", false, row); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("EncodeCursor with field key error = %v, want ErrInvalidCursor", err)
	}
}
