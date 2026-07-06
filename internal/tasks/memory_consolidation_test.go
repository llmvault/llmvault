package tasks

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/usehivy/hivy/internal/memory"
	"github.com/usehivy/hivy/internal/model"
)

func TestConsolidationIDMapRoundTrip(t *testing.T) {
	m := newConsolidationIDMap()
	first := uuid.New()
	second := uuid.New()

	if key := m.keyFor(first); key != "0" {
		t.Fatalf("first key = %q, want 0", key)
	}
	if key := m.keyFor(second); key != "1" {
		t.Fatalf("second key = %q, want 1", key)
	}
	if key := m.keyFor(first); key != "0" {
		t.Fatalf("repeat key = %q, want stable 0", key)
	}

	got, ok := m.uuidFor("1")
	if !ok || got != second {
		t.Fatalf("uuidFor(1) = %v %v, want %v true", got, ok, second)
	}
	if _, ok := m.uuidFor("99"); ok {
		t.Fatal("uuidFor(99) should be unknown")
	}
	if _, ok := m.uuidFor(second.String()); ok {
		t.Fatal("raw UUID strings must not resolve — only issued integer keys")
	}
}

func TestResolveConsolidationOpsRejectsUnknownIDs(t *testing.T) {
	factIDs := newConsolidationIDMap()
	obsIDs := newConsolidationIDMap()
	factA := uuid.New()
	obsA := uuid.New()
	factKey := factIDs.keyFor(factA) // "0"
	obsKey := obsIDs.keyFor(obsA)    // "0"

	ops := consolidationOps{
		Creates: []consolidationCreate{
			{Text: "valid", Kind: "decision", SourceFactIDs: []string{factKey}, Scope: "channel", Reason: "r"},
			{Text: "bad fact id", Kind: "decision", SourceFactIDs: []string{"7"}, Scope: "channel", Reason: "r"},
			{Text: "no reason", Kind: "decision", SourceFactIDs: []string{factKey}, Scope: "channel"},
		},
		Updates: []consolidationUpdate{
			{ObservationID: obsKey, Text: "valid", SourceFactIDs: []string{factKey}, Reason: "r"},
			{ObservationID: "42", Text: "unknown obs", Reason: "r"},
			{ObservationID: obsKey, Text: "unknown fact", SourceFactIDs: []string{"9"}, Reason: "r"},
		},
		Deletes: []consolidationDelete{
			{ObservationID: obsKey, Reason: "r"},
			{ObservationID: "13", Reason: "r"},
		},
	}
	resolved := resolveConsolidationOps(ops, factIDs, obsIDs)

	if len(resolved.Creates) != 1 || resolved.Creates[0].SourceFactIDs[0] != factA {
		t.Fatalf("creates = %+v, want exactly the valid one mapped to %v", resolved.Creates, factA)
	}
	if len(resolved.Updates) != 1 || resolved.Updates[0].ObservationID != obsA {
		t.Fatalf("updates = %+v, want exactly the valid one mapped to %v", resolved.Updates, obsA)
	}
	if len(resolved.Deletes) != 1 || resolved.Deletes[0].ObservationID != obsA {
		t.Fatalf("deletes = %+v, want exactly the valid one mapped to %v", resolved.Deletes, obsA)
	}
	if resolved.Skipped != 5 {
		t.Fatalf("skipped = %d, want 5", resolved.Skipped)
	}
}

func TestConsolidationCreateChannelIDPromotionGate(t *testing.T) {
	channelID := uuid.New()
	tests := []struct {
		name        string
		scope       string
		proofCount  int
		humanSource bool
		wantOrgWide bool
	}{
		{name: "channel scope stays channel", scope: "channel", proofCount: 5, humanSource: true, wantOrgWide: false},
		{name: "org scope single machine fact stays channel", scope: "org", proofCount: 1, humanSource: false, wantOrgWide: false},
		{name: "org scope with proof>=2 promotes", scope: "org", proofCount: 2, humanSource: false, wantOrgWide: true},
		{name: "org scope with human statement promotes", scope: "org", proofCount: 1, humanSource: true, wantOrgWide: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := consolidationCreateChannelID(tt.scope, channelID, tt.proofCount, tt.humanSource)
			if tt.wantOrgWide {
				if got != nil {
					t.Fatalf("channel id = %v, want nil (org-wide)", got)
				}
				return
			}
			if got == nil || *got != channelID {
				t.Fatalf("channel id = %v, want %v", got, channelID)
			}
		})
	}
}

func TestFactFromHumanActor(t *testing.T) {
	human := model.AgentMemory{Metadata: model.JSON{"actor_display_name": "Priya"}}
	humanRef := model.AgentMemory{Metadata: model.JSON{"actor_external_ref": "U123"}}
	agent := model.AgentMemory{Metadata: model.JSON{"actor_display_name": "  "}}
	if !factFromHumanActor(human) || !factFromHumanActor(humanRef) {
		t.Fatal("facts with actor attribution must count as human statements")
	}
	if factFromHumanActor(agent) {
		t.Fatal("blank actor fields must not count as human statements")
	}
}

func TestApplyConsolidationUpdateHumanVerifiedProtection(t *testing.T) {
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	existingFact := uuid.New()
	newFact := uuid.New()

	verified := &model.AgentObservation{
		Content:       "Human-approved wording.",
		HumanVerified: true,
		ProofCount:    3,
		SourceFactIDs: pq.StringArray{existingFact.String()},
	}
	changed := applyConsolidationUpdate(verified, "LLM rewrite attempt", "restated", []uuid.UUID{newFact}, now)
	if changed {
		t.Fatal("human_verified content must never be rewritten")
	}
	if verified.Content != "Human-approved wording." {
		t.Fatalf("content = %q, want protected original", verified.Content)
	}
	if verified.ProofCount != 4 {
		t.Fatalf("proof_count = %d, want 4 (evidence still appended)", verified.ProofCount)
	}
	if len(verified.SourceFactIDs) != 2 || verified.SourceFactIDs[1] != newFact.String() {
		t.Fatalf("source_fact_ids = %v, want new fact appended", verified.SourceFactIDs)
	}
	if !verified.LastMentionedAt.Equal(now) {
		t.Fatalf("last_mentioned_at = %v, want refreshed to %v", verified.LastMentionedAt, now)
	}
	audit, _ := verified.Metadata["audit"].([]any)
	if len(audit) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(audit))
	}
	entry, _ := audit[0].(map[string]any)
	if entry["op"] != "update" || entry["reason"] != "restated" {
		t.Fatalf("audit entry = %v, want op=update reason=restated", entry)
	}

	unverified := &model.AgentObservation{
		Content:       "Old wording.",
		ProofCount:    1,
		SourceFactIDs: pq.StringArray{existingFact.String()},
	}
	changed = applyConsolidationUpdate(unverified, "New wording with dates.", "state change", []uuid.UUID{existingFact}, now)
	if !changed || unverified.Content != "New wording with dates." {
		t.Fatalf("unverified content should be rewritten, got changed=%v content=%q", changed, unverified.Content)
	}
	if unverified.ProofCount != 2 {
		t.Fatalf("proof_count = %d, want 2 (re-mention of known fact still bumps)", unverified.ProofCount)
	}
	if len(unverified.SourceFactIDs) != 1 {
		t.Fatalf("source_fact_ids = %v, duplicate fact must not be appended twice", unverified.SourceFactIDs)
	}
}

func TestSuppressionFingerprintNormalization(t *testing.T) {
	a := memory.SuppressionFingerprint("  Deploys   run on\tRailway. ")
	b := memory.SuppressionFingerprint("deploys run on railway.")
	if a != b {
		t.Fatalf("fingerprints differ across whitespace/case: %q vs %q", a, b)
	}
	sum := sha256.Sum256([]byte("deploys run on railway."))
	if want := hex.EncodeToString(sum[:]); a != want {
		t.Fatalf("fingerprint = %q, want sha256 of normalized content %q", a, want)
	}
	if a == memory.SuppressionFingerprint("deploys run on vercel.") {
		t.Fatal("different content must not collide")
	}
}

func TestDigestRankingAndRendering(t *testing.T) {
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	obs := func(kind string, proof int, ageDays int, content string) model.AgentObservation {
		return model.AgentObservation{
			ID:              uuid.New(),
			Kind:            kind,
			ProofCount:      proof,
			Content:         content,
			LastMentionedAt: now.AddDate(0, 0, -ageDays),
		}
	}
	rule := obs("rule", 3, 1, "Migrations need a second reviewer.")
	finding := obs("finding", 3, 1, "Qdrant restart clears stale aliases.")
	decision := obs("decision", 3, 1, "Chose Railway over Fly for previews.")
	staleRule := obs("rule", 3, 720, "Old CI rule mentioned two years ago.")

	rows := []model.AgentObservation{finding, staleRule, decision, rule}
	ranked := memory.RankObservationsForDigest(rows, now)
	if ranked[0].ID != rule.ID {
		t.Fatalf("top ranked = %s, want the fresh rule", ranked[0].Content)
	}
	if ranked[1].ID != decision.ID {
		t.Fatalf("second ranked = %s, want the decision", ranked[1].Content)
	}
	if ranked[len(ranked)-1].ID != finding.ID && ranked[len(ranked)-1].ID != staleRule.ID {
		t.Fatalf("lowest ranked = %s, want finding or decayed rule", ranked[len(ranked)-1].Content)
	}

	content, count := memory.RenderChannelDigest(rows, now, memory.DigestMaxObservations, memory.DigestByteBudget)
	if count != 4 {
		t.Fatalf("count = %d, want 4", count)
	}
	lines := strings.Split(content, "\n")
	if lines[0] != "- [rule] Migrations need a second reviewer." {
		t.Fatalf("first line = %q, want `- [kind] content` format with the rule first", lines[0])
	}

	// K cap: the default cap is 250 observations, so 260 short rows (well
	// under the byte budget) must render exactly K lines.
	many := make([]model.AgentObservation, 0, memory.DigestMaxObservations+10)
	for i := 0; i < memory.DigestMaxObservations+10; i++ {
		many = append(many, obs("finding", 1, 1, "Observation number filler content."))
	}
	_, count = memory.RenderChannelDigest(many, now, memory.DigestMaxObservations, memory.DigestByteBudget)
	if count != memory.DigestMaxObservations {
		t.Fatalf("count = %d, want K=%d", count, memory.DigestMaxObservations)
	}

	// Byte budget trims whole lines.
	long := []model.AgentObservation{
		obs("rule", 5, 1, strings.Repeat("a", 100)),
		obs("rule", 4, 1, strings.Repeat("b", 100)),
		obs("rule", 3, 1, strings.Repeat("c", 100)),
	}
	content, count = memory.RenderChannelDigest(long, now, memory.DigestMaxObservations, 250)
	if count != 2 {
		t.Fatalf("count = %d, want 2 lines within a 250-byte budget", count)
	}
	if len(content) > 250 {
		t.Fatalf("content = %d bytes, want <= 250", len(content))
	}
	for _, line := range strings.Split(content, "\n") {
		if !strings.HasPrefix(line, "- [rule] ") {
			t.Fatalf("line %q lost its format — trimming must drop whole lines only", line)
		}
	}
}

func TestParseConsolidationExpiresAt(t *testing.T) {
	if got := parseConsolidationExpiresAt(""); got != nil {
		t.Fatalf("empty expires_at = %v, want nil", got)
	}
	got := parseConsolidationExpiresAt("2026-09-01")
	if got == nil || got.Format("2006-01-02") != "2026-09-01" {
		t.Fatalf("ISO date parsed as %v", got)
	}
	if got := parseConsolidationExpiresAt("not-a-date"); got != nil {
		t.Fatalf("garbage expires_at = %v, want nil", got)
	}
}
