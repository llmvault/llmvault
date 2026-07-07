package memory

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

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
	// Soft-deleted rules are history, never injected — even when still active.
	softDeleted := seedDirective(&fixture.channel.ID, "Deleted rule.", true, now.Add(-3*time.Hour))
	if err := db.Exec(`UPDATE agent_directives SET deleted_at = now() WHERE id = ?`, softDeleted).Error; err != nil {
		t.Fatalf("soft-delete directive: %v", err)
	}

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
