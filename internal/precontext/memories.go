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
	if isNilValue(s.cfg.Memories) || req.OrgID == uuid.Nil || req.ChannelID == uuid.Nil {
		return "", nil
	}
	channelID := req.ChannelID
	rows, err := s.cfg.Memories.List(ctx, memory.ListRequest{
		OrgID: req.OrgID,
		Scope: memory.ChannelScope{
			ChannelID:          &channelID,
			IncludeOrgMemories: req.IncludeOrgMemories,
		},
		Limit: latestOrgMemoryLimit,
	})
	if err != nil {
		return "", fmt.Errorf("list channel memories: %w", err)
	}
	return section("## Memories", formatMemories(rows), MemoriesBudgetBytes), nil
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
