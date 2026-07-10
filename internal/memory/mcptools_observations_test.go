package memory

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// requireMemoryTables skips the test until the named tables exist. The
// observations/digests/directives tables land in migrations 000074/000075;
// these tests activate automatically once those are applied to the test DB.
func requireMemoryTables(t *testing.T, db *gorm.DB, tables ...string) {
	t.Helper()
	for _, table := range tables {
		var exists bool
		if err := db.Raw(`SELECT to_regclass(?) IS NOT NULL`, table).Scan(&exists).Error; err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if !exists {
			t.Skipf("table %s not migrated yet; test activates once the migration lands", table)
		}
	}
}

type observationSeed struct {
	channelID     *uuid.UUID
	content       string
	kind          string
	entities      []string
	proofCount    int
	lastMentioned time.Time
	expiresAt     *time.Time
	archivedAt    *time.Time
	embedded      bool
}

func seedObservation(t *testing.T, db *gorm.DB, orgID uuid.UUID, seed observationSeed) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if seed.proofCount <= 0 {
		seed.proofCount = 1
	}
	if seed.lastMentioned.IsZero() {
		seed.lastMentioned = time.Now()
	}
	if seed.entities == nil {
		seed.entities = []string{}
	}
	var embedding any
	embeddingStatus := model.AgentMemoryEmbeddingPending
	embeddingModel := ""
	if seed.embedded {
		embedding = vectorLiteral(testMemoryVector())
		embeddingStatus = model.AgentMemoryEmbeddingReady
		embeddingModel = DefaultEmbeddingModel
	}
	if err := db.Exec(`
INSERT INTO agent_observations
  (id, org_id, channel_id, content, kind, entities, proof_count, last_mentioned_at, expires_at, archived_at,
   embedding, embedding_status, embedding_model)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?::vector, ?, ?)`,
		id, orgID, seed.channelID, seed.content, seed.kind, pq.StringArray(seed.entities),
		seed.proofCount, seed.lastMentioned, seed.expiresAt, seed.archivedAt,
		embedding, embeddingStatus, embeddingModel,
	).Error; err != nil {
		t.Fatalf("seed observation: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM agent_observations WHERE id = ?`, id) })
	return id
}

func TestManageMemoriesSearchObservations(t *testing.T) {
	ctx := context.Background()
	db := connectMemoryToolTestDB(t)
	requireMemoryTables(t, db, "agent_observations")
	fixture := seedMemoryToolFixture(t, db)
	service := NewService(Config{DB: db, Embedder: staticMemoryToolEmbedder{vector: testMemoryVector()}})
	_, defaultToken := createManageAgent(t, db, fixture.org.ID, "Manage Observations", true)
	client := connectMemoryToolClient(t, ctx, service, defaultToken)
	actorID := createManageManager(t, db, fixture.org.ID)

	channelObs := seedObservation(t, db, fixture.org.ID, observationSeed{
		channelID: &fixture.channel.ID, content: "Channel-level convention: PRs need one approval.", kind: "convention", embedded: true,
	})
	orgObs := seedObservation(t, db, fixture.org.ID, observationSeed{
		content: "Org-wide: escalate outages within 15 minutes.", kind: "rule", embedded: true,
	})

	all := callMemoryTool(t, ctx, client, "manage_memories", map[string]any{"action": "search", "query": "conventions and rules", "_hivy_actor_user_id": actorID})
	if all["layer"] != memoryLayerObservations {
		t.Fatalf("manage search layer = %v, want %q", all["layer"], memoryLayerObservations)
	}
	ids := resultIDSet(all)
	if len(ids) != 2 || !ids[channelObs.String()] || !ids[orgObs.String()] {
		t.Fatalf("manage observation search = %#v, want both observations", all)
	}
	for _, raw := range all["results"].([]any) {
		item := raw.(map[string]any)
		switch item["id"] {
		case channelObs.String():
			if item["org_wide"] != false || item["channel_name"] != fixture.channel.Name {
				t.Fatalf("channel observation annotation mismatch: %#v", item)
			}
		case orgObs.String():
			if item["org_wide"] != true || item["channel_name"] != nil {
				t.Fatalf("org-wide observation annotation mismatch: %#v", item)
			}
		}
	}
}

func TestTopObservationsRankingAndScope(t *testing.T) {
	ctx := context.Background()
	db := connectMemoryToolTestDB(t)
	requireMemoryTables(t, db, "agent_observations")
	fixture := seedMemoryToolFixture(t, db)
	service := NewService(Config{DB: db})

	now := time.Now()
	strong := seedObservation(t, db, fixture.org.ID, observationSeed{
		channelID: &fixture.channel.ID, content: "Strong observation.", kind: "rule", proofCount: 5, lastMentioned: now.Add(-time.Hour),
	})
	recent := seedObservation(t, db, fixture.org.ID, observationSeed{
		channelID: &fixture.channel.ID, content: "Recent observation.", kind: "finding", proofCount: 2, lastMentioned: now,
	})
	older := seedObservation(t, db, fixture.org.ID, observationSeed{
		channelID: &fixture.channel.ID, content: "Older observation.", kind: "finding", proofCount: 2, lastMentioned: now.Add(-2 * time.Hour),
	})
	orgWide := seedObservation(t, db, fixture.org.ID, observationSeed{
		content: "Org-wide observation.", kind: "org-fact", proofCount: 1,
	})

	channelID := fixture.channel.ID
	rows, err := service.TopObservations(ctx, fixture.org.ID, ChannelScope{ChannelID: &channelID, IncludeOrgMemories: true}, 25)
	if err != nil {
		t.Fatalf("TopObservations: %v", err)
	}
	if len(rows) != 4 || rows[0].ID != strong || rows[1].ID != recent || rows[2].ID != older || rows[3].ID != orgWide {
		got := make([]string, 0, len(rows))
		for _, row := range rows {
			got = append(got, row.Content)
		}
		t.Fatalf("TopObservations order = %v, want proof_count DESC then last_mentioned_at DESC", got)
	}

	scoped, err := service.TopObservations(ctx, fixture.org.ID, ChannelScope{ChannelID: &channelID}, 25)
	if err != nil {
		t.Fatalf("TopObservations channel-only: %v", err)
	}
	for _, row := range scoped {
		if row.ChannelID == nil {
			t.Fatalf("channel-only scope leaked org-wide observation: %#v", row)
		}
	}
}
