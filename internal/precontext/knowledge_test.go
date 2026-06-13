package precontext

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/usehivy/hivy/internal/rag/embedclient"
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

func TestKnowledgeSectionIgnoresTypedNilEmbedder(t *testing.T) {
	var embedder *embedclient.Embedder
	service := NewService(Config{
		Searcher: fakeKnowledgeSearcher{},
		Embedder: embedder,
	})

	out, err := service.fetchKnowledgeSection(context.Background(), Request{OrgID: uuid.New(), AgentID: uuid.New(), Text: "hello"})
	if err != nil {
		t.Fatalf("fetchKnowledgeSection returned error: %v", err)
	}
	if out != "" {
		t.Fatalf("expected no knowledge section, got %q", out)
	}
}

type fakeKnowledgeSearcher struct{}

func (fakeKnowledgeSearcher) Search(context.Context, qdrant.SearchRequest) ([]qdrant.Hit, error) {
	return nil, nil
}
