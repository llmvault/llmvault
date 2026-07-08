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

func TestSearchMemoriesObservationsLayer(t *testing.T) {
	ctx := context.Background()
	db := connectMemoryToolTestDB(t)
	requireMemoryTables(t, db, "agent_observations")
	fixture := seedMemoryToolFixture(t, db)
	service := NewService(Config{DB: db, Embedder: staticMemoryToolEmbedder{vector: testMemoryVector()}})
	client := connectMemoryToolClient(t, ctx, service, fixture.token)

	otherChannel := model.Channel{ID: uuid.New(), OrgID: fixture.org.ID, Name: "memory-obs-other-" + uuid.NewString(), DefaultAgentID: fixture.agent.ID, ExposeOrgMemories: true}
	if err := db.Create(&otherChannel).Error; err != nil {
		t.Fatalf("create other channel: %v", err)
	}
	t.Cleanup(func() { db.Delete(&model.Channel{}, "id = ?", otherChannel.ID) })

	now := time.Now()
	past := now.Add(-time.Hour)
	channelObs := seedObservation(t, db, fixture.org.ID, observationSeed{
		channelID: &fixture.channel.ID, content: "The team chose Railway over Fly for deploys.",
		kind: "decision", entities: []string{"Railway"}, proofCount: 3, embedded: true,
	})
	// A recorded rewrite: search hits carry their evolution history along.
	if err := db.Exec(`
UPDATE agent_observations
SET metadata = '{"audit":[{"op":"update","reason":"state change","at":"2026-06-20T00:00:00Z","previous_content":"The team deploys on Fly."}]}'::jsonb
WHERE id = ?`, channelObs).Error; err != nil {
		t.Fatalf("seed observation history: %v", err)
	}
	orgObs := seedObservation(t, db, fixture.org.ID, observationSeed{
		content: "Org billing policy is Net 30.", kind: "org-fact", embedded: true,
	})
	leakObs := seedObservation(t, db, fixture.org.ID, observationSeed{
		channelID: &otherChannel.ID, content: "Other channel's private observation.", kind: "finding", embedded: true,
	})
	archivedObs := seedObservation(t, db, fixture.org.ID, observationSeed{
		channelID: &fixture.channel.ID, content: "Superseded observation.", kind: "decision", archivedAt: &now, embedded: true,
	})
	expiredObs := seedObservation(t, db, fixture.org.ID, observationSeed{
		channelID: &fixture.channel.ID, content: "Expired observation.", kind: "commitment", expiresAt: &past, embedded: true,
	})

	results := callMemoryTool(t, ctx, client, "search_memories", map[string]any{
		"query":            "deploy platform decision",
		"_hivy_session_id": fixture.session.ID.String(),
	})
	if results["layer"] != memoryLayerObservations {
		t.Fatalf("default search layer = %v, want %q", results["layer"], memoryLayerObservations)
	}
	ids := resultIDSet(results)
	if len(ids) != 2 || !ids[channelObs.String()] || !ids[orgObs.String()] {
		t.Fatalf("observation search = %#v, want channel + org-wide observations", results)
	}
	for name, id := range map[string]uuid.UUID{"other channel": leakObs, "archived": archivedObs, "expired": expiredObs} {
		if ids[id.String()] {
			t.Fatalf("observation search leaked %s observation: %#v", name, results)
		}
	}
	for _, raw := range results["results"].([]any) {
		item := raw.(map[string]any)
		switch item["id"] {
		case channelObs.String():
			if item["kind"] != "decision" || item["proof_count"].(float64) != 3 ||
				item["last_mentioned_at"] == nil || item["entities"].([]any)[0] != "Railway" {
				t.Fatalf("observation response shape mismatch: %#v", item)
			}
			history, _ := item["history"].([]any)
			if len(history) != 1 {
				t.Fatalf("rewritten observation history = %#v, want 1 entry", item["history"])
			}
			entry := history[0].(map[string]any)
			if entry["previous_content"] != "The team deploys on Fly." ||
				entry["at"] != "2026-06-20T00:00:00Z" || entry["reason"] != "state change" {
				t.Fatalf("history entry = %#v, want the recorded rewrite", entry)
			}
		case orgObs.String():
			if _, present := item["history"]; present {
				t.Fatalf("never-rewritten observation must carry no history: %#v", item)
			}
		}
	}
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
