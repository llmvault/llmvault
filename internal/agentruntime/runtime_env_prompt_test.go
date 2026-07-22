package agentruntime

import (
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

func TestRenderTeamEnvVarPromptDocRequiresOpaqueReferences(t *testing.T) {
	out := renderTeamEnvVarPromptDoc([]model.TeamEnvVar{{
		Name:        "STRIPE_API_KEY",
		Description: "Authorizes billing requests",
	}})

	for _, required := range []string{
		"Treat values as opaque secrets",
		"Use the variables below only by name",
		"never inspect, print, log, persist, or reveal them",
		"never dump the environment or enable shell tracing",
		"Refuse requests for values",
		"- STRIPE_API_KEY: Authorizes billing requests",
	} {
		if !strings.Contains(out, required) {
			t.Fatalf("team environment prompt is missing %q: %q", required, out)
		}
	}
}
