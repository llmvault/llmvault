package tasks

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/memory"
	"github.com/usehivy/hivy/internal/model"
)

type staticConsolidationEmbedder struct{}

func (staticConsolidationEmbedder) Embed(_ context.Context, inputs []string) ([][]float32, int, error) {
	out := make([][]float32, len(inputs))
	for i := range inputs {
		vector := make([]float32, memory.DefaultEmbeddingDim)
		vector[0] = 1
		out[i] = vector
	}
	return out, 0, nil
}

// TestConsolidationConsumesAgentRetainedFacts proves the full pipeline for a
// legacy agent-retained fact. The retain_memory MCP tool has been removed
// (agents are read-only on memory), but historical facts with source
// "mcp_memory_tool" still exist and must keep consolidating: the fact is
// discovered by the stranded-facts sweep, consumed by a consolidation run,
// folded into a durable observation (the only layer recall injects and
// search reads), and stamped consolidated.
func TestConsolidationConsumesAgentRetainedFacts(t *testing.T) {
	ctx := context.Background()
	db := connectTestDB(t)

	org := model.Org{ID: uuid.New(), Name: "consolidate-" + uuid.NewString(), Active: true, RateLimit: 1000}
	team := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "consolidate-team-" + uuid.NewString()}
	agent := model.Agent{ID: uuid.New(), OrgID: &org.ID, TeamID: team.ID, Name: "Consolidate Agent " + uuid.NewString(), Model: "test", Status: "active"}
	channel := model.Channel{
		ID:             uuid.New(),
		OrgID:          org.ID,
		Name:           "consolidate-" + uuid.NewString(),
		Category:       "general",
		DefaultAgentID: agent.ID,
	}
	for _, seed := range []error{
		db.Create(&org).Error,
		db.Create(&team).Error,
		db.Create(&agent).Error,
		db.Create(&channel).Error,
	} {
		if seed != nil {
			t.Fatalf("seed fixtures: %v", seed)
		}
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM channel_memory_digests WHERE channel_id = ?`, channel.ID)
		db.Where("org_id = ?", org.ID).Delete(&model.AgentObservation{})
		db.Where("org_id = ?", org.ID).Delete(&model.AgentMemory{})
		db.Where("org_id = ?", org.ID).Delete(&model.Channel{})
		db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		db.Where("org_id = ?", org.ID).Delete(&model.Team{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})

	fact := model.AgentMemory{
		OrgID:     org.ID,
		ChannelID: &channel.ID,
		Content:   "ACME prefers invoices in EUR.",
		Metadata:  model.JSON{"source": "mcp_memory_tool", "agent_id": agent.ID.String()},
	}
	if err := db.Omit("Org", "Channel").Create(&fact).Error; err != nil {
		t.Fatalf("seed agent-retained fact: %v", err)
	}

	svc := memory.NewService(memory.Config{DB: db, Embedder: staticConsolidationEmbedder{}})

	// The stranded-facts sweep must discover the channel from an
	// agent-retained fact alone.
	channels, err := svc.ChannelsWithUnconsolidatedFacts(ctx, 200)
	if err != nil {
		t.Fatalf("scan unconsolidated channels: %v", err)
	}
	found := false
	for _, row := range channels {
		if row.OrgID == org.ID && row.ChannelID == channel.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("sweep must find channels holding mcp_memory_tool facts, got %#v", channels)
	}

	handler := &MemoryConsolidationHandler{
		db:        db,
		memorySvc: svc,
		memoryCfg: MemoryEmbeddingConfig{Model: memory.DefaultEmbeddingModel, Dim: memory.DefaultEmbeddingDim},
		now:       func() time.Time { return time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC) },
		complete: func(_ context.Context, _, userPrompt, _ string, _ json.RawMessage, _ int) (string, error) {
			// The retained fact must reach the consolidation prompt.
			if !strings.Contains(userPrompt, "ACME prefers invoices in EUR.") {
				t.Fatalf("consolidation prompt missing the agent-retained fact:\n%s", userPrompt)
			}
			return `{"creates":[{"text":"ACME prefers invoices in EUR.","kind":"preference","entities":["ACME"],"source_fact_ids":["0"],"scope":"channel","expires_at":"","reason":"agent-retained fact with no existing observation to merge into"}],"updates":[],"deletes":[]}`, nil
		},
	}

	task, _, err := NewMemoryConsolidateTask(MemoryConsolidatePayload{OrgID: org.ID, ChannelID: channel.ID})
	if err != nil {
		t.Fatalf("build task: %v", err)
	}
	if err := handler.Handle(ctx, task); err != nil {
		t.Fatalf("consolidation run: %v", err)
	}

	// The fact became a durable observation carrying its provenance.
	var obs model.AgentObservation
	if err := db.Where("org_id = ? AND channel_id = ?", org.ID, channel.ID).First(&obs).Error; err != nil {
		t.Fatalf("load consolidated observation: %v", err)
	}
	if obs.Content != "ACME prefers invoices in EUR." || obs.Kind != "preference" {
		t.Fatalf("observation = %q kind=%q, want the retained fact as a preference", obs.Content, obs.Kind)
	}
	if len(obs.SourceFactIDs) != 1 || obs.SourceFactIDs[0] != fact.ID.String() {
		t.Fatalf("observation source_fact_ids = %v, want the retained fact %s", obs.SourceFactIDs, fact.ID)
	}
	if obs.EmbeddingStatus != model.AgentMemoryEmbeddingReady {
		t.Fatalf("observation embedding_status = %q, want ready (synchronous embed)", obs.EmbeddingStatus)
	}

	// The fact left the consolidation queue but survives as provenance
	// (consolidated_at is stamped, never archived; the column lives only in
	// SQL, not on the model).
	var stored struct {
		ConsolidatedAt *time.Time
		ArchivedAt     *time.Time
	}
	if err := db.Raw(`SELECT consolidated_at, archived_at FROM agent_memories WHERE id = ?`, fact.ID).
		Scan(&stored).Error; err != nil {
		t.Fatalf("load fact: %v", err)
	}
	if stored.ConsolidatedAt == nil {
		t.Fatal("fact must be stamped consolidated_at after the run")
	}
	if stored.ArchivedAt != nil {
		t.Fatal("consolidation must not archive the source fact")
	}
}
