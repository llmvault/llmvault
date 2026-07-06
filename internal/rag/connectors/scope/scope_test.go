package scope

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestParse_NoScope(t *testing.T) {
	for _, raw := range []string{``, `null`, `{}`, `{"other":1}`} {
		_, present, err := Parse(json.RawMessage(raw))
		if err != nil {
			t.Fatalf("Parse(%q) err: %v", raw, err)
		}
		if present {
			t.Fatalf("Parse(%q) present=true, want false", raw)
		}
	}
}

func TestParse_WithScope(t *testing.T) {
	raw := json.RawMessage(`{"scope":{"resource_type":"repository","items":[
		{"id":"acme/widget","name":"widget"},
		{"id":" acme/gadget ","name":"gadget"},
		{"id":"acme/widget"},
		{"id":""}
	]}}`)
	sc, present, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse err: %v", err)
	}
	if !present {
		t.Fatal("present=false, want true")
	}
	if sc.ResourceType != "repository" {
		t.Fatalf("resource_type=%q", sc.ResourceType)
	}
	got := sc.IDs()
	want := []string{"acme/widget", "acme/gadget"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("IDs()=%v, want %v (trim+dedup+drop-empty)", got, want)
	}
}

func TestParse_Malformed(t *testing.T) {
	if _, _, err := Parse(json.RawMessage(`{"scope":"notanobject"}`)); err == nil {
		t.Fatal("expected error for malformed scope")
	}
}
