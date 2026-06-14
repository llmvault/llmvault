package docker

import "testing"

func TestNormalizeRuntimeOrigin(t *testing.T) {
	t.Parallel()

	got, err := normalizeRuntimeOrigin("http://192.0.2.10/")
	if err != nil {
		t.Fatalf("normalizeRuntimeOrigin valid origin: %v", err)
	}
	if got != "http://192.0.2.10" {
		t.Fatalf("origin = %q", got)
	}

	for _, value := range []string{
		"192.0.2.10",
		"http://192.0.2.10/runtime",
		"http://192.0.2.10:7080",
		"http://host.docker.internal",
	} {
		if got, err := normalizeRuntimeOrigin(value); err == nil {
			t.Fatalf("normalizeRuntimeOrigin(%q) = %q, want error", value, got)
		}
	}
}
