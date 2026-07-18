package memory

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/model"
)

const (
	memoryLayerObservations = "observations"
	memoryLayerFacts        = "facts"
)

func observationToolResponse(obs model.AgentObservation, similarity *float64) map[string]any {
	out := map[string]any{
		"id":                obs.ID.String(),
		"content":           obs.Content,
		"kind":              obs.Kind,
		"entities":          []string(obs.Entities),
		"proof_count":       obs.ProofCount,
		"last_mentioned_at": obs.LastMentionedAt,
		"human_verified":    obs.HumanVerified,
		"agent_id":          obs.AgentID.String(),
		"created_at":        obs.CreatedAt,
		"updated_at":        obs.UpdatedAt,
	}
	if similarity != nil {
		out["similarity"] = *similarity
	}
	// Evolution history rides along automatically: superseded wordings with
	// dates and reasons, newest first, so one search call yields the belief
	// plus how it changed over time.
	if history := ObservationHistory(obs, searchHistoryMaxEntries); len(history) > 0 {
		out["history"] = history
	}
	return out
}

func memoryToolMemoryResponse(mem model.AgentMemory, similarity *float64) map[string]any {
	out := map[string]any{
		"id":                 mem.ID.String(),
		"content":            mem.Content,
		"tags":               []string(mem.Tags),
		"agent_id":           mem.AgentID.String(),
		"embedding_status":   mem.EmbeddingStatus,
		"embedding_revision": mem.EmbeddingRevision,
		"created_at":         mem.CreatedAt,
		"updated_at":         mem.UpdatedAt,
	}
	if similarity != nil {
		out["similarity"] = *similarity
	}
	return out
}

func normalizeMemoryToolSearchQuery(raw string) (string, error) {
	query := strings.TrimSpace(raw)
	if query == "" {
		return "", fmt.Errorf("query is required")
	}
	if len(query) > memoryToolQueryMaxChars {
		return "", fmt.Errorf("query must be at most %d characters", memoryToolQueryMaxChars)
	}
	if len(strings.Fields(query)) > memoryToolQueryMaxWords {
		return "", fmt.Errorf("query must be at most %d words", memoryToolQueryMaxWords)
	}
	return query, nil
}

func normalizeMemoryToolTags(values []string) ([]string, error) {
	if len(values) > memoryToolMaxTags {
		return nil, fmt.Errorf("tags must contain at most %d items", memoryToolMaxTags)
	}
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, raw := range values {
		tag := strings.TrimSpace(raw)
		if tag == "" {
			return nil, fmt.Errorf("tags must be non-empty lowercase kebab-case slugs")
		}
		if !memoryToolTagRE.MatchString(tag) {
			return nil, fmt.Errorf("tags must be lowercase kebab-case slugs, for example project-helio")
		}
		if seen[tag] {
			return nil, fmt.Errorf("tags must be unique")
		}
		seen[tag] = true
		out = append(out, tag)
	}
	return out, nil
}

func memoryToolError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + msg}},
		IsError: true,
	}
}

func memoryToolJSON(v any) (*mcp.CallToolResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return memoryToolError("failed to serialize response"), nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil
}
