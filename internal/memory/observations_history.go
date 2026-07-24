package memory

import (
	"strings"
	"unicode/utf8"

	"github.com/usehivy/hivy/internal/model"
)

const (
	// digestEvolutionMaxEntries caps how many superseded wordings one digest
	// bullet may carry: enough to show how the fact evolved without letting
	// stale text crowd out current knowledge in the injected prompt.
	digestEvolutionMaxEntries = 2
	// evolutionContentMaxBytes clamps one superseded wording inside an
	// injected evolution note.
	evolutionContentMaxBytes = 200
)

// ObservationHistoryEntry is one recorded rewrite of an observation's
// content: the wording it replaced, when the rewrite happened, and the
// recorded reason. Sourced from metadata.audit entries (consolidation
// update/merge and human correct ops record previous_content).
type ObservationHistoryEntry struct {
	At              string `json:"at"`
	PreviousContent string `json:"previous_content"`
	Reason          string `json:"reason,omitempty"`
}

// ObservationHistory returns the observation's content rewrites, newest
// first, capped at limit (0 = all). Audit entries without previous_content
// (evidence-only updates, archives, creates) are not history.
func ObservationHistory(obs model.AgentObservation, limit int) []ObservationHistoryEntry {
	raw, _ := obs.Metadata["audit"].([]any)
	entries := make([]ObservationHistoryEntry, 0, len(raw))
	for _, item := range raw {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		previous, _ := entry["previous_content"].(string)
		previous = strings.TrimSpace(previous)
		if previous == "" {
			continue
		}
		at, _ := entry["at"].(string)
		reason, _ := entry["reason"].(string)
		entries = append(entries, ObservationHistoryEntry{
			At:              strings.TrimSpace(at),
			PreviousContent: previous,
			Reason:          strings.TrimSpace(reason),
		})
	}
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	return entries
}

// RenderObservationEvolution renders a compact evolution note for prompt
// injection: the memory's most recent superseded wordings, newest first,
// explicitly marked as former wordings so models never mistake them for
// current facts. Returns "" when the content never changed.
//
// Example: (evolved — was "Production hosting runs on Vercel." until
// 2026-06-20)
func RenderObservationEvolution(obs model.AgentObservation, maxEntries int) string {
	history := ObservationHistory(obs, maxEntries)
	if len(history) == 0 {
		return ""
	}
	parts := make([]string, 0, len(history))
	for _, entry := range history {
		part := `"` + clampEvolutionText(entry.PreviousContent) + `"`
		if date := evolutionDate(entry.At); date != "" {
			part += " until " + date
		}
		parts = append(parts, part)
	}
	return " (evolved — was " + strings.Join(parts, "; earlier ") + ")"
}

// evolutionDate reduces an RFC3339 audit timestamp to its date part.
func evolutionDate(at string) string {
	if len(at) >= 10 {
		return at[:10]
	}
	return at
}

// clampEvolutionText collapses whitespace and clamps one superseded wording
// to evolutionContentMaxBytes on a rune boundary.
func clampEvolutionText(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= evolutionContentMaxBytes {
		return value
	}
	cut := evolutionContentMaxBytes
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return strings.TrimSpace(value[:cut]) + "..."
}
