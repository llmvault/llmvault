package interfaces

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestSectionEmpty_AllowedByContract(t *testing.T) {
	// The contract: Section{Text: ""} is a valid value; the chunker
	// (Phase 2E) is responsible for skipping empty sections. This test
	// pins the contract by exercising construction + round-trip without
	// any validation gate firing.
	s := Section{Text: ""}
	if s.Text != "" {
		t.Fatalf("Section.Text = %q, want empty", s.Text)
	}

	// A Document built from empty sections round-trips cleanly.
	doc := Document{
		DocID:      "d",
		SemanticID: "d",
		Sections:   []Section{s, {Text: "hello"}, {}},
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back Document
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(back.Sections) != 3 {
		t.Fatalf("round-trip lost sections: got %d, want 3", len(back.Sections))
	}
}

// ---------------------------------------------------------------------
// 8. Document JSON round-trip preserves every field
// ---------------------------------------------------------------------

func TestDocument_JSONRoundtrip(t *testing.T) {
	updated := time.Date(2026, 4, 22, 12, 30, 45, 0, time.UTC)
	orig := Document{
		DocID:      "gh-pr-42",
		SemanticID: "Fix the flaky test",
		Link:       "https://github.com/acme/foo/pull/42",
		Sections: []Section{
			{Text: "body", Link: "https://github.com/acme/foo/pull/42", Title: "PR body"},
			{Text: "comment one", Link: "https://github.com/acme/foo/pull/42#c1"},
		},
		DocUpdatedAt:    &updated,
		Metadata:        map[string]string{"state": "closed", "repo": "acme/foo"},
		PrimaryOwners:   []string{"alice@example.com"},
		SecondaryOwners: []string{"bob@example.com", "carol@example.com"},
	}

	raw, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var back Document
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// DocUpdatedAt: pointer comparison is wrong; compare Time values.
	if back.DocUpdatedAt == nil || !back.DocUpdatedAt.Equal(*orig.DocUpdatedAt) {
		t.Fatalf("DocUpdatedAt lost in round-trip: %v vs %v", back.DocUpdatedAt, orig.DocUpdatedAt)
	}
	// Replace pointer fields for the structural compare.
	back.DocUpdatedAt = orig.DocUpdatedAt

	if !reflect.DeepEqual(orig, back) {
		t.Fatalf("round-trip not equal:\norig=%+v\nback=%+v\nraw=%s", orig, back, string(raw))
	}
}

// ---------------------------------------------------------------------
// 9. Checkpoint marker interface composes with generics
// ---------------------------------------------------------------------
