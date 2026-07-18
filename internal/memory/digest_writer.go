package memory

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

const (
	// DigestMaxObservations caps how many ranked observations one agent
	// digest may carry (K).
	DigestMaxObservations = 250
	// DigestByteBudget caps the rendered digest block size; trimming happens
	// on whole lines. The memories section ceiling is 64KB and rules take
	// priority within it, so the digest keeps a comfortable margin below.
	DigestByteBudget = 48 * 1024
	// digestRecencyHalfLifeDays gives the mild recency decay: an observation
	// last mentioned 180 days ago scores half of a fresh one, all else equal.
	digestRecencyHalfLifeDays = 180.0
)

// digestKindWeights rank durable rules and decisions above findings.
var digestKindWeights = map[string]float64{
	"rule":       4,
	"decision":   3,
	"org-fact":   3,
	"convention": 3,
	"preference": 2.5,
	"person":     2,
	"commitment": 2,
	"workaround": 1.5,
	"finding":    1,
}

// DigestKindWeight returns the ranking weight for an observation kind
// (unknown kinds weigh like findings).
func DigestKindWeight(kind string) float64 {
	if weight, ok := digestKindWeights[kind]; ok {
		return weight
	}
	return 1
}

// DigestScore ranks one observation for an agent digest:
// kind weight * ln(1+proof_count), with a mild half-life recency decay on
// last_mentioned_at.
func DigestScore(obs model.AgentObservation, now time.Time) float64 {
	ageDays := now.Sub(obs.LastMentionedAt).Hours() / 24
	if ageDays < 0 {
		ageDays = 0
	}
	decay := math.Pow(0.5, ageDays/digestRecencyHalfLifeDays)
	return DigestKindWeight(obs.Kind) * math.Log(1+float64(obs.ProofCount)) * decay
}

// RankObservationsForDigest sorts observations by descending digest score
// (ties broken by recency then id for stability).
func RankObservationsForDigest(rows []model.AgentObservation, now time.Time) []model.AgentObservation {
	ranked := make([]model.AgentObservation, len(rows))
	copy(ranked, rows)
	sort.SliceStable(ranked, func(i, j int) bool {
		si, sj := DigestScore(ranked[i], now), DigestScore(ranked[j], now)
		if si != sj {
			return si > sj
		}
		if !ranked[i].LastMentionedAt.Equal(ranked[j].LastMentionedAt) {
			return ranked[i].LastMentionedAt.After(ranked[j].LastMentionedAt)
		}
		return ranked[i].ID.String() < ranked[j].ID.String()
	})
	return ranked
}

// RenderAgentDigest ranks observations, takes the top maxObservations, and
// renders them as `- [kind] content` markdown bullets within budgetBytes
// (whole lines only). Observations whose content was rewritten carry a
// compact evolution note (their most recent superseded wordings with dates,
// explicitly marked as former wordings) so agents see how a memory evolved
// without any tool call. Returns the rendered block and how many observations
// it carries.
func RenderAgentDigest(rows []model.AgentObservation, now time.Time, maxObservations, budgetBytes int) (string, int) {
	if maxObservations <= 0 {
		maxObservations = DigestMaxObservations
	}
	if budgetBytes <= 0 {
		budgetBytes = DigestByteBudget
	}
	ranked := RankObservationsForDigest(rows, now)
	if len(ranked) > maxObservations {
		ranked = ranked[:maxObservations]
	}
	var b strings.Builder
	count := 0
	for _, obs := range ranked {
		content := strings.TrimSpace(strings.Join(strings.Fields(obs.Content), " "))
		if content == "" {
			continue
		}
		line := "- [" + obs.Kind + "] " + content + RenderObservationEvolution(obs, digestEvolutionMaxEntries)
		extra := len(line)
		if count > 0 {
			extra++ // newline separator
		}
		if b.Len()+extra > budgetBytes {
			break
		}
		if count > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
		count++
	}
	return b.String(), count
}

// RecomputeAgentDigest rebuilds and upserts one agent's memory digest from
// its live observations.
func (s *Service) RecomputeAgentDigest(ctx context.Context, orgID, agentID uuid.UUID) error {
	if s == nil || s.cfg.DB == nil {
		return fmt.Errorf("memory service is not configured")
	}
	if orgID == uuid.Nil || agentID == uuid.Nil {
		return fmt.Errorf("org_id and agent_id are required")
	}

	now := time.Now().UTC()
	q := s.cfg.DB.WithContext(ctx).
		Where("org_id = ? AND archived_at IS NULL", orgID).
		Where("agent_id = ?", agentID).
		Where("(expires_at IS NULL OR expires_at > ?)", now)
	var rows []model.AgentObservation
	if err := q.Find(&rows).Error; err != nil {
		return fmt.Errorf("load observations for digest: %w", err)
	}

	content, count := RenderAgentDigest(rows, now, DigestMaxObservations, DigestByteBudget)
	digest := model.AgentMemoryDigest{
		AgentID:          agentID,
		OrgID:            orgID,
		Content:          content,
		ObservationCount: count,
		UpdatedAt:        now,
	}
	if err := s.cfg.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "agent_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"org_id", "content", "observation_count", "updated_at"}),
	}).Create(&digest).Error; err != nil {
		return fmt.Errorf("upsert agent memory digest: %w", err)
	}
	return nil
}
