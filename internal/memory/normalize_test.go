package memory

import (
	"strings"
	"testing"
)

func TestNormalizeTagsDedupesAndBounds(t *testing.T) {
	tags := NormalizeTags([]string{" Launch ", "launch", "Escalation", "", strings.Repeat("x", 80)})
	if len(tags) != 3 {
		t.Fatalf("expected 3 normalized tags, got %#v", tags)
	}
	if tags[0] != "launch" || tags[1] != "escalation" || len(tags[2]) != 64 {
		t.Fatalf("unexpected normalized tags: %#v", tags)
	}
}

func TestNormalizeContentAndVectorValidation(t *testing.T) {
	content, err := normalizeContent("  The launch codename is Helio.  ")
	if err != nil {
		t.Fatalf("normalizeContent returned error: %v", err)
	}
	if content != "The launch codename is Helio." {
		t.Fatalf("unexpected content: %q", content)
	}
	if _, err := normalizeContent(" "); err == nil {
		t.Fatalf("expected empty content error")
	}
	if err := validateVector([]float32{0.1, 0.2}, 2); err != nil {
		t.Fatalf("validateVector returned error: %v", err)
	}
	if err := validateVector([]float32{0.1}, 2); err == nil {
		t.Fatalf("expected vector dimension error")
	}
}
