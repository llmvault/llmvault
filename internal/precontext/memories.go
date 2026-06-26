package precontext

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/memory"
	"github.com/usehivy/hivy/internal/model"
)

const latestOrgMemoryLimit = 500

func (s *Service) fetchMemoriesSection(ctx context.Context, req Request) (string, error) {
	if isNilValue(s.cfg.Memories) || req.OrgID == uuid.Nil {
		return "", nil
	}
	orgRows, err := s.cfg.Memories.List(ctx, memory.ListRequest{
		OrgID:           req.OrgID,
		Scope:           model.AgentMemoryScopeOrg,
		AgentVisibility: memory.AgentVisibilityAllAgents,
		Limit:           latestOrgMemoryLimit,
	})
	if err != nil {
		return "", fmt.Errorf("list org memories: %w", err)
	}
	var agentRows []model.AgentMemory
	if req.AgentID != uuid.Nil {
		agentRows, err = s.cfg.Memories.List(ctx, memory.ListRequest{
			OrgID:           req.OrgID,
			AgentID:         &req.AgentID,
			Scope:           model.AgentMemoryScopeOrg,
			AgentVisibility: memory.AgentVisibilityThisAgent,
			NoLimit:         true,
		})
		if err != nil {
			return "", fmt.Errorf("list agent memories: %w", err)
		}
	}
	return section("## Memories", formatMemoryGroups(orgRows, agentRows), MemoriesBudgetBytes), nil
}

func formatMemoryGroups(orgRows, agentRows []model.AgentMemory) string {
	parts := make([]string, 0, 2)
	if org := formatMemories(orgRows); org != "" {
		parts = append(parts, "Organization:\n"+org)
	}
	if agent := formatMemories(agentRows); agent != "" {
		parts = append(parts, "Agent:\n"+agent)
	}
	return strings.Join(parts, "\n")
}

func formatMemories(rows []model.AgentMemory) string {
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		if line := formatMemory(row); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func formatMemory(mem model.AgentMemory) string {
	content := cleanText(mem.Content)
	if content == "" {
		return ""
	}
	line := "- "
	if tags := memory.NormalizeTags([]string(mem.Tags)); len(tags) > 0 {
		line += "[" + trimToBytes(strings.Join(tags, ","), 80) + "] "
	}
	return trimToBytes(line+content, 320)
}
