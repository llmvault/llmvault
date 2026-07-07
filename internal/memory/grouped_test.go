package memory

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestGroupedByChannelAndPagination(t *testing.T) {
	db := connectMemoryToolTestDB(t)
	ctx := context.Background()
	service := NewService(Config{DB: db, Embedder: staticMemoryToolEmbedder{vector: testMemoryVector()}})
	fx := seedMemoryToolFixture(t, db)
	channelID := fx.channel.ID

	// 12 memories in one channel (> perChannel), 2 global.
	for i := range 12 {
		seedReadyMemory(t, service, fx.org.ID, &channelID, "channel memory "+string(rune('a'+i)))
	}
	seedReadyMemory(t, service, fx.org.ID, nil, "global memory one")
	seedReadyMemory(t, service, fx.org.ID, nil, "global memory two")

	groups, err := service.GroupedByChannel(ctx, fx.org.ID, 10, Visibility{})
	if err != nil {
		t.Fatalf("grouped: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}
	// Global group is first.
	if groups[0].ChannelID != nil {
		t.Fatalf("first group should be global, got channel %v", groups[0].ChannelID)
	}
	if len(groups[0].Memories) != 2 || groups[0].HasMore || groups[0].Total != 2 {
		t.Fatalf("global group = %d mems has_more=%v total=%d", len(groups[0].Memories), groups[0].HasMore, groups[0].Total)
	}
	channelGroup := groups[1]
	if channelGroup.ChannelID == nil || *channelGroup.ChannelID != channelID {
		t.Fatalf("second group channel = %v, want %v", channelGroup.ChannelID, channelID)
	}
	if len(channelGroup.Memories) != 10 {
		t.Fatalf("channel group returned %d, want 10 (perChannel cap)", len(channelGroup.Memories))
	}
	if !channelGroup.HasMore || channelGroup.Total != 12 {
		t.Fatalf("channel group has_more=%v total=%d, want true/12", channelGroup.HasMore, channelGroup.Total)
	}

	// Page the channel: first page of 10, then the remaining 2.
	page1, next, hasMore, err := service.ListChannelPage(ctx, fx.org.ID, ChannelScope{ChannelID: &channelID}, "", 10)
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if len(page1) != 10 || !hasMore || next == "" {
		t.Fatalf("page1 len=%d has_more=%v next=%q", len(page1), hasMore, next)
	}
	page2, next2, hasMore2, err := service.ListChannelPage(ctx, fx.org.ID, ChannelScope{ChannelID: &channelID}, next, 10)
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(page2) != 2 || hasMore2 || next2 != "" {
		t.Fatalf("page2 len=%d has_more=%v next=%q", len(page2), hasMore2, next2)
	}
	// No overlap between pages.
	seen := map[uuid.UUID]bool{}
	for _, m := range append(page1, page2...) {
		if seen[m.ID] {
			t.Fatalf("duplicate memory %s across pages", m.ID)
		}
		seen[m.ID] = true
	}

	// Global scope paging returns only the 2 channel-less memories.
	globalPage, _, globalMore, err := service.ListChannelPage(ctx, fx.org.ID, ChannelScope{}, "", 10)
	if err != nil {
		t.Fatalf("global page: %v", err)
	}
	if len(globalPage) != 2 || globalMore {
		t.Fatalf("global page len=%d has_more=%v", len(globalPage), globalMore)
	}
	for _, m := range globalPage {
		if m.ChannelID != nil {
			t.Fatalf("global page returned a channel memory: %v", m.ChannelID)
		}
	}
}
