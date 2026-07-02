package sheets

import (
	"strings"
	"testing"
)

func TestNewFieldIDFormat(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id, err := NewFieldID()
		if err != nil {
			t.Fatalf("NewFieldID: %v", err)
		}
		if !ValidFieldID(id) {
			t.Fatalf("generated id %q does not match the canonical pattern", id)
		}
		if !strings.HasPrefix(id, "fld_") || len(id) != len("fld_")+10 {
			t.Fatalf("generated id %q has wrong shape", id)
		}
		for _, r := range id[len("fld_"):] {
			if !strings.ContainsRune(fieldIDCharset, r) {
				t.Fatalf("generated id %q contains non-base36 rune %q", id, r)
			}
		}
		if seen[id] {
			t.Fatalf("duplicate id generated: %q", id)
		}
		seen[id] = true
	}
}

func TestValidFieldIDRejectsMalformed(t *testing.T) {
	bad := []string{
		"",
		"fld_",
		"fld_short",
		"fld_UPPERCASE1",
		"fld_0123456789a",       // 11 chars
		"xfld_0123456789",       // wrong prefix
		"fld_0123456789 ",       // trailing space
		"fld_01234'; DROP --",   // injection attempt
		"data->>'fld_012345aa'", // expression smuggling
	}
	for _, id := range bad {
		if ValidFieldID(id) {
			t.Fatalf("ValidFieldID accepted malformed id %q", id)
		}
	}
	if !ValidFieldID("fld_0123456789") {
		t.Fatalf("ValidFieldID rejected a canonical id")
	}
}
