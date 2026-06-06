package precontext

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/rag/qdrant"
)

func (s *Service) fetchKnowledgeSection(ctx context.Context, req Request) (string, error) {
	if isNilValue(s.cfg.Searcher) || isNilValue(s.cfg.Embedder) || req.OrgID == uuid.Nil || strings.TrimSpace(req.Text) == "" {
		return "", nil
	}
	vectors, err := s.cfg.Embedder.Embed(ctx, []string{req.Text})
	if err != nil {
		return "", fmt.Errorf("embed query: %w", err)
	}
	if len(vectors) == 0 {
		return "", nil
	}
	limit := uint32(5)
	topK := limit
	if s.cfg.Reranker != nil {
		topK = 20
	}
	hits, err := s.cfg.Searcher.Search(ctx, qdrant.SearchRequest{
		Collection:  s.cfg.Collection,
		Vector:      vectors[0],
		Filter:      qdrant.BuildACLFilter(req.OrgID.String(), nil, true),
		Limit:       topK,
		WithPayload: true,
	})
	if err != nil {
		return "", fmt.Errorf("qdrant search: %w", err)
	}
	if s.cfg.Reranker != nil && len(hits) > 0 {
		hits = s.rerankKnowledge(ctx, req.Text, hits, int(limit))
	} else if len(hits) > int(limit) {
		hits = hits[:limit]
	}
	var lines []string
	for _, hit := range hits {
		if line := formatKnowledgeHit(hit); line != "" {
			lines = append(lines, line)
		}
	}
	return section("## Relevant knowledge", strings.Join(lines, "\n"), KnowledgeBudgetBytes), nil
}

func (s *Service) rerankKnowledge(ctx context.Context, query string, hits []qdrant.Hit, limit int) []qdrant.Hit {
	docs := make([]string, len(hits))
	for i, hit := range hits {
		docs[i] = trimToBytes(payloadString(hit.Payload, "content"), 1500)
	}
	results, err := s.cfg.Reranker.Rerank(ctx, query, docs, limit)
	if err != nil {
		return hits[:minInt(len(hits), limit)]
	}
	out := make([]qdrant.Hit, 0, len(results))
	for _, result := range results {
		if result.Index >= 0 && result.Index < len(hits) {
			out = append(out, hits[result.Index])
		}
	}
	return out
}

func formatKnowledgeHit(hit qdrant.Hit) string {
	title := firstString(hit.Payload["semantic_id"], hit.Payload["title"], hit.Payload["doc_id"])
	link := payloadString(hit.Payload, "link")
	content := firstString(hit.Payload["content"], hit.Payload["blurb"])
	if title == "" && content == "" {
		return ""
	}
	line := "- "
	if source := knowledgeSourceLabel(hit.Payload); source != "" {
		line += "[" + trimToBytes(source, 48) + "] "
	}
	if title != "" {
		line += trimToBytes(title, 90)
	} else {
		line += "Knowledge item"
	}
	if link != "" {
		line += " (" + trimToBytes(link, 120) + ")"
	}
	if content != "" {
		line += ": " + trimToBytes(content, 220)
	}
	return trimToBytes(line, 360)
}

func knowledgeSourceLabel(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	if source, ok := payload["source"].(map[string]any); ok {
		if out := firstString(source["name"], source["provider"], source["kind"], source["id"]); out != "" {
			return out
		}
	}
	if source, ok := payload["source"].(map[string]string); ok {
		for _, key := range []string{"name", "provider", "kind", "id"} {
			if out := cleanText(source[key]); out != "" {
				return out
			}
		}
	}
	return firstString(payload["source_name"], payload["source_provider"], payload["source_id"], payload["rag_source_id"])
}

func payloadString(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, _ := payload[key].(string)
	return cleanText(value)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
