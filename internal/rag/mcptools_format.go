package rag

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/rag/qdrant"
)

func groupKnowledgeHitsBySource(hits []qdrant.Hit) []map[string]any {
	type sourceGroup struct {
		key     string
		summary map[string]any
		results []map[string]any
	}
	ordered := make([]*sourceGroup, 0)
	byKey := make(map[string]*sourceGroup)
	for _, hit := range hits {
		sourceKey := sourceKey(hit)
		group := byKey[sourceKey]
		if group == nil {
			group = &sourceGroup{
				key: sourceKey,
				summary: map[string]any{
					"source_id": sourceKey,
				},
			}
			if hit.Payload != nil {
				copySourceSummary(group.summary, hit.Payload)
				copyFirstString(group.summary, "link", hit.Payload, "link", "url", "permalink")
				copyFirstString(group.summary, "rag_source_id", hit.Payload, "rag_source_id")
				copyFirstString(group.summary, "doc_id", hit.Payload, "doc_id")
			}
			byKey[sourceKey] = group
			ordered = append(ordered, group)
		}
		row := map[string]any{
			"id":    hit.ID,
			"score": hit.Score,
		}
		if hit.Payload != nil {
			copyFirstString(row, "doc_id", hit.Payload, "doc_id")
			copyFirstString(row, "semantic_id", hit.Payload, "semantic_id")
			copyFirstString(row, "link", hit.Payload, "link", "url", "permalink")
			copyFirstString(row, "title", hit.Payload, "title", "semantic_id")
			copyInt(row, "part_index", hit.Payload)
			if content, ok := hit.Payload["content"].(string); ok {
				row["excerpt"] = truncate(content, 900)
			}
		}
		group.results = append(group.results, row)
	}

	out := make([]map[string]any, 0, len(ordered))
	for _, group := range ordered {
		group.summary["result_count"] = len(group.results)
		group.summary["chunks"] = group.results
		out = append(out, group.summary)
	}
	return out
}

func sourceKey(hit qdrant.Hit) string {
	if hit.Payload != nil {
		if source := sourceMap(hit.Payload); source != nil {
			if value := stringFromPayload(source, "id"); value != "" {
				return value
			}
		}
		for _, key := range []string{"rag_source_id", "source_id", "doc_id", "semantic_id", "link"} {
			if value, ok := hit.Payload[key].(string); ok && value != "" {
				return value
			}
		}
	}
	return fmt.Sprint(hit.ID)
}

func copySourceSummary(dst map[string]any, payload map[string]any) {
	source := sourceMap(payload)
	if source == nil {
		return
	}
	dst["source"] = source
	copyFirstString(dst, "source_id", source, "id")
	copyFirstString(dst, "title", source, "name")
}

func sourceMap(payload map[string]any) map[string]any {
	if source, ok := payload["source"].(map[string]any); ok {
		return source
	}
	if source, ok := payload["source"].(map[string]string); ok {
		out := make(map[string]any, len(source))
		for key, value := range source {
			out[key] = value
		}
		return out
	}
	return nil
}

func copyFirstString(dst map[string]any, dstKey string, src map[string]any, keys ...string) {
	for _, key := range keys {
		if value := stringFromPayload(src, key); value != "" {
			dst[dstKey] = value
			return
		}
	}
}

func stringFromPayload(src map[string]any, key string) string {
	if value, ok := src[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func copyInt(dst map[string]any, key string, src map[string]any) {
	switch value := src[key].(type) {
	case int:
		dst[key] = value
	case int64:
		dst[key] = value
	case float64:
		dst[key] = int(value)
	}
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max] + "..."
}

func toolError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: %s", msg)}},
		IsError: true,
	}
}

func toolJSON(v any) (*mcp.CallToolResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return toolError("failed to serialize response"), nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(b)}},
	}, nil
}
