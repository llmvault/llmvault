package agentemail

import "testing"

func TestMessageIDsAndHeaders(t *testing.T) {
	headers := map[string]string{"references": "<one@example.test> ignored <two@example.test>"}
	if got := Header(headers, "References"); got != "<one@example.test> ignored <two@example.test>" {
		t.Fatalf("Header() = %q", got)
	}
	ids := MessageIDs(Header(headers, "REFERENCES"))
	if len(ids) != 2 || ids[0] != "<one@example.test>" || ids[1] != "<two@example.test>" {
		t.Fatalf("MessageIDs() = %#v", ids)
	}
}
