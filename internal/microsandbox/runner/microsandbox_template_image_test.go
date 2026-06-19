package runner

import (
	"strings"
	"testing"
)

func TestTemplateDockerfilePreservesBaseEntrypointAndRunsCommands(t *testing.T) {
	got := templateDockerfile("ghcr.io/usehivy/runtime:test", []string{
		"apt-get update",
		"apt-get install -y openjdk-21-jdk",
	})
	for _, want := range []string{
		"FROM ghcr.io/usehivy/runtime:test",
		"SHELL [\"/bin/bash\", \"-o\", \"pipefail\", \"-c\"]",
		"RUN <<'HIVY_TEMPLATE_SCRIPT'",
		"apt-get update",
		"apt-get install -y openjdk-21-jdk",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Dockerfile missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "ENTRYPOINT") || strings.Contains(got, "CMD ") {
		t.Fatalf("Dockerfile must not override base entrypoint/cmd:\n%s", got)
	}
}

func TestTemplateImageRefsUseFixedImagesPathAndDigest(t *testing.T) {
	mutable := templateMutableImageRef("10.80.0.3:5000", "org_1", "tpl_ready", "bld-123")
	if mutable != "10.80.0.3:5000/images/org_1/tpl_ready:bld-123" {
		t.Fatalf("mutable ref = %q", mutable)
	}
	final := templateDigestImageRef(mutable, "sha256:abc")
	if final != "10.80.0.3:5000/images/org_1/tpl_ready@sha256:abc" {
		t.Fatalf("final ref = %q", final)
	}
}
