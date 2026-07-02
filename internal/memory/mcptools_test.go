package memory

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func TestMemoryMCPToolsRetainSearchAndForget(t *testing.T) {
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

	retained := callMemoryTool(t, ctx, client, "retain_memory", map[string]any{
		"content":          "The agent should use the Helio launch checklist before release planning.",
		"target":           map[string]any{"owner": "user", "visibility": "this_agent"},
		"tags":             []string{"launch", "preference"},
		"_hivy_session_id": fixture.session.ID.String(),
	})
	retainedMemory := retained["memory"].(map[string]any)
	retainedID := uuid.MustParse(retainedMemory["id"].(string))
	assertRetainedMemoryTarget(t, db, retainedID, fixture.user.ID, fixture.agent.ID)

	seedReadyMemory(t, service, fixture.org.ID, nil, nil, "The org billing policy is Net 30 for annual contracts.")
	seedReadyMemory(t, service, fixture.org.ID, &fixture.agent.ID, nil, "This agent owns the Helio release checklist.")

	agentResults := callMemoryTool(t, ctx, client, "search_memories", map[string]any{
		"query":            "Helio release checklist",
		"target":           map[string]any{"owner": "org", "visibility": "this_agent"},
		"_hivy_session_id": fixture.session.ID.String(),
	})
	if got := len(agentResults["results"].([]any)); got != 1 {
		t.Fatalf("this_agent search returned %d results, want 1: %#v", got, agentResults)
	}

	sharedResults := callMemoryTool(t, ctx, client, "search_memories", map[string]any{
		"query":            "billing policy",
		"target":           map[string]any{"owner": "org", "visibility": "all_agents"},
		"_hivy_session_id": fixture.session.ID.String(),
	})
	if got := len(sharedResults["results"].([]any)); got != 1 {
		t.Fatalf("all_agents search returned %d results, want 1: %#v", got, sharedResults)
	}

	assertMemoryToolError(t, ctx, client, "search_memories", map[string]any{
		"query":            "this query has far too many words",
		"_hivy_session_id": fixture.session.ID.String(),
	}, "query must be at most")
	assertMemoryToolError(t, ctx, client, "retain_memory", map[string]any{
		"content":          "The agent should remember this malformed tag test.",
		"target":           map[string]any{"owner": "org", "visibility": "this_agent"},
		"tags":             []string{"Project Helio"},
		"_hivy_session_id": fixture.session.ID.String(),
	}, "lowercase kebab-case")

	sharedID := seedReadyMemory(t, service, fixture.org.ID, nil, nil, "Shared org memory can be forgotten by an agent.")
	callMemoryTool(t, ctx, client, "forget_memory", map[string]any{
		"memory_id":        sharedID.String(),
		"_hivy_session_id": fixture.session.ID.String(),
	})
	assertMemoryArchived(t, db, sharedID)

	otherUserMemoryID := seedReadyMemory(t, service, fixture.org.ID, nil, &fixture.otherUser.ID, "Other user's billing preference.")
	assertMemoryToolError(t, ctx, client, "forget_memory", map[string]any{
		"memory_id":        otherUserMemoryID.String(),
		"_hivy_session_id": fixture.session.ID.String(),
	}, "another user's memory")

	callMemoryTool(t, ctx, client, "forget_memory", map[string]any{
		"memory_id":        retainedID.String(),
		"_hivy_session_id": fixture.session.ID.String(),
	})
	assertMemoryArchived(t, db, retainedID)
}

func TestMemoryMCPToolsForgetAgentIsolation(t *testing.T) {
	ctx := context.Background()
	db := connectMemoryToolTestDB(t)
	fixture := seedMemoryToolFixture(t, db)
	service := NewService(Config{DB: db, Embedder: staticMemoryToolEmbedder{vector: testMemoryVector()}})

	otherAgent := model.Agent{ID: uuid.New(), OrgID: &fixture.org.ID, Name: "Memory MCP Other Agent " + uuid.NewString(), Model: "test", Status: "active"}
	if err := db.Create(&otherAgent).Error; err != nil {
		t.Fatalf("create other agent: %v", err)
	}
	otherToken := &model.Token{
		OrgID: fixture.org.ID,
		Meta: model.JSON{
			model.TokenMetaType:    model.TokenTypeAgentProxy,
			model.TokenMetaAgentID: otherAgent.ID.String(),
		},
	}
	otherClient := connectMemoryToolClient(t, ctx, service, otherToken)

	privateID := seedReadyMemory(t, service, fixture.org.ID, &fixture.agent.ID, nil, "Agent A's private release checklist notes.")
	assertMemoryToolError(t, ctx, otherClient, "forget_memory", map[string]any{
		"memory_id": privateID.String(),
	}, "another agent's memory")
	assertMemoryNotArchived(t, db, privateID)

	sharedID := seedReadyMemory(t, service, fixture.org.ID, nil, nil, "Org-wide memory forgettable by any agent.")
	callMemoryTool(t, ctx, otherClient, "forget_memory", map[string]any{
		"memory_id": sharedID.String(),
	})
	assertMemoryArchived(t, db, sharedID)

	// SQL hardening: an agent-scoped archive must not cross agent boundaries
	// even when the Go guard is bypassed.
	if err := service.Archive(ctx, ArchiveRequest{OrgID: fixture.org.ID, ID: privateID, AgentID: &otherAgent.ID}); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("agent-scoped archive of another agent's memory = %v, want gorm.ErrRecordNotFound", err)
	}
	assertMemoryNotArchived(t, db, privateID)

	// The unscoped path (REST/dashboard) can still archive agent-bound memories.
	if err := service.Archive(ctx, ArchiveRequest{OrgID: fixture.org.ID, ID: privateID}); err != nil {
		t.Fatalf("unscoped archive: %v", err)
	}
	assertMemoryArchived(t, db, privateID)
}

func assertMemoryToolError(t *testing.T, ctx context.Context, client *mcp.ClientSession, name string, args map[string]any, want string) {
	t.Helper()
	result, err := client.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
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
