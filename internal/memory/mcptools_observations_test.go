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
		if item["id"] != channelObs.String() {
			continue
		}
		if item["kind"] != "decision" || item["proof_count"].(float64) != 3 ||
			item["last_mentioned_at"] == nil || item["entities"].([]any)[0] != "Railway" {
			t.Fatalf("observation response shape mismatch: %#v", item)
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

	channelObs := seedObservation(t, db, fixture.org.ID, observationSeed{
		channelID: &fixture.channel.ID, content: "Channel-level convention: PRs need one approval.", kind: "convention", embedded: true,
	})
	orgObs := seedObservation(t, db, fixture.org.ID, observationSeed{
		content: "Org-wide: escalate outages within 15 minutes.", kind: "rule", embedded: true,
	})

	all := callMemoryTool(t, ctx, client, "manage_memories", map[string]any{"action": "search", "query": "conventions and rules"})
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

func TestChannelMemoryDigestRead(t *testing.T) {
	ctx := context.Background()
	db := connectMemoryToolTestDB(t)
	requireMemoryTables(t, db, "channel_memory_digests")
	fixture := seedMemoryToolFixture(t, db)
	service := NewService(Config{DB: db})

	// Missing row reads as empty, not an error.
	digest, err := service.ChannelMemoryDigest(ctx, fixture.org.ID, fixture.channel.ID)
	if err != nil || digest != "" {
		t.Fatalf("missing digest = %q, %v; want empty, nil", digest, err)
	}

	content := "- [decision] Chose Railway over Fly in June 2026.\n- [rule] PRs need one approval."
	if err := db.Exec(`
INSERT INTO channel_memory_digests (channel_id, org_id, content, observation_count, updated_at)
VALUES (?, ?, ?, ?, now())`,
		fixture.channel.ID, fixture.org.ID, content, 2,
	).Error; err != nil {
		t.Fatalf("seed digest: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM channel_memory_digests WHERE channel_id = ?`, fixture.channel.ID) })

	digest, err = service.ChannelMemoryDigest(ctx, fixture.org.ID, fixture.channel.ID)
	if err != nil || digest != content {
		t.Fatalf("digest = %q, %v; want stored content", digest, err)
	}
}

func TestActiveDirectivesRead(t *testing.T) {
	ctx := context.Background()
	db := connectMemoryToolTestDB(t)
	requireMemoryTables(t, db, "agent_directives")
	fixture := seedMemoryToolFixture(t, db)
	service := NewService(Config{DB: db})

	seedDirective := func(channelID *uuid.UUID, content string, active bool, createdAt time.Time) uuid.UUID {
		id := uuid.New()
		if err := db.Exec(`
INSERT INTO agent_directives (id, org_id, channel_id, content, created_by_user_id, source, active, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, 'user-pinned', ?, ?, ?)`,
			id, fixture.org.ID, channelID, content, fixture.user.ID, active, createdAt, createdAt,
		).Error; err != nil {
			t.Fatalf("seed directive: %v", err)
		}
		t.Cleanup(func() { db.Exec(`DELETE FROM agent_directives WHERE id = ?`, id) })
		return id
	}

	now := time.Now()
	first := seedDirective(&fixture.channel.ID, "Always deploy through Railway.", true, now.Add(-2*time.Hour))
	second := seedDirective(nil, "Escalate outages within 15 minutes.", true, now.Add(-time.Hour))
	seedDirective(&fixture.channel.ID, "Inactive rule.", false, now)

	channelID := fixture.channel.ID
	directives, err := service.ActiveDirectives(ctx, fixture.org.ID, ChannelScope{ChannelID: &channelID, IncludeOrgMemories: true})
	if err != nil {
		t.Fatalf("ActiveDirectives: %v", err)
	}
	if len(directives) != 2 || directives[0].ID != first || directives[1].ID != second {
		t.Fatalf("ActiveDirectives = %#v, want the two active directives oldest-first", directives)
	}

	channelOnly, err := service.ActiveDirectives(ctx, fixture.org.ID, ChannelScope{ChannelID: &channelID})
	if err != nil {
		t.Fatalf("ActiveDirectives channel-only: %v", err)
	}
	if len(channelOnly) != 1 || channelOnly[0].ID != first {
		t.Fatalf("channel-only directives = %#v, want only the channel directive", channelOnly)
	}
}
