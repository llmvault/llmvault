package precontext

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/memory"
	"github.com/usehivy/hivy/internal/model"
)

const (
	latestOrgMemoryLimit     = 500
	fallbackObservationLimit = 25
	memoryLineMaxBytes       = 320
	memoriesSectionTitle     = "## Memories"
	memoryRulesHeading       = "### Rules"
)

// fetchMemoriesSection builds the recall block injected into every session:
// active directives first (hard rules, verbatim, never trimmed before the rest
// of the block), then the channel's precomputed memory digest. When the digest
// row is missing or empty it falls back to the channel's top observations, and
// when observations are unavailable too (pre-migration or pre-consolidation)
// to the legacy newest-facts listing, so nothing breaks during rollout.
//
// Hard constraint: this runs on the synchronous session-create path. No LLM or
// embedding calls; at most three cheap indexed queries (directives + digest,
// plus one more only on the fallback chain).
func (s *Service) fetchMemoriesSection(ctx context.Context, req Request) (string, error) {
	if isNilValue(s.cfg.Memories) || req.OrgID == uuid.Nil || req.ChannelID == uuid.Nil {
		return "", nil
	}
	channelID := req.ChannelID
	scope := memory.ChannelScope{
		ChannelID:          &channelID,
		IncludeOrgMemories: req.IncludeOrgMemories,
	}
	rules := s.directivesBlock(ctx, req.OrgID, scope)
	body, err := s.recallBlock(ctx, req, scope)
	if err != nil {
		if rules == "" {
			return "", err
		}
		// Directives are hard rules: render them even when recall fails.
		logging.Capture(ctx, fmt.Errorf("agent precontext memories recall: %w", err))
	}
	return memoriesSection(rules, body), nil
}

// directivesBlock renders every active directive in scope as a Rules
// subsection, one bullet each, verbatim. Directives are human-approved, so
// they are never ranked or sampled; a directives failure only costs the
// subsection, never the rest of recall.
func (s *Service) directivesBlock(ctx context.Context, orgID uuid.UUID, scope memory.ChannelScope) string {
	directives, err := s.cfg.Memories.ActiveDirectives(ctx, orgID, scope)
	if err != nil {
		logging.Capture(ctx, fmt.Errorf("agent precontext memories directives: %w", err))
		return ""
	}
	lines := make([]string, 0, len(directives))
	for _, directive := range directives {
		if content := cleanText(directive.Content); content != "" {
			lines = append(lines, "- "+content)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return memoryRulesHeading + "\n" + strings.Join(lines, "\n")
}

// recallBlock returns the ranked memory body: digest → top observations →
// legacy facts. Digest and observation errors degrade down the chain; only a
// legacy listing failure surfaces as an error.
func (s *Service) recallBlock(ctx context.Context, req Request, scope memory.ChannelScope) (string, error) {
	digest, err := s.cfg.Memories.ChannelMemoryDigest(ctx, req.OrgID, req.ChannelID)
	if err != nil {
		logging.Capture(ctx, fmt.Errorf("agent precontext memories digest: %w", err))
	} else if digest = strings.TrimSpace(digest); digest != "" {
		// Pre-rendered markdown bullets, ranked and byte-budgeted at write time
		// by the consolidation worker (org-wide observations already folded in
		// per the channel's expose_org_memories flag).
		return digest, nil
	}
	observations, err := s.cfg.Memories.TopObservations(ctx, req.OrgID, scope, fallbackObservationLimit)
	if err != nil {
		logging.Capture(ctx, fmt.Errorf("agent precontext memories observations: %w", err))
	} else if len(observations) > 0 {
		return formatObservations(observations), nil
	}
	rows, err := s.cfg.Memories.List(ctx, memory.ListRequest{
		OrgID: req.OrgID,
		Scope: scope,
		Limit: latestOrgMemoryLimit,
	})
	if err != nil {
		return "", fmt.Errorf("list channel memories: %w", err)
	}
	return formatMemories(rows), nil
}

// memoriesSection assembles the section within MemoriesBudgetBytes. Directives
// get the budget first; the digest/observations body is trimmed to whatever
// remains, so rules are never trimmed away before observations are.
func memoriesSection(rules, body string) string {
	bodyBudget := MemoriesBudgetBytes - len(memoriesSectionTitle) - 8
	rules = trimToBytes(rules, bodyBudget)
	if rules != "" {
		bodyBudget -= len(rules) + 2
	}
	body = trimToBytes(body, bodyBudget)
	combined := rules
	if body != "" {
		if combined != "" {
			combined += "\n\n"
		}
		combined += body
	}
	return section(memoriesSectionTitle, combined, MemoriesBudgetBytes)
}

func formatObservations(rows []model.AgentObservation) string {
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		content := cleanText(row.Content)
		if content == "" {
			continue
		}
		line := "- "
		if kind := cleanText(row.Kind); kind != "" {
			line += "[" + trimToBytes(kind, 80) + "] "
		}
		lines = append(lines, trimToBytes(line+content, memoryLineMaxBytes))
	}
	return strings.Join(lines, "\n")
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
	return trimToBytes(line+content, memoryLineMaxBytes)
}
