package precontext

import (
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/rag/qdrant"
)

func TestFormatKnowledgeHitIncludesSourceLabel(t *testing.T) {
	out := formatKnowledgeHit(qdrant.Hit{
		Payload: map[string]any{
			"source": map[string]any{
				"name":     "Engineering Slack",
				"provider": "slack",
			},
			"semantic_id": "Deploy policy",
			"link":        "https://example.com/deploy",
			"content":     "Production deploys need rollback notes.",
		},
	})

	for _, want := range []string{"[Engineering Slack]", "Deploy policy", "https://example.com/deploy", "rollback notes"} {
		if !strings.Contains(out, want) {
			t.Fatalf("formatted knowledge missing %q: %s", want, out)
		}
	}
}
