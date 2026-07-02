package sheets

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

var (
	testPageID   = uuid.MustParse("11111111-1111-4111-8111-111111111111")
	testOrgID    = uuid.MustParse("22222222-2222-4222-8222-222222222222")
	testTargetID = uuid.MustParse("33333333-3333-4333-8333-333333333333")
)

const whereBase = "page_id = ? AND org_id = ? AND archived_at IS NULL"

func testFieldDefs() map[string]*model.SheetField {
	defs := []*model.SheetField{
		testField("fld_txt0000001", FieldTypeText, nil),
		testField("fld_lng0000001", FieldTypeLongText, nil),
		testField("fld_num0000001", FieldTypeNumber, nil),
		testField("fld_chk0000001", FieldTypeCheckbox, nil),
		testField("fld_sel0000001", FieldTypeSelect, model.JSON{"choices": []any{"new", "qualified"}}),
		testField("fld_mse0000001", FieldTypeMultiSelect, model.JSON{"choices": []any{"a", "b"}}),
		testField("fld_dat0000001", FieldTypeDate, nil),
		testField("fld_url0000001", FieldTypeURL, nil),
		testField("fld_eml0000001", FieldTypeEmail, nil),
		testField("fld_phn0000001", FieldTypePhone, nil),
		testField("fld_att0000001", FieldTypeAttachment, nil),
		testField("fld_rel0000001", FieldTypeRelation, model.JSON{"target_page_id": testTargetID.String()}),
	}
	out := map[string]*model.SheetField{}
	for _, def := range defs {
		out[def.ID] = def
	}
	return out
}

// guardedNumeric / guardedTimestamp mirror the cast-guard shapes castExpr
// must emit: invalid (pre-re-type) cells become NULL instead of failing the
// ::numeric / ::timestamptz cast.
func guardedNumeric(id string) string {
	return "CASE WHEN data->>'" + id + "' ~ '" + numericGuardPattern + "' THEN (data->>'" + id + "')::numeric END"
}

func guardedTimestamp(id string) string {
	return "CASE WHEN data->>'" + id + "' ~ '" + timestampGuardPattern + "' THEN (data->>'" + id + "')::timestamptz END"
}

func mustCompile(t *testing.T, q Query) *Compiled {
	t.Helper()
	compiled, err := Compile(testPageID, testOrgID, testFieldDefs(), q, QueryLimitMCP)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if compiled.Args[0] != testPageID || compiled.Args[1] != testOrgID {
		t.Fatalf("compiled args must start with page and org ids, got %#v", compiled.Args[:2])
	}
	return compiled
}

func TestCompileConditionsPerOp(t *testing.T) {
	linkID := "6f1e64ac-0f8f-4bb8-9b6c-0a4dfc6a1a01"
	cases := []struct {
		name     string
		filter   Filter
		wantCond string
		wantArgs []any
	}{
		{"text eq", Filter{Field: "fld_txt0000001", Op: "eq", Value: "acme"}, "data->>'fld_txt0000001' = ?", []any{"acme"}},
		{"text neq", Filter{Field: "fld_txt0000001", Op: "neq", Value: "acme"}, "data->>'fld_txt0000001' IS DISTINCT FROM ?", []any{"acme"}},
		{"text contains escapes wildcards", Filter{Field: "fld_txt0000001", Op: "contains", Value: `50%_x\`}, "data->>'fld_txt0000001' ILIKE ?", []any{`%50\%\_x\\%`}},
		{"text not_contains", Filter{Field: "fld_txt0000001", Op: "not_contains", Value: "spam"}, "(data->>'fld_txt0000001' IS NULL OR data->>'fld_txt0000001' NOT ILIKE ?)", []any{"%spam%"}},
		{"text starts_with", Filter{Field: "fld_txt0000001", Op: "starts_with", Value: "ac"}, "data->>'fld_txt0000001' ILIKE ?", []any{"ac%"}},
		{"text in", Filter{Field: "fld_txt0000001", Op: "in", Value: []any{"a", "b"}}, "data->>'fld_txt0000001' IN (?, ?)", []any{"a", "b"}},
		{"text is_empty", Filter{Field: "fld_txt0000001", Op: "is_empty"}, "(data->'fld_txt0000001' IS NULL OR data->'fld_txt0000001' = 'null'::jsonb OR data->>'fld_txt0000001' = '')", nil},
		{"text is_not_empty", Filter{Field: "fld_txt0000001", Op: "is_not_empty"}, "NOT (data->'fld_txt0000001' IS NULL OR data->'fld_txt0000001' = 'null'::jsonb OR data->>'fld_txt0000001' = '')", nil},
		{"number eq casts", Filter{Field: "fld_num0000001", Op: "eq", Value: 7}, guardedNumeric("fld_num0000001") + " = ?", []any{7.0}},
		{"number gt", Filter{Field: "fld_num0000001", Op: "gt", Value: 5.5}, guardedNumeric("fld_num0000001") + " > ?", []any{5.5}},
		{"number gte", Filter{Field: "fld_num0000001", Op: "gte", Value: "5"}, guardedNumeric("fld_num0000001") + " >= ?", []any{5.0}},
		{"number lt", Filter{Field: "fld_num0000001", Op: "lt", Value: 1}, guardedNumeric("fld_num0000001") + " < ?", []any{1.0}},
		{"number lte", Filter{Field: "fld_num0000001", Op: "lte", Value: 1}, guardedNumeric("fld_num0000001") + " <= ?", []any{1.0}},
		{"number in casts", Filter{Field: "fld_num0000001", Op: "in", Value: []any{1, "2"}}, guardedNumeric("fld_num0000001") + " IN (?, ?)", []any{1.0, 2.0}},
		{"checkbox eq coalesces", Filter{Field: "fld_chk0000001", Op: "eq", Value: true}, "COALESCE((data->>'fld_chk0000001')::boolean, false) = ?", []any{true}},
		{"checkbox neq", Filter{Field: "fld_chk0000001", Op: "neq", Value: false}, "COALESCE((data->>'fld_chk0000001')::boolean, false) IS DISTINCT FROM ?", []any{false}},
		{"select eq checks choices", Filter{Field: "fld_sel0000001", Op: "eq", Value: "new"}, "data->>'fld_sel0000001' = ?", []any{"new"}},
		{"select in", Filter{Field: "fld_sel0000001", Op: "in", Value: []any{"new", "qualified"}}, "data->>'fld_sel0000001' IN (?, ?)", []any{"new", "qualified"}},
		{"multi_select contains", Filter{Field: "fld_mse0000001", Op: "contains", Value: "a"}, "data->'fld_mse0000001' @> ?::jsonb", []any{`["a"]`}},
		{"multi_select not_contains", Filter{Field: "fld_mse0000001", Op: "not_contains", Value: "a"}, "NOT (COALESCE(data->'fld_mse0000001', '[]'::jsonb) @> ?::jsonb)", []any{`["a"]`}},
		{"multi_select is_empty array form", Filter{Field: "fld_mse0000001", Op: "is_empty"}, "(data->'fld_mse0000001' IS NULL OR data->'fld_mse0000001' = 'null'::jsonb OR data->'fld_mse0000001' = '[]'::jsonb)", nil},
		{"date gte casts both sides", Filter{Field: "fld_dat0000001", Op: "gte", Value: "2026-01-01"}, guardedTimestamp("fld_dat0000001") + " >= ?::timestamptz", []any{"2026-01-01T00:00:00Z"}},
		{"date eq", Filter{Field: "fld_dat0000001", Op: "eq", Value: "2026-07-02T10:00:00Z"}, guardedTimestamp("fld_dat0000001") + " = ?::timestamptz", []any{"2026-07-02T10:00:00Z"}},
		{"url starts_with", Filter{Field: "fld_url0000001", Op: "starts_with", Value: "https://a"}, "data->>'fld_url0000001' ILIKE ?", []any{"https://a%"}},
		{"email eq", Filter{Field: "fld_eml0000001", Op: "eq", Value: "kim@example.com"}, "data->>'fld_eml0000001' = ?", []any{"kim@example.com"}},
		{"phone contains", Filter{Field: "fld_phn0000001", Op: "contains", Value: "555"}, "data->>'fld_phn0000001' ILIKE ?", []any{"%555%"}},
		{"attachment is_not_empty", Filter{Field: "fld_att0000001", Op: "is_not_empty"}, "NOT (data->'fld_att0000001' IS NULL OR data->'fld_att0000001' = 'null'::jsonb OR data->'fld_att0000001' = '[]'::jsonb)", nil},
		{"relation contains uuid", Filter{Field: "fld_rel0000001", Op: "contains", Value: linkID}, "data->'fld_rel0000001' @> ?::jsonb", []any{`["` + linkID + `"]`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			compiled := mustCompile(t, Query{Filter: &tc.filter})
			want := whereBase + " AND (" + tc.wantCond + ")"
			if compiled.Where != want {
				t.Fatalf("where:\n got %s\nwant %s", compiled.Where, want)
			}
			gotArgs := compiled.Args[2:]
			if len(tc.wantArgs) == 0 {
				if len(gotArgs) != 0 {
					t.Fatalf("expected no args, got %#v", gotArgs)
				}
				return
			}
			if !reflect.DeepEqual(gotArgs, tc.wantArgs) {
				t.Fatalf("args:\n got %#v\nwant %#v", gotArgs, tc.wantArgs)
			}
		})
	}
}

func TestCompileNestingAndSearch(t *testing.T) {
	q := Query{
		Filter: &Filter{And: []Filter{
			{Field: "fld_sel0000001", Op: "eq", Value: "new"},
			{Or: []Filter{
				{Field: "fld_num0000001", Op: "gt", Value: 10},
				{Field: "fld_chk0000001", Op: "eq", Value: true},
			}},
		}},
		Search: `ac_me%`,
	}
	compiled := mustCompile(t, q)
	want := whereBase + " AND ((data->>'fld_sel0000001' = ? AND (" + guardedNumeric("fld_num0000001") + " > ? OR COALESCE((data->>'fld_chk0000001')::boolean, false) = ?))) AND data::text ILIKE ?"
	if compiled.Where != want {
		t.Fatalf("where:\n got %s\nwant %s", compiled.Where, want)
	}
	wantArgs := []any{"new", 10.0, true, `%ac\_me\%%`}
	if !reflect.DeepEqual(compiled.Args[2:], wantArgs) {
		t.Fatalf("args:\n got %#v\nwant %#v", compiled.Args[2:], wantArgs)
	}
}

func TestCompileRejectsHostileInput(t *testing.T) {
	defs := testFieldDefs()
	compile := func(f Filter) error {
		_, err := Compile(testPageID, testOrgID, defs, Query{Filter: &f}, QueryLimitMCP)
		return err
	}
	var unknownField *UnknownFieldError
	if err := compile(Filter{Field: "fld_zzz9999999", Op: "eq", Value: "x"}); !errors.As(err, &unknownField) {
		t.Fatalf("unknown field error = %v", err)
	}
	if err := compile(Filter{Field: `data->>'x'; DROP TABLE sheet_rows;--`, Op: "eq", Value: "x"}); !errors.As(err, &unknownField) {
		t.Fatalf("injected field id error = %v", err)
	}
	var unsupported *UnsupportedOpError
	for _, bad := range []Filter{
		{Field: "fld_num0000001", Op: "contains", Value: "1"},
		{Field: "fld_mse0000001", Op: "eq", Value: "a"},
		{Field: "fld_txt0000001", Op: "gt", Value: "a"},
		{Field: "fld_chk0000001", Op: "in", Value: []any{true}},
		{Field: "fld_att0000001", Op: "eq", Value: "k"},
		{Field: "fld_rel0000001", Op: "neq", Value: "x"},
		{Field: "fld_txt0000001", Op: "eq; DROP TABLE sheet_rows;--", Value: "x"},
		{Field: "fld_txt0000001", Op: "ILIKE", Value: "x"},
	} {
		if err := compile(bad); !errors.As(err, &unsupported) {
			t.Fatalf("op %q on %s error = %v, want UnsupportedOpError", bad.Op, bad.Field, err)
		}
	}
	// A tampered field definition (bad ID shape) must never be spliced.
	tampered := map[string]*model.SheetField{
		"fld_evil": testField(`fld_x'||(SELECT 1)--`, FieldTypeText, nil),
	}
	if _, err := Compile(testPageID, testOrgID, tampered, Query{Filter: &Filter{Field: "fld_evil", Op: "eq", Value: "x"}}, 100); !errors.As(err, &unknownField) {
		t.Fatalf("tampered def error = %v, want UnknownFieldError", err)
	}
	// Values are always bind params, never spliced.
	hostile := `x' OR '1'='1`
	compiled := mustCompile(t, Query{Filter: &Filter{Field: "fld_txt0000001", Op: "eq", Value: hostile}})
	if strings.Contains(compiled.Where, hostile) {
		t.Fatalf("hostile value leaked into SQL: %s", compiled.Where)
	}
	if compiled.Args[2] != hostile {
		t.Fatalf("hostile value should be a bind arg, got %#v", compiled.Args[2])
	}
}

func TestCompileStructuralValidation(t *testing.T) {
	defs := testFieldDefs()
	compileErr := func(f *Filter) error {
		_, err := Compile(testPageID, testOrgID, defs, Query{Filter: f}, QueryLimitMCP)
		return err
	}
	if err := compileErr(&Filter{And: []Filter{{Field: "fld_txt0000001", Op: "eq", Value: "x"}}, Field: "fld_txt0000001", Op: "eq", Value: "x"}); err == nil {
		t.Fatalf("group+leaf node must be rejected")
	}
	if err := compileErr(&Filter{}); err == nil {
		t.Fatalf("empty node must be rejected")
	}
	deep := Filter{Field: "fld_txt0000001", Op: "eq", Value: "x"}
	for i := 0; i < maxFilterDepth+2; i++ {
		deep = Filter{And: []Filter{deep}}
	}
	if err := compileErr(&deep); err == nil {
		t.Fatalf("over-deep filter must be rejected")
	}
	wide := Filter{And: []Filter{}}
	for i := 0; i < maxFilterConditions+1; i++ {
		wide.And = append(wide.And, Filter{Field: "fld_txt0000001", Op: "eq", Value: "x"})
	}
	if err := compileErr(&wide); err == nil {
		t.Fatalf("over-wide filter must be rejected")
	}
	var valueErr *ValueError
	if err := compileErr(&Filter{Field: "fld_txt0000001", Op: "eq", Value: nil}); !errors.As(err, &valueErr) {
		t.Fatalf("eq nil error = %v, want ValueError", err)
	}
	if err := compileErr(&Filter{Field: "fld_dat0000001", Op: "gte", Value: "junk"}); !errors.As(err, &valueErr) {
		t.Fatalf("bad date filter value error = %v, want ValueError", err)
	}
	if err := compileErr(&Filter{Field: "fld_txt0000001", Op: "in", Value: "scalar"}); !errors.As(err, &valueErr) {
		t.Fatalf("scalar in error = %v, want ValueError", err)
	}
	var limitErr *LimitError
	tooMany := make([]any, maxInValues+1)
	for i := range tooMany {
		tooMany[i] = "v"
	}
	if err := compileErr(&Filter{Field: "fld_txt0000001", Op: "in", Value: tooMany}); !errors.As(err, &limitErr) {
		t.Fatalf("oversized in error = %v, want LimitError", err)
	}
}
