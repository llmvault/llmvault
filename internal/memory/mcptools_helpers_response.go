package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func (s *Service) loadToolMemory(ctx context.Context, orgID, memoryID uuid.UUID) (model.AgentMemory, error) {
	var mem model.AgentMemory
	err := s.cfg.DB.WithContext(ctx).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", memoryID, orgID).
		First(&mem).Error
	return mem, err
}

func memoryToolMetadata(metadata model.JSON, agentID uuid.UUID) model.JSON {
	out := model.JSON{}
	for key, value := range metadata {
		out[key] = value
	}
	out["source"] = "mcp_memory_tool"
	out["created_by_agent_id"] = agentID.String()
	return out
}

const (
	memoryLayerObservations = "observations"
	memoryLayerFacts        = "facts"
)

func observationToolSearchResponses(hits []ObservationHit) []map[string]any {
	out := make([]map[string]any, 0, len(hits))
	for _, hit := range hits {
		similarity := hit.Similarity
		out = append(out, observationToolResponse(hit.Observation, &similarity))
	}
	return out
}

func observationToolResponse(obs model.AgentObservation, similarity *float64) map[string]any {
	out := map[string]any{
		"id":                obs.ID.String(),
		"content":           obs.Content,
		"kind":              obs.Kind,
		"entities":          []string(obs.Entities),
		"proof_count":       obs.ProofCount,
		"last_mentioned_at": obs.LastMentionedAt,
		"human_verified":    obs.HumanVerified,
		"channel_id":        channelIDValue(obs.ChannelID),
		"created_at":        obs.CreatedAt,
		"updated_at":        obs.UpdatedAt,
	}
	if similarity != nil {
		out["similarity"] = *similarity
	}
	return out
}

func memoryToolSearchResponses(hits []SearchHit) []map[string]any {
	out := make([]map[string]any, 0, len(hits))
	for _, hit := range hits {
		similarity := hit.Similarity
		out = append(out, memoryToolMemoryResponse(hit.Memory, &similarity))
	}
	return out
}

func memoryToolMemoryResponse(mem model.AgentMemory, similarity *float64) map[string]any {
	out := map[string]any{
		"id":                 mem.ID.String(),
		"content":            mem.Content,
		"tags":               []string(mem.Tags),
		"channel_id":         channelIDValue(mem.ChannelID),
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

// channelIDValue renders a nullable channel id: nil = org-wide.
func channelIDValue(id *uuid.UUID) any {
	if id == nil {
		return nil
	}
	return id.String()
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

func memoryToolLoadMessage(err error) string {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "memory not found"
	}
	return "failed to load memory: " + err.Error()
}
