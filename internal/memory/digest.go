package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// ChannelMemoryDigest returns the channel's precomputed recall block: markdown
// bullets ranked and byte-budgeted at write time by the consolidation worker
// (org-wide observations already folded in per the channel's
// expose_org_memories flag). One indexed point-read; "" means no digest yet —
// callers fall back to TopObservations, then to the legacy fact listing.
func (s *Service) ChannelMemoryDigest(ctx context.Context, orgID, channelID uuid.UUID) (string, error) {
	if s == nil || s.cfg.DB == nil {
		return "", fmt.Errorf("memory service is not configured")
	}
	if orgID == uuid.Nil || channelID == uuid.Nil {
		return "", fmt.Errorf("org_id and channel_id are required")
	}
	var content string
	err := s.cfg.DB.WithContext(ctx).
		Raw(`SELECT content FROM channel_memory_digests WHERE channel_id = ? AND org_id = ?`, channelID, orgID).
		Scan(&content).Error
	if err != nil {
		if isUndefinedTable(err) {
			return "", nil
		}
		return "", fmt.Errorf("load channel memory digest: %w", err)
	}
	return strings.TrimSpace(content), nil
}

// ActiveDirectives returns every active directive in scope, oldest first (all
// of them: the bar for a directive is human approval, so recall never ranks or
// samples them). Org-wide directives (channel_id IS NULL) fold in via the same
// ChannelScope semantics as every other memory read. Before the directives
// migration lands the table reads as empty.
func (s *Service) ActiveDirectives(ctx context.Context, orgID uuid.UUID, scope ChannelScope) ([]model.AgentDirective, error) {
	if s == nil || s.cfg.DB == nil {
		return nil, fmt.Errorf("memory service is not configured")
	}
	if orgID == uuid.Nil {
		return nil, fmt.Errorf("org_id is required")
	}
	q := s.cfg.DB.WithContext(ctx).
		Where("org_id = ? AND active", orgID).
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
