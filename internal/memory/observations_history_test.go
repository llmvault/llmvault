package memory

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func evolvedObservation(kind, content string, audit []any) model.AgentObservation {
	return model.AgentObservation{
		ID:              uuid.New(),
		Kind:            kind,
		Content:         content,
		ProofCount:      2,
		LastMentionedAt: time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC),
		Metadata:        model.JSON{"audit": audit},
	}
}

func TestObservationHistoryExtractsRewritesNewestFirst(t *testing.T) {
	obs := evolvedObservation("decision", "Production runs on Railway.", []any{
		map[string]any{"op": "create", "reason": "new decision", "at": "2026-01-10T00:00:00Z"},
		map[string]any{"op": "update", "reason": "evidence only", "at": "2026-02-01T00:00:00Z"},
		map[string]any{"op": "update", "reason": "vendor named", "at": "2026-03-05T00:00:00Z", "previous_content": "Production runs on the new hosting vendor."},
		map[string]any{"op": "update", "reason": "state change", "at": "2026-06-20T00:00:00Z", "previous_content": "Production hosting runs on Vercel."},
	})

	history := ObservationHistory(obs, 0)
	if len(history) != 2 {
		t.Fatalf("history entries = %d, want 2 (only ops with previous_content)", len(history))
	}
	if history[0].PreviousContent != "Production hosting runs on Vercel." || history[0].At != "2026-06-20T00:00:00Z" {
		t.Fatalf("history[0] = %+v, want the newest rewrite first", history[0])
	}
	if history[1].PreviousContent != "Production runs on the new hosting vendor." {
		t.Fatalf("history[1] = %+v, want the older rewrite second", history[1])
	}

	capped := ObservationHistory(obs, 1)
	if len(capped) != 1 || capped[0].At != "2026-06-20T00:00:00Z" {
		t.Fatalf("capped history = %+v, want only the newest rewrite", capped)
	}

	if got := ObservationHistory(model.AgentObservation{}, 3); len(got) != 0 {
		t.Fatalf("no-metadata observation history = %+v, want empty", got)
	}
}

func TestRenderObservationEvolution(t *testing.T) {
	obs := evolvedObservation("decision", "Production runs on Railway.", []any{
		map[string]any{"op": "update", "at": "2026-03-05T00:00:00Z", "previous_content": "Production runs on the new hosting vendor."},
		map[string]any{"op": "update", "at": "2026-06-20T00:00:00Z", "previous_content": "Production hosting runs on Vercel."},
	})

	note := RenderObservationEvolution(obs, 2)
	want := ` (evolved — was "Production hosting runs on Vercel." until 2026-06-20; earlier "Production runs on the new hosting vendor." until 2026-03-05)`
	if note != want {
		t.Fatalf("evolution note = %q, want %q", note, want)
	}

	if note := RenderObservationEvolution(model.AgentObservation{}, 2); note != "" {
		t.Fatalf("never-rewritten observation note = %q, want empty", note)
	}

	// Oversized superseded wordings clamp on whole runes with an ellipsis.
	long := evolvedObservation("finding", "Current.", []any{
		map[string]any{"op": "update", "at": "2026-06-20T00:00:00Z", "previous_content": strings.Repeat("x", 500)},
	})
	note = RenderObservationEvolution(long, 1)
	if !strings.Contains(note, "...") || len(note) > evolutionContentMaxBytes+64 {
		t.Fatalf("long previous wording must be clamped, got %d bytes: %q", len(note), note[:80])
	}
}

func TestRenderAgentDigestIncludesEvolution(t *testing.T) {
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	evolved := evolvedObservation("decision", "Production runs on Railway.", []any{
		map[string]any{"op": "update", "at": "2026-06-20T00:00:00Z", "previous_content": "Production hosting runs on Vercel."},
	})
	plain := model.AgentObservation{
		ID: uuid.New(), Kind: "finding", Content: "Qdrant restart clears stale aliases.",
		ProofCount: 1, LastMentionedAt: now,
	}

	digest, count := RenderAgentDigest([]model.AgentObservation{evolved, plain}, now, 10, DigestByteBudget)
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
	if !strings.Contains(digest, `Production runs on Railway. (evolved — was "Production hosting runs on Vercel." until 2026-06-20)`) {
		t.Fatalf("digest missing evolution note: %q", digest)
	}
	for _, line := range strings.Split(digest, "\n") {
		if strings.Contains(line, "Qdrant restart") && strings.Contains(line, "evolved") {
			t.Fatalf("never-rewritten observation must carry no evolution note: %q", line)
		}
	}
}
