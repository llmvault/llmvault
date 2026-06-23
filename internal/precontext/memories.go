package precontext

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/memory"
)

func (s *Service) fetchMemoriesSection(ctx context.Context, req Request) (string, error) {
	if isNilValue(s.cfg.Memories) || req.OrgID == uuid.Nil || req.AgentID == uuid.Nil || strings.TrimSpace(req.Text) == "" {
		return "", nil
	}
	var userID *uuid.UUID
	if parsed, err := uuid.Parse(strings.TrimSpace(req.UserID)); err == nil {
		userID = &parsed
	}
	memories, err := s.cfg.Memories.SearchForTurn(ctx, memory.SearchRequest{
		OrgID:   req.OrgID,
		UserID:  userID,
		AgentID: req.AgentID,
		Query:   req.Text,
		Limit:   5,
	})
	if err != nil {
		return "", fmt.Errorf("search memories: %w", err)
	}
	parts := make([]string, 0, 2)
	if org := formatMemoryHits("Organization", memories.Org); org != "" {
		parts = append(parts, org)
	}
	if user := formatMemoryHits("User", memories.User); user != "" {
		parts = append(parts, user)
	}
	return section("## Relevant memories", strings.Join(parts, "\n"), MemoriesBudgetBytes), nil
}

func formatMemoryHits(label string, hits []memory.SearchHit) string {
	lines := make([]string, 0, len(hits)+1)
	for _, hit := range hits {
		if line := formatMemoryHit(hit); line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return label + ":\n" + strings.Join(lines, "\n")
}

func formatMemoryHit(hit memory.SearchHit) string {
	content := cleanText(hit.Memory.Content)
	if content == "" {
		return ""
	}
	line := "- "
	if tags := memory.NormalizeTags([]string(hit.Memory.Tags)); len(tags) > 0 {
		line += "[" + trimToBytes(strings.Join(tags, ","), 80) + "] "
	}
	return trimToBytes(line+content, 320)
}
