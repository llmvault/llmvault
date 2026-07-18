package tasks

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

type consolidationOps struct {
	Creates []consolidationCreate `json:"creates"`
	Updates []consolidationUpdate `json:"updates"`
	Deletes []consolidationDelete `json:"deletes"`
}

type consolidationCreate struct {
	Text          string   `json:"text"`
	Kind          string   `json:"kind"`
	Entities      []string `json:"entities"`
	SourceFactIDs []string `json:"source_fact_ids"`
	ExpiresAt     string   `json:"expires_at"`
	Reason        string   `json:"reason"`
}

type consolidationUpdate struct {
	ObservationID string   `json:"observation_id"`
	Text          string   `json:"text"`
	SourceFactIDs []string `json:"source_fact_ids"`
	Reason        string   `json:"reason"`
}

type consolidationDelete struct {
	ObservationID string `json:"observation_id"`
	Reason        string `json:"reason"`
}

type consolidationDedupDecision struct {
	Action string `json:"action"`
	Text   string `json:"text"`
	Reason string `json:"reason"`
}

// consolidationIDMap maps real UUIDs to small integer strings ("0", "1", ...)
// before anything reaches the LLM, and back afterwards — the mem0
// anti-hallucination pattern. Ops referencing unknown integer ids are
// rejected by the caller.
type consolidationIDMap struct {
	byUUID map[uuid.UUID]string
	byKey  map[string]uuid.UUID
	next   int
}

func newConsolidationIDMap() *consolidationIDMap {
	return &consolidationIDMap{
		byUUID: map[uuid.UUID]string{},
		byKey:  map[string]uuid.UUID{},
	}
}

// keyFor returns the stable integer key for id, assigning the next integer on
// first sight.
func (m *consolidationIDMap) keyFor(id uuid.UUID) string {
	if key, ok := m.byUUID[id]; ok {
		return key
	}
	key := strconv.Itoa(m.next)
	m.next++
	m.byUUID[id] = key
	m.byKey[key] = id
	return key
}

// uuidFor resolves an integer key back to its UUID.
func (m *consolidationIDMap) uuidFor(key string) (uuid.UUID, bool) {
	id, ok := m.byKey[strings.TrimSpace(key)]
	return id, ok
}

// resolvedConsolidationOps carries ops with integer ids mapped back to UUIDs;
// ops that referenced unknown ids were dropped and counted.
type resolvedConsolidationOps struct {
	Creates []resolvedCreate
	Updates []resolvedUpdate
	Deletes []resolvedDelete
	Skipped int
}

type resolvedCreate struct {
	Op            consolidationCreate
	SourceFactIDs []uuid.UUID
}

type resolvedUpdate struct {
	Op            consolidationUpdate
	ObservationID uuid.UUID
	SourceFactIDs []uuid.UUID
}

type resolvedDelete struct {
	Op            consolidationDelete
	ObservationID uuid.UUID
}

// resolveConsolidationOps maps integer ids back to UUIDs, dropping any op
// that references an id the mapper never issued. Creates additionally require
// at least one valid source fact and a non-empty reason.
func resolveConsolidationOps(ops consolidationOps, factIDs, observationIDs *consolidationIDMap) resolvedConsolidationOps {
	var out resolvedConsolidationOps
	for _, op := range ops.Creates {
		facts, ok := resolveFactKeys(op.SourceFactIDs, factIDs)
		if !ok || len(facts) == 0 || strings.TrimSpace(op.Reason) == "" || strings.TrimSpace(op.Text) == "" {
			out.Skipped++
			continue
		}
		out.Creates = append(out.Creates, resolvedCreate{Op: op, SourceFactIDs: facts})
	}
	for _, op := range ops.Updates {
		obsID, ok := observationIDs.uuidFor(op.ObservationID)
		if !ok || strings.TrimSpace(op.Reason) == "" {
			out.Skipped++
			continue
		}
		facts, ok := resolveFactKeys(op.SourceFactIDs, factIDs)
		if !ok {
			out.Skipped++
			continue
		}
		out.Updates = append(out.Updates, resolvedUpdate{Op: op, ObservationID: obsID, SourceFactIDs: facts})
	}
	for _, op := range ops.Deletes {
		obsID, ok := observationIDs.uuidFor(op.ObservationID)
		if !ok || strings.TrimSpace(op.Reason) == "" {
			out.Skipped++
			continue
		}
		out.Deletes = append(out.Deletes, resolvedDelete{Op: op, ObservationID: obsID})
	}
	return out
}

func resolveFactKeys(keys []string, factIDs *consolidationIDMap) ([]uuid.UUID, bool) {
	out := make([]uuid.UUID, 0, len(keys))
	seen := map[uuid.UUID]bool{}
	for _, key := range keys {
		id, ok := factIDs.uuidFor(key)
		if !ok {
			return nil, false
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, true
}

// buildConsolidationUserPrompt renders the per-batch input: facts as
// integer-id lines and pooled observations as a JSON array.
func buildConsolidationUserPrompt(
	facts []model.AgentMemory,
	observations []model.AgentObservation,
	factIDs, observationIDs *consolidationIDMap,
) (string, error) {
	var b strings.Builder
	b.WriteString("## INPUT\n\n### New facts\n\n")
	for _, fact := range facts {
		key := factIDs.keyFor(fact.ID)
		kind, _ := fact.Metadata["kind"].(string)
		actor, _ := fact.Metadata["actor_display_name"].(string)
		human := factFromHumanActor(fact)
		line := fmt.Sprintf("[%s] %s (kind=%s, mentioned_at=%s, actor=%s, human=%t)",
			key,
			strings.Join(strings.Fields(fact.Content), " "),
			emptyMarker(kind),
			fact.CreatedAt.UTC().Format("2006-01-02"),
			emptyMarker(actor),
			human,
		)
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString("\n### Existing observations\n\n")
	entries := make([]map[string]any, 0, len(observations))
	for _, obs := range observations {
		entries = append(entries, map[string]any{
			"id":                observationIDs.keyFor(obs.ID),
			"text":              obs.Content,
			"kind":              obs.Kind,
			"proof_count":       obs.ProofCount,
			"last_mentioned_at": obs.LastMentionedAt.UTC().Format("2006-01-02"),
		})
	}
	encoded, err := json.Marshal(entries)
	if err != nil {
		return "", fmt.Errorf("marshal observations for prompt: %w", err)
	}
	b.Write(encoded)
	return b.String(), nil
}

func parseConsolidationResponse(raw string) (consolidationOps, error) {
	var ops consolidationOps
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &ops); err != nil {
		return consolidationOps{}, fmt.Errorf("decode consolidation json: %w", err)
	}
	return ops, nil
}

func parseConsolidationDedupResponse(raw string) (consolidationDedupDecision, error) {
	var decision consolidationDedupDecision
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &decision); err != nil {
		return consolidationDedupDecision{}, fmt.Errorf("decode dedup json: %w", err)
	}
	return decision, nil
}

// parseConsolidationExpiresAt accepts an ISO date or RFC3339 timestamp;
// empty or unparseable values mean "no expiry".
func parseConsolidationExpiresAt(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			parsed = parsed.UTC()
			return &parsed
		}
	}
	return nil
}

// sortedUUIDStrings gives deterministic audit entries.
func sortedUUIDStrings(ids []uuid.UUID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	sort.Strings(out)
	return out
}
