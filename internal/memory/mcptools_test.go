package memory

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func TestMemoryMCPToolsChannelScoping(t *testing.T) {
	ctx := context.Background()
	db := connectMemoryToolTestDB(t)
	fixture := seedMemoryToolFixture(t, db)
	service := NewService(Config{DB: db, Embedder: staticMemoryToolEmbedder{vector: testMemoryVector()}})
	client := connectMemoryToolClient(t, ctx, service, fixture.token)

	tools, err := client.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	assertMemoryToolDescriptions(t, tools.Tools)

	// retain writes to the session's channel, no target argument required.
	retained := callMemoryTool(t, ctx, client, "retain_memory", map[string]any{
		"content":          "The team uses the Helio launch checklist before release planning.",
		"tags":             []string{"launch", "preference"},
		"_hivy_session_id": fixture.session.ID.String(),
	})
	retainedID := uuid.MustParse(retained["memory"].(map[string]any)["id"].(string))
	assertMemoryChannel(t, db, retainedID, &fixture.channel.ID)

	otherChannel := model.Channel{ID: uuid.New(), OrgID: fixture.org.ID, Name: "memory-mcp-other-" + uuid.NewString(), DefaultAgentID: fixture.agent.ID, ExposeOrgMemories: true}
	if err := db.Create(&otherChannel).Error; err != nil {
		t.Fatalf("create other channel: %v", err)
	}

	channelMem := seedReadyMemory(t, service, fixture.org.ID, &fixture.channel.ID, "This channel owns the Helio release checklist.")
	orgMem := seedReadyMemory(t, service, fixture.org.ID, nil, "The org billing policy is Net 30 for annual contracts.")
	otherMem := seedReadyMemory(t, service, fixture.org.ID, &otherChannel.ID, "A different channel's private note.")

	// Search is auto-scoped: this channel plus org-wide (channel exposes them).
	results := callMemoryTool(t, ctx, client, "search_memories", map[string]any{
		"query":            "release checklist",
		"_hivy_session_id": fixture.session.ID.String(),
	})
	ids := resultIDSet(results)
	if len(ids) != 2 || !ids[channelMem.String()] || !ids[orgMem.String()] {
		t.Fatalf("channel search = %#v, want channel + org-wide memories", results)
	}
	if ids[otherMem.String()] {
		t.Fatalf("search leaked another channel's memory: %#v", results)
	}

	assertMemoryToolError(t, ctx, client, "search_memories", map[string]any{
		"query":            "this query has far too many words",
		"_hivy_session_id": fixture.session.ID.String(),
	}, "query must be at most")
	assertMemoryToolError(t, ctx, client, "retain_memory", map[string]any{
		"content":          "The agent should remember this malformed tag test.",
		"tags":             []string{"Project Helio"},
		"_hivy_session_id": fixture.session.ID.String(),
	}, "lowercase kebab-case")

	// forget refuses a memory that belongs to another channel.
	assertMemoryToolError(t, ctx, client, "forget_memory", map[string]any{
		"memory_id":        otherMem.String(),
		"_hivy_session_id": fixture.session.ID.String(),
	}, "only archive memories in this channel")
	assertMemoryNotArchived(t, db, otherMem)

	// forget allows this channel's memory and org-wide memories (channel exposes them).
	callMemoryTool(t, ctx, client, "forget_memory", map[string]any{
		"memory_id":        channelMem.String(),
		"_hivy_session_id": fixture.session.ID.String(),
	})
	assertMemoryArchived(t, db, channelMem)
	callMemoryTool(t, ctx, client, "forget_memory", map[string]any{
		"memory_id":        orgMem.String(),
		"_hivy_session_id": fixture.session.ID.String(),
	})
	assertMemoryArchived(t, db, orgMem)
}

func TestMemoryMCPToolsExposeOrgMemoriesToggle(t *testing.T) {
	ctx := context.Background()
	db := connectMemoryToolTestDB(t)
	fixture := seedMemoryToolFixture(t, db)
	if err := db.Model(&model.Channel{}).Where("id = ?", fixture.channel.ID).Update("expose_org_memories", false).Error; err != nil {
		t.Fatalf("disable expose_org_memories: %v", err)
	}
	service := NewService(Config{DB: db, Embedder: staticMemoryToolEmbedder{vector: testMemoryVector()}})
	client := connectMemoryToolClient(t, ctx, service, fixture.token)

	channelMem := seedReadyMemory(t, service, fixture.org.ID, &fixture.channel.ID, "Channel-only note about the release.")
	orgMem := seedReadyMemory(t, service, fixture.org.ID, nil, "Org-wide note hidden from this channel.")

	results := callMemoryTool(t, ctx, client, "search_memories", map[string]any{
		"query":            "release note",
		"_hivy_session_id": fixture.session.ID.String(),
	})
	ids := resultIDSet(results)
	if len(ids) != 1 || !ids[channelMem.String()] {
		t.Fatalf("search with exposure off = %#v, want only the channel memory", results)
	}
	if ids[orgMem.String()] {
		t.Fatalf("search leaked org-wide memory despite exposure off: %#v", results)
	}

	// With exposure off, org-wide memories are out of this channel's forget scope.
	assertMemoryToolError(t, ctx, client, "forget_memory", map[string]any{
		"memory_id":        orgMem.String(),
		"_hivy_session_id": fixture.session.ID.String(),
	}, "only archive memories in this channel")
	assertMemoryNotArchived(t, db, orgMem)
}

func TestMemoryMCPToolsSessionRequired(t *testing.T) {
	ctx := context.Background()
	db := connectMemoryToolTestDB(t)
	fixture := seedMemoryToolFixture(t, db)
	service := NewService(Config{DB: db, Embedder: staticMemoryToolEmbedder{vector: testMemoryVector()}})
	client := connectMemoryToolClient(t, ctx, service, fixture.token)

	assertMemoryToolError(t, ctx, client, "search_memories", map[string]any{
		"query": "release checklist",
	}, "_hivy_session_id is required")
	assertMemoryToolError(t, ctx, client, "retain_memory", map[string]any{
		"content": "This should be rejected without a session.",
	}, "_hivy_session_id is required")
	assertMemoryToolError(t, ctx, client, "search_memories", map[string]any{
		"query":            "release checklist",
		"_hivy_session_id": uuid.NewString(),
	}, "session not found for this agent")
}

func resultIDSet(resp map[string]any) map[string]bool {
	out := map[string]bool{}
	for _, raw := range resp["results"].([]any) {
		out[raw.(map[string]any)["id"].(string)] = true
	}
	return out
}

func assertMemoryChannel(t *testing.T, db *gorm.DB, memoryID uuid.UUID, want *uuid.UUID) {
	t.Helper()
	var mem model.AgentMemory
	if err := db.First(&mem, "id = ?", memoryID).Error; err != nil {
		t.Fatalf("load memory: %v", err)
	}
	if want == nil {
		if mem.ChannelID != nil {
			t.Fatalf("memory %s channel = %v, want org-wide (nil)", memoryID, mem.ChannelID)
		}
		return
	}
	if mem.ChannelID == nil || *mem.ChannelID != *want {
		t.Fatalf("memory %s channel = %v, want %s", memoryID, mem.ChannelID, want)
	}
}

func assertMemoryToolError(t *testing.T, ctx context.Context, client *mcp.ClientSession, name string, args map[string]any, want string) {
	t.Helper()
	result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	if !result.IsError || !strings.Contains(memoryToolText(result), want) {
		t.Fatalf("%s error = %v text %q, want %q", name, result.IsError, memoryToolText(result), want)
	}
}

func assertMemoryArchived(t *testing.T, db *gorm.DB, memoryID uuid.UUID) {
	t.Helper()
	var archived model.AgentMemory
	if err := db.First(&archived, "id = ?", memoryID).Error; err != nil {
		t.Fatalf("load archived memory: %v", err)
	}
	if archived.ArchivedAt == nil {
		t.Fatalf("memory %s was not archived", memoryID)
	}
}

func assertMemoryNotArchived(t *testing.T, db *gorm.DB, memoryID uuid.UUID) {
	t.Helper()
	var mem model.AgentMemory
	if err := db.First(&mem, "id = ?", memoryID).Error; err != nil {
		t.Fatalf("load memory: %v", err)
	}
	if mem.ArchivedAt != nil {
		t.Fatalf("memory %s was archived but should not be", memoryID)
	}
}
