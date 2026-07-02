package sheets

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func testField(id, fieldType string, options model.JSON) *model.SheetField {
	if options == nil {
		options = model.JSON{}
	}
	return &model.SheetField{ID: id, Type: fieldType, Options: options}
}

func TestCoerceValueAllTypes(t *testing.T) {
	relTarget := uuid.New().String()
	rowA := "6f1e64ac-0f8f-4bb8-9b6c-0a4dfc6a1a01"
	cases := []struct {
		name    string
		field   *model.SheetField
		in      any
		want    any
		wantErr bool
	}{
		{"text string", testField("fld_txt0000001", FieldTypeText, nil), "hello", "hello", false},
		{"text from number", testField("fld_txt0000001", FieldTypeText, nil), float64(42), "42", false},
		{"text from bool", testField("fld_txt0000001", FieldTypeText, nil), true, "true", false},
		{"text rejects object", testField("fld_txt0000001", FieldTypeText, nil), map[string]any{}, nil, true},
		{"long_text", testField("fld_lng0000001", FieldTypeLongText, nil), "para", "para", false},
		{"number float", testField("fld_num0000001", FieldTypeNumber, nil), 12.5, 12.5, false},
		{"number from string", testField("fld_num0000001", FieldTypeNumber, nil), " 7 ", 7.0, false},
		{"number rejects text", testField("fld_num0000001", FieldTypeNumber, nil), "abc", nil, true},
		{"number rejects bool", testField("fld_num0000001", FieldTypeNumber, nil), true, nil, true},
		{"checkbox bool", testField("fld_chk0000001", FieldTypeCheckbox, nil), true, true, false},
		{"checkbox from string", testField("fld_chk0000001", FieldTypeCheckbox, nil), "yes", true, false},
		{"checkbox from number", testField("fld_chk0000001", FieldTypeCheckbox, nil), float64(0), false, false},
		{"checkbox rejects junk", testField("fld_chk0000001", FieldTypeCheckbox, nil), "maybe", nil, true},
		{"select in choices", testField("fld_sel0000001", FieldTypeSelect, model.JSON{"choices": []any{"new", "qualified"}}), "new", "new", false},
		{"select outside choices", testField("fld_sel0000001", FieldTypeSelect, model.JSON{"choices": []any{"new"}}), "spam", nil, true},
		{"select unrestricted", testField("fld_sel0000001", FieldTypeSelect, nil), "anything", "anything", false},
		{"multi_select dedupes", testField("fld_mse0000001", FieldTypeMultiSelect, model.JSON{"choices": []any{"a", "b"}}), []any{"a", "b", "a"}, []string{"a", "b"}, false},
		{"multi_select outside choices", testField("fld_mse0000001", FieldTypeMultiSelect, model.JSON{"choices": []any{"a"}}), []any{"z"}, nil, true},
		{"multi_select wraps single", testField("fld_mse0000001", FieldTypeMultiSelect, nil), "solo", []string{"solo"}, false},
		{"date rfc3339", testField("fld_dat0000001", FieldTypeDate, nil), "2026-07-02T10:00:00Z", "2026-07-02T10:00:00Z", false},
		{"date short form", testField("fld_dat0000001", FieldTypeDate, nil), "2026-07-02", "2026-07-02T00:00:00Z", false},
		{"date rejects junk", testField("fld_dat0000001", FieldTypeDate, nil), "not-a-date", nil, true},
		{"date rejects number", testField("fld_dat0000001", FieldTypeDate, nil), float64(20260702), nil, true},
		{"url https", testField("fld_url0000001", FieldTypeURL, nil), "https://example.com/x", "https://example.com/x", false},
		{"url rejects scheme", testField("fld_url0000001", FieldTypeURL, nil), "javascript:alert(1)", nil, true},
		{"url rejects bare word", testField("fld_url0000001", FieldTypeURL, nil), "example", nil, true},
		{"email ok", testField("fld_eml0000001", FieldTypeEmail, nil), "kim@example.com", "kim@example.com", false},
		{"email rejects display name", testField("fld_eml0000001", FieldTypeEmail, nil), "Kim <kim@example.com>", nil, true},
		{"email rejects junk", testField("fld_eml0000001", FieldTypeEmail, nil), "not-an-email", nil, true},
		{"phone ok", testField("fld_phn0000001", FieldTypePhone, nil), "+1 (555) 010-2030", "+1 (555) 010-2030", false},
		{"phone rejects letters", testField("fld_phn0000001", FieldTypePhone, nil), "call me", nil, true},
		{"attachment keys", testField("fld_att0000001", FieldTypeAttachment, nil), []any{"pub/o/x/sheets/attachments/a.png"}, []string{"pub/o/x/sheets/attachments/a.png"}, false},
		{"attachment rejects traversal", testField("fld_att0000001", FieldTypeAttachment, nil), []any{"pub/o/x/../y/a.png"}, nil, true},
		{"attachment rejects absolute", testField("fld_att0000001", FieldTypeAttachment, nil), []any{"/etc/passwd"}, nil, true},
		{"relation normalizes", testField("fld_rel0000001", FieldTypeRelation, model.JSON{"target_page_id": relTarget}), []any{strings.ToUpper(rowA)}, []string{rowA}, false},
		{"relation rejects non-uuid", testField("fld_rel0000001", FieldTypeRelation, model.JSON{"target_page_id": relTarget}), []any{"not-a-uuid"}, nil, true},
		{"nil clears any type", testField("fld_num0000001", FieldTypeNumber, nil), nil, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CoerceValue(tc.field, tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got value %#v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestCoerceValueLimits(t *testing.T) {
	att := testField("fld_att0000001", FieldTypeAttachment, nil)
	keys := make([]any, MaxAttachmentsPerCell+1)
	for i := range keys {
		keys[i] = uuid.NewString() + ".png"
	}
	var limitErr *LimitError
	if _, err := CoerceValue(att, keys); !errors.As(err, &limitErr) {
		t.Fatalf("attachment overflow error = %v, want LimitError", err)
	}

	rel := testField("fld_rel0000001", FieldTypeRelation, model.JSON{"target_page_id": uuid.New().String()})
	links := make([]any, MaxRelationLinksPerCell+1)
	for i := range links {
		links[i] = uuid.New().String()
	}
	if _, err := CoerceValue(rel, links); !errors.As(err, &limitErr) {
		t.Fatalf("relation overflow error = %v, want LimitError", err)
	}
}

func TestValidateFieldOptions(t *testing.T) {
	if err := ValidateFieldOptions("formula", model.JSON{}); err == nil {
		t.Fatalf("formula must be rejected in v1")
	}
	var optErr *OptionsError
	if err := ValidateFieldOptions(FieldTypeSelect, model.JSON{"choices": "nope"}); !errors.As(err, &optErr) {
		t.Fatalf("bad choices error = %v, want OptionsError", err)
	}
	if err := ValidateFieldOptions(FieldTypeSelect, model.JSON{"choices": []any{"a", "a"}}); !errors.As(err, &optErr) {
		t.Fatalf("duplicate choices error = %v, want OptionsError", err)
	}
	too := make([]any, MaxSelectOptionsPerField+1)
	for i := range too {
		too[i] = uuid.NewString()
	}
	var limitErr *LimitError
	if err := ValidateFieldOptions(FieldTypeSelect, model.JSON{"choices": too}); !errors.As(err, &limitErr) {
		t.Fatalf("choices overflow error = %v, want LimitError", err)
	}
	if err := ValidateFieldOptions(FieldTypeRelation, model.JSON{}); !errors.As(err, &optErr) {
		t.Fatalf("relation without target error = %v, want OptionsError", err)
	}
	if err := ValidateFieldOptions(FieldTypeRelation, model.JSON{"target_page_id": "nope"}); !errors.As(err, &optErr) {
		t.Fatalf("relation bad target error = %v, want OptionsError", err)
	}
	if err := ValidateFieldOptions(FieldTypeRelation, model.JSON{"target_page_id": uuid.New().String()}); err != nil {
		t.Fatalf("valid relation options rejected: %v", err)
	}
	if err := ValidateFieldOptions(FieldTypeNumber, model.JSON{"format": "currency"}); err != nil {
		t.Fatalf("unknown option keys must be allowed: %v", err)
	}
	if len(FieldTypes()) != 12 {
		t.Fatalf("expected 12 field types, got %d: %v", len(FieldTypes()), FieldTypes())
	}
}

func TestCheckCellAndRowSize(t *testing.T) {
	big := strings.Repeat("x", MaxCellValueBytes+1)
	var limitErr *LimitError
	if err := checkCellSize("fld_txt0000001", big); !errors.As(err, &limitErr) {
		t.Fatalf("oversized cell error = %v, want LimitError", err)
	}
	data := map[string]any{}
	for i := 0; i < 9; i++ {
		data[uuid.NewString()] = strings.Repeat("y", MaxCellValueBytes)
	}
	if err := checkRowSize(data); !errors.As(err, &limitErr) {
		t.Fatalf("oversized row error = %v, want LimitError", err)
	}
	if err := checkRowSize(map[string]any{"fld_txt0000001": "small"}); err != nil {
		t.Fatalf("small row rejected: %v", err)
	}
}
