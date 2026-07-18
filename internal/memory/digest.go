package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// AgentMemoryDigest returns one agent's precomputed recall block. One indexed
// point-read; an empty result means consolidation has not produced a digest.
func (s *Service) AgentMemoryDigest(ctx context.Context, orgID, agentID uuid.UUID) (string, error) {
	if s == nil || s.cfg.DB == nil {
		return "", fmt.Errorf("memory service is not configured")
	}
	if orgID == uuid.Nil || agentID == uuid.Nil {
		return "", fmt.Errorf("org_id and agent_id are required")
	}
	var content string
	err := s.cfg.DB.WithContext(ctx).
		Raw(`SELECT content FROM agent_memory_digests WHERE agent_id = ? AND org_id = ?`, agentID, orgID).
		Scan(&content).Error
	if err != nil {
		if isUndefinedTable(err) {
			return "", nil
		}
		return "", fmt.Errorf("load agent memory digest: %w", err)
	}
	return strings.TrimSpace(content), nil
}

// ActiveDirectives returns active, agent-owned directives oldest first.
func (s *Service) ActiveDirectives(ctx context.Context, orgID uuid.UUID, scope AgentScope) ([]model.AgentDirective, error) {
	if s == nil || s.cfg.DB == nil {
		return nil, fmt.Errorf("memory service is not configured")
	}
	if orgID == uuid.Nil {
		return nil, fmt.Errorf("org_id is required")
	}
	q := s.cfg.DB.WithContext(ctx).
		Where("org_id = ? AND active AND deleted_at IS NULL", orgID).
		Order("created_at ASC, id ASC")
	if clause, args := scope.whereSQL(); clause != "" {
		q = q.Where(clause, args...)
	}
	var rows []model.AgentDirective
	if err := q.Find(&rows).Error; err != nil {
		if isUndefinedTable(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list active directives: %w", err)
	}
	return rows, nil
}
