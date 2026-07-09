package memory

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

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

	// Agents are read-only on memory: the write tools must not register.
	for _, tool := range tools.Tools {
		if tool.Name == "retain_memory" || tool.Name == "forget_memory" {
			t.Fatalf("removed memory write tool %s is still registered", tool.Name)
		}
	}

	otherChannel := model.Channel{ID: uuid.New(), OrgID: fixture.org.ID, TeamID: fixture.channel.TeamID, Name: "memory-mcp-other-" + uuid.NewString(), DefaultAgentID: fixture.agent.ID, ExposeOrgMemories: true}
	if err := db.Create(&otherChannel).Error; err != nil {
		t.Fatalf("create other channel: %v", err)
	}

	channelMem := seedReadyMemory(t, service, fixture.org.ID, &fixture.channel.ID, "This channel owns the Helio release checklist.")
	orgMem := seedReadyMemory(t, service, fixture.org.ID, nil, "The org billing policy is Net 30 for annual contracts.")
	otherMem := seedReadyMemory(t, service, fixture.org.ID, &otherChannel.ID, "A different channel's private note.")

	// Search is auto-scoped: this channel plus org-wide (channel exposes them).
	// include_facts pins the legacy facts layer; the default observations layer
	// has its own coverage in mcptools_observations_test.go.
	results := callMemoryTool(t, ctx, client, "search_memories", map[string]any{
		"query":            "release checklist",
		"include_facts":    true,
		"_hivy_session_id": fixture.session.ID.String(),
	})
	if results["layer"] != memoryLayerFacts {
		t.Fatalf("include_facts search layer = %v, want %q", results["layer"], memoryLayerFacts)
	}
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
	assertMemoryToolError(t, ctx, client, "search_memories", map[string]any{
		"query":            "release checklist",
		"tags":             []string{"Project Helio"},
		"_hivy_session_id": fixture.session.ID.String(),
	}, "lowercase kebab-case")
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
		"include_facts":    true,
		"_hivy_session_id": fixture.session.ID.String(),
	})
	ids := resultIDSet(results)
	if len(ids) != 1 || !ids[channelMem.String()] {
		t.Fatalf("search with exposure off = %#v, want only the channel memory", results)
	}
	if ids[orgMem.String()] {
		t.Fatalf("search leaked org-wide memory despite exposure off: %#v", results)
	}
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
