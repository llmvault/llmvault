package employeesandbox

import "testing"

func TestImageRepositoryNormalizesTagsAndDigests(t *testing.T) {
	cases := map[string]string{
		"ghcr.io/usehivy/hivy-sandboxes-runtime:v3.1.5":                   "ghcr.io/usehivy/hivy-sandboxes-runtime",
		"ghcr.io/usehivy/hivy-sandboxes-runtime@sha256:abcdef":            "ghcr.io/usehivy/hivy-sandboxes-runtime",
		"ghcr.io/usehivy/hivy-sandboxes-runtime-specialist:v3.1.5":        "ghcr.io/usehivy/hivy-sandboxes-runtime-specialist",
		"localhost:5000/usehivy/hivy-sandboxes-runtime:v3.1.5":            "localhost:5000/usehivy/hivy-sandboxes-runtime",
		" localhost:5000/usehivy/hivy-sandboxes-runtime-specialist:test ": "localhost:5000/usehivy/hivy-sandboxes-runtime-specialist",
	}
	for input, want := range cases {
		if got := imageRepository(input); got != want {
			t.Fatalf("imageRepository(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDefaultSpecialistRepositoryDerivesFromEmployeeRepository(t *testing.T) {
	got := defaultSpecialistRepository("ghcr.io/usehivy/hivy-sandboxes-runtime:v3.1.6")
	want := "ghcr.io/usehivy/hivy-sandboxes-runtime-specialist"
	if got != want {
		t.Fatalf("defaultSpecialistRepository() = %q, want %q", got, want)
	}
}
