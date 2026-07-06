package memory

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func searchHit(kind string, proof int, ageDays int, similarity float64, now time.Time) ObservationHit {
	return ObservationHit{
		Observation: model.AgentObservation{
			ID:              uuid.New(),
			Kind:            kind,
			ProofCount:      proof,
			Content:         "content",
			LastMentionedAt: now.AddDate(0, 0, -ageDays),
		},
		Similarity: similarity,
	}
}

func TestFilterAndRankSearchHitsDropsWeakMatches(t *testing.T) {
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	strong := searchHit("finding", 1, 1, 0.62, now)
	borderline := searchHit("finding", 1, 1, SearchMinSimilarity, now)
	weak := searchHit("rule", 50, 1, SearchMinSimilarity-0.01, now)

	out := filterAndRankSearchHits([]ObservationHit{weak, borderline, strong}, now, 10)
	if len(out) != 2 {
		t.Fatalf("kept %d hits, want 2 (cutoff at %v)", len(out), SearchMinSimilarity)
	}
	for _, hit := range out {
		if hit.Observation.ID == weak.Observation.ID {
			t.Fatalf("hit below the similarity cutoff must never reach the agent, even a strong rule")
		}
	}
	if out[0].Observation.ID != strong.Observation.ID {
		t.Fatalf("higher-similarity hit must rank first among equal-strength hits")
	}
}

func TestFilterAndRankSearchHitsBlendsStrengthIntoNearTies(t *testing.T) {
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	// Near-tie: a fresh, repeatedly confirmed rule at slightly lower cosine
	// similarity must outrank a one-off finding.
	provenRule := searchHit("rule", 5, 1, 0.60, now)
	oneOffFinding := searchHit("finding", 1, 1, 0.65, now)
	out := filterAndRankSearchHits([]ObservationHit{oneOffFinding, provenRule}, now, 10)
	if out[0].Observation.ID != provenRule.Observation.ID {
		t.Fatalf("proven rule (sim %.2f, score %.3f) should win the near-tie over one-off finding (sim %.2f, score %.3f)",
			provenRule.Similarity, SearchScore(provenRule, now),
			oneOffFinding.Similarity, SearchScore(oneOffFinding, now))
	}

	// Not a tie: a clearly better semantic match wins no matter how strong the
	// weaker memory is — similarity dominates the blend.
	clearWinner := searchHit("finding", 1, 1, 0.90, now)
	out = filterAndRankSearchHits([]ObservationHit{provenRule, clearWinner}, now, 10)
	if out[0].Observation.ID != clearWinner.Observation.ID {
		t.Fatalf("clear semantic winner (sim %.2f) must outrank the proven rule (sim %.2f): strength is a tie-breaker, not a takeover",
			clearWinner.Similarity, provenRule.Similarity)
	}
}

func TestFilterAndRankSearchHitsAppliesLimitAfterRanking(t *testing.T) {
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	best := searchHit("rule", 5, 1, 0.80, now)
	hits := []ObservationHit{
		searchHit("finding", 1, 1, 0.50, now),
		searchHit("finding", 1, 1, 0.55, now),
		best,
		searchHit("finding", 1, 1, 0.20, now), // below cutoff
	}
	out := filterAndRankSearchHits(hits, now, 2)
	if len(out) != 2 {
		t.Fatalf("len = %d, want limit 2", len(out))
	}
	if out[0].Observation.ID != best.Observation.ID {
		t.Fatalf("limit must apply after ranking so the best hit survives")
	}
}
