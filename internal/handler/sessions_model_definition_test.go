package handler

import "testing"

func TestNormalizeSessionReasoningEffort_AllowsSparseDefault(t *testing.T) {
	got, err := normalizeSessionReasoningEffort("")
	if err != nil {
		t.Fatalf("normalize blank reasoning: %v", err)
	}
	if got != "" {
		t.Fatalf("blank reasoning = %q, want sparse empty value", got)
	}
}
